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
)

var (
	wsEventListener        func(context.Context, ...*model.Event) error
	wsSubscriptionListener EventGetter
	reqMustAuth            ReqMustAuthenticate
	eventMustAuth          EventAuthenticate
	hdl                    *handler
)

func RegisterWSEventListener(listen func(context.Context, ...*model.Event) error) {
	wsEventListener = listen
}

func RegisterWSSubscriptionListener(listen EventGetter) {
	wsSubscriptionListener = listen
}

func RegisterReqMustAuthenticate(cb ReqMustAuthenticate) {
	reqMustAuth = cb
}

func RegisterEventMustAuthenticate(cb EventAuthenticate) {
	eventMustAuth = cb
}

func notifySubscriptions(event *model.Event) error {
	if hdl == nil {
		log.Panic("Server is not started")
	}

	return hdl.notifyListenersAboutNewEvents(event)
}

func newHandler(relayURL string) *handler {
	// Initialize the GLOBAL handler.
	hdl = &handler{
		subListeners: make(map[adapters.WSWriter]map[string]*model.Subscription),
		connAuth:     xsync.NewMapOf[adapters.WSWriter, connAuthData](),
		relayURL:     relayURL,
	}

	return hdl
}

func NewHandler(relayURL string) WSHandler {
	return newHandler(relayURL)
}

func New(cfg *Config, routes internal.RegisterRoutes) Server {
	return internal.NewWSServer(routes, cfg)
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
	if err := h.CancelSubscription(ctx, stream, nil); err != nil {
		log.Printf("ERROR:%v", errors.Wrap(err, "failed to cancel subscriptions opened on closing conn"))
	}
}

func (h *handler) populateContext(ctx context.Context, respWriter adapters.WSWriter) context.Context {
	if v, ok := h.connAuth.Load(respWriter); ok {
		return model.SetUserDataInContext(ctx, v.PublicKey, v.Authenticated)
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
		var resp = nostr.OKEnvelope{
			EventID: e.Event.ID,
		}
		state, ok := h.connAuth.Load(respWriter)
		switch {
		case !ok:
			resp.Reason = "received unexpected auth message: no challenge"

		case state.Authenticated:
			resp.Reason = "received unexpected auth message: already authenticated"

		case !state.Authenticated && e.Event.Sig != "":
			_, vErr := model.ValidateAuthEvent(&model.Event{Event: e.Event}, state.Challenge, h.relayURL)
			if vErr != nil {
				resp.Reason = errors.Wrap(vErr, "failed to validate auth event").Error()
			} else {
				h.connAuth.Store(respWriter, connAuthData{
					Challenge:     state.Challenge,
					PublicKey:     e.Event.PubKey,
					Authenticated: true,
				})
				resp.OK = true
			}
		default: // Should never happen.
			log.Printf("ERROR: unexpected auth message %+v", e)
			resp.Reason = "received unexpected auth message"
		}
		err = h.writeResponse(respWriter, &resp)
	case *nostr.ReqEnvelope:
		err = h.handleReq(ctx, respWriter, &model.Subscription{Filters: e.Filters, SubscriptionID: e.SubscriptionID})
	case *nostr.CountEnvelope:
		err = h.handleCount(ctx, e)
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
		subID := string(*e)
		err = h.CancelSubscription(ctx, respWriter, &subID)
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
