// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

const (
	operationLogThreshold = time.Millisecond * 150
)

type (
	EventIterator       = query.EventIterator
	EventGetter         func(context.Context, ...model.Filter) EventIterator
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

func NewHandler(relayURL string) Handler {
	return newHandler(relayURL)
}

func New(cfg *Config, routes internal.RegisterRoutes) Server {
	return internal.NewWSServer(routes, cfg)
}

func newHandler(relayURL string) *handler {
	return &handler{
		Subscriptions: xsync.NewMap[string, subscription](),
		ConnAuth:      xsync.NewMap[Writer, connAuthData](),
		RelayURL:      relayURL,
	}
}

func (h *handler) Read(ctx context.Context, stream internal.WS) {
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
			go h.Handle(ctx, stream, msgBytes)
		}
	}
	h.unlinkSubscription(stream, nil)
}

func (h *handler) populateContext(ctx context.Context, respWriter adapters.WSWriter) context.Context {
	if v, ok := h.ConnAuth.Load(respWriter); ok {
		return model.SetUserDataInContext(ctx, v.UserDataContext)
	}
	return ctx
}

func (h *handler) logOperation(respWriter adapters.WSWriter, duration time.Duration, msg string, args ...any) {
	if duration < operationLogThreshold {
		return
	}

	prefix := "[WS]: stats: duration: [" + duration.String() + "]"
	if v, ok := h.ConnAuth.Load(respWriter); ok && v.Authenticated {
		prefix += " master: [" + v.MasterPublicKey + "]"
		if v.UserAgent != "" {
			prefix += " agent: [" + v.UserAgent + "]"
		}
	}
	prefix += ": "
	log.Printf(prefix+msg, args...)
}

func (h *handler) Handle(ctx context.Context, respWriter adapters.WSWriter, msgBytes []byte) {
	input, err := nostr.ParseMessage(msgBytes)
	if err != nil {
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", errors.Join(err, h.writeResponse(ctx, respWriter, &notice)))

		return
	}

	start := time.Now()
	switch e := input.(type) {
	case *nostr.EventEnvelope:
		events := make([]*model.Event, 0, len(e.Events))
		for i := range e.Events {
			events = append(events, &model.Event{Event: *e.Events[i]})
		}
		err = h.handleEvents(h.populateContext(context.WithoutCancel(ctx), respWriter), respWriter, events)
		if err != nil {
			log.Printf("ERROR: cannot process events: %s: %v", model.Events(events).String(), err)
		}
		h.logOperation(respWriter, time.Since(start), "events: handle [%d] events: %v", len(events), string(msgBytes))
		sendStart := time.Now()
		for i := range e.Events {
			resp := &nostr.OKEnvelope{
				EventID: e.Events[i].ID,
				OK:      true,
			}
			if err != nil {
				resp.OK = false
				resp.Reason = err.Error()
			}

			wErr := h.writeResponse(ctx, respWriter, resp)
			if wErr != nil {
				log.Printf("ERROR: write event response %v: %v", i, wErr)

				break
			}
		}
		h.logOperation(respWriter, time.Since(sendStart), "events: send [%d] responses", len(events))
		return
	case *nostr.AuthEnvelope:
		ev := &model.Event{Event: e.Event}
		err = h.writeResponse(ctx, respWriter, h.handleAuth(ctx, respWriter, ev))
		h.logOperation(respWriter, time.Since(start), "auth")
	case *nostr.ReqEnvelope:
		err = h.handleReq(h.populateContext(ctx, respWriter), respWriter, model.NewSubscription(e.SubscriptionID, e.Filters))
		h.logOperation(respWriter, time.Since(start), "req: %s: handle [%d] filters: %v: [%v]",
			e.SubscriptionID, len(e.Filters), string(msgBytes),
			err)
	case *nostr.CountEnvelope:
		err = h.handleCount(h.populateContext(ctx, respWriter), e)
		if err != nil {
			defer respWriter.Close()

			closedEnvelope := nostr.ClosedEnvelope{
				SubscriptionID: e.SubscriptionID,
				Reason:         err.Error(),
			}
			err = h.writeResponse(ctx, respWriter, &closedEnvelope)
		} else {
			err = h.writeResponse(ctx, respWriter, e)
		}
	case *nostr.CloseEnvelope:
		h.unlinkSubscription(respWriter, (*string)(e))
		h.logOperation(respWriter, time.Since(start), "req: close: %s", (*string)(e))
	default:
		err = errors.Errorf("unknown message type %v", input.Label())
	}

	if err != nil {
		err = errors.Wrapf(err, "error: failed to handle %v %+v", input.Label(), input)
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", errors.Join(err, h.writeResponse(ctx, respWriter, &notice)))
	}
}

func (h *handler) writeResponse(ctx context.Context, respWriter adapters.WSWriter, envelope nostr.Envelope) error {
	b, err := envelope.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "failed to serialize %+v into json", envelope)
	}
	return respWriter.WriteMessage(ctx, int(ws.OpText), b)
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
