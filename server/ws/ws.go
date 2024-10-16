// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"io"
	"log"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

type EventGetter func(context.Context, *model.Subscription) query.EventIterator

var wsEventListener func(context.Context, ...*model.Event) error
var wsSubscriptionListener EventGetter

func RegisterWSEventListener(listen func(context.Context, ...*model.Event) error) {
	wsEventListener = listen
}

func RegisterWSSubscriptionListener(listen EventGetter) {
	wsSubscriptionListener = listen
}

func notifySubscriptions(event *model.Event) error {
	if hdl == nil {
		log.Panic("Server is not started")
	}

	return hdl.notifyListenersAboutNewEvents(event)
}

var hdl *handler

func NewHandler() WSHandler {
	hdl = new(handler)

	return hdl
}

func New(cfg *Config, routes internal.RegisterRoutes) Server {
	return internal.NewWSServer(routes, cfg)
}

func (h *handler) Read(ctx context.Context, stream internal.WS, cfg *Config) {
	for {
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

func (h *handler) Handle(ctx context.Context, respWriter adapters.WSWriter, msgBytes []byte, cfg *Config) {
	input, err := model.ParseMessage(msgBytes)
	if err != nil {
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())

		return
	}

	switch e := input.(type) {
	case *model.EventEnvelope:
		err = h.handleEvents(ctx, e.Events, cfg)
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
	case *nostr.ReqEnvelope:
		err = h.handleReq(ctx, respWriter, &subscription{Subscription: &model.Subscription{Filters: e.Filters}, SubscriptionID: e.SubscriptionID})
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
