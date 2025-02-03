// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

type (
	EventIterator       = query.EventIterator
	EventGetter         func(context.Context, *model.Subscription) EventIterator
	ReqMustAuthenticate func(context.Context, *model.Subscription) (authRequired bool)
	EventAuthenticate   func(context.Context, ...*model.Event) (authRequired bool)
	EventListener       func(context.Context, ...*model.Event) error
)

var (
	wsEventListener         EventListener
	wsSubscriptionListeners []EventGetter
	reqMustAuth             ReqMustAuthenticate
	eventMustAuth           EventAuthenticate
)

func RegisterWSEventListener(listen EventListener) {
	wsEventListener = listen
}

func RegisterWSSubscriptionListener(listen ...EventGetter) {
	wsSubscriptionListeners = listen
}

func RegisterReqMustAuthenticate(cb ReqMustAuthenticate) {
	reqMustAuth = cb
}

func RegisterEventMustAuthenticate(cb EventAuthenticate) {
	eventMustAuth = cb
}

func NewHandler(relayURL string) WSHandler {
	return newHandler(relayURL)
}

func New(cfg *Config, routes internal.RegisterRoutes) Server {
	return internal.NewWSServer(routes, cfg)
}

func newHandler(relayURL string) *handler {
	return &handler{
		connSubs: xsync.NewMapOf[Writer, connSubscriptions](),
		connAuth: xsync.NewMapOf[Writer, connAuthData](),
		relayURL: relayURL,
	}
}

func (h *handler) Read(ctx context.Context, stream internal.WS, cfg *Config) {
	for ctx.Err() == nil {
		t, msgBytes, err := stream.ReadMessage()
		if err != nil {
			closed := new(wsutil.ClosedError)
			if errors.As(err, closed) {
				if closed.Code != ws.StatusNormalClosure &&
					closed.Code != ws.StatusGoingAway &&
					closed.Code != ws.StatusAbnormalClosure &&
					closed.Code != ws.StatusNoStatusRcvd {
					log.Printf("WARN: unexpected close error %v: %v", closed.Code, closed.Code)
				}
			} else if !errors.Is(err, io.EOF) {
				log.Printf("WARN: unexpected close error %v: %v", closed.Code, closed.Code)
			}
			break
		}
		if len(msgBytes) > 0 && ws.OpCode(t) == ws.OpText {
			h.Handle(ctx, stream, msgBytes, cfg)
		}
	}
	h.unlinkSubscription(stream, nil)
}

func (h *handler) populateContext(ctx context.Context, respWriter adapters.WSWriter) context.Context {
	if v, ok := h.connAuth.Load(respWriter); ok {
		return model.SetUserDataInContext(ctx, v.MasterPublicKey, v.PublicKey, v.Authenticated)
	}
	return ctx
}

func (h *handler) Handle(ctx context.Context, respWriter adapters.WSWriter, msgBytes []byte, cfg *Config) {
	input, err := nostr.ParseMessage(msgBytes)
	if err != nil {
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())

		return
	}

	switch e := input.(type) {
	case *nostr.EventEnvelope:
		var events []*model.Event
		for i := range e.Events {
			events = append(events, &model.Event{Event: *e.Events[i]})
		}
		err = h.handleEvents(h.populateContext(ctx, respWriter), respWriter, events, cfg)
		if errors.Is(err, ErrNotifyFailed) {
			// Not critical, just log it.
			log.Printf("WARN: notification failed: %v", err)
			err = nil
		}
		for i := range e.Events {
			resp := &nostr.OKEnvelope{
				EventID: e.Events[i].ID,
				OK:      true,
			}
			if err != nil {
				log.Printf("ERROR: failed to handle event %v: %v", e.Events[i], err)
				resp.OK = false
				resp.Reason = err.Error()
			}

			wErr := h.writeResponse(respWriter, resp)
			if wErr != nil {
				log.Printf("ERROR: write event response %v: %v", i, wErr)

				return
			}
		}
		return
	case *nostr.AuthEnvelope:
		err = h.writeResponse(respWriter, h.handleAuth(ctx, respWriter, &model.Event{Event: e.Event}))
	case *nostr.ReqEnvelope:
		err = h.handleReq(h.populateContext(ctx, respWriter), respWriter, &model.Subscription{Filters: e.Filters, SubscriptionID: e.SubscriptionID})
	case *nostr.CountEnvelope:
		err = h.handleCount(h.populateContext(ctx, respWriter), e)
		if err != nil {
			defer respWriter.Close()

			closedEnvelope := nostr.ClosedEnvelope{
				SubscriptionID: e.SubscriptionID,
				Reason:         err.Error(),
			}
			err = h.writeResponse(respWriter, &closedEnvelope)
		} else {
			err = h.writeResponse(respWriter, e)
		}
	case *nostr.CloseEnvelope:
		h.unlinkSubscription(respWriter, (*string)(e))
	default:
		err = errors.Errorf("unknown message type %v", input.Label())
	}

	if err != nil {
		err = errors.Wrapf(err, "error: failed to handle %v %+v", input.Label(), input)
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())
	}
}

func (h *handler) writeResponse(respWriter adapters.WSWriter, envelope nostr.Envelope) error {
	b, err := envelope.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "failed to serialize %+v into json", envelope)
	}

	return respWriter.WriteMessage(int(ws.OpText), b)
}

func LoadTLSConfig(certOrFileName, keyOrFileName string) *tls.Config {
	var cert tls.Certificate
	var err error
	if !strings.Contains(certOrFileName, "-----BEGIN CERTIFICATE-----") {
		cert, err = tls.LoadX509KeyPair(certOrFileName, keyOrFileName)
		if err != nil {
			log.Panic(err)
		}
	} else {
		cert, err = tls.X509KeyPair([]byte(certOrFileName), []byte(keyOrFileName))
		if err != nil {
			log.Panic(err)
		}
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}
