// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"errors"
	"io"
	"log"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/gookit/goutil/errorx"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

type EventGetter func(context.Context, *model.Subscription) query.EventIterator

var wsEventListener func(context.Context, *model.Event) error
var wsSubscriptionListener EventGetter

func RegisterWSEventListener(listen func(context.Context, *model.Event) error) {
	wsEventListener = listen
}

func RegisterWSSubscriptionListener(listen EventGetter) {
	wsSubscriptionListener = listen
}

func NotifySubscriptions(event *model.Event) error {
	if hdl == nil {
		log.Panic("Server is not started")
	}

	return hdl.notifyListenersAboutNewEvent(event)
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
		log.Printf("ERROR:%v", errorx.Withf(err, "failed to cancel subscriptions opened on closing conn"))
	}
}

func (h *handler) Handle(ctx context.Context, respWriter adapters.WSWriter, msgBytes []byte, cfg *Config) {
	input := nostr.ParseMessage(msgBytes)
	if input == nil {
		err := errorx.New("failed to parse input")
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())

		return
	}
	var err error
	switch e := input.(type) {
	case *nostr.EventEnvelope:
		err = h.handleEvent(ctx, &model.Event{Event: e.Event}, cfg)
		if err == nil {
			if err = h.writeResponse(respWriter, &nostr.OKEnvelope{
				EventID: e.ID,
				OK:      true,
				Reason:  "",
			}); err != nil {
				log.Printf("ERROR:%v", err)
			}
		}
	case *nostr.ReqEnvelope:
		err = h.handleReq(ctx, respWriter, &subscription{Subscription: &model.Subscription{Filters: model.FromNostrFilters(e.Filters)}, SubscriptionID: e.SubscriptionID})
	case *nostr.CountEnvelope:
		err = h.handleCount(ctx, e)
		if err == nil {
			err = h.writeResponse(respWriter, e)
		}
	case *nostr.CloseEnvelope:
		subID := string(*e)
		err = h.CancelSubscription(ctx, respWriter, &subID)
	default:
		err = errorx.Errorf("unknown message type %v", input.Label())
	}
	if err != nil {
		if e, isEvent := input.(*nostr.EventEnvelope); isEvent {
			err = errorx.Withf(err, "error: failed to handle EVENT %+v", e)
			log.Printf("ERROR:%v", multierror.Append(err, h.writeResponse(respWriter, &nostr.OKEnvelope{
				EventID: e.ID,
				OK:      false,
				Reason:  err.Error(),
			})).ErrorOrNil())

			return
		}
		err = errorx.Withf(err, "error: failed to handle %v %+v", input.Label(), input)
		notice := nostr.NoticeEnvelope(err.Error())
		log.Printf("ERROR:%v", multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())
	}
}

func (h *handler) writeResponse(respWriter adapters.WSWriter, envelope nostr.Envelope) error {
	b, err := envelope.MarshalJSON()
	if err != nil {
		return errorx.Withf(err, "failed to serialize %+v into json", envelope)
	}

	return respWriter.WriteMessage(int(ws.OpText), b)
}
