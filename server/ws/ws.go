package ws

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/hashicorp/go-multierror"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/wintr/log"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/pkg/errors"
	"io"
	"net/http"
)

var wsEventListener func(*model.Event) error
var wsSubscriptionListener func(*model.Subscription) ([]*model.Event, error)

func RegisterWSEventListener(listen func(*model.Event) error) {
	wsEventListener = listen
}

func RegisterWSSubscriptionListener(listen func(*model.Subscription) ([]*model.Event, error)) {
	wsSubscriptionListener = listen
}
func NotifySubscriptions(event *model.Event) error {
	if hdl == nil {
		log.Panic("Server is not started")
	}
	return hdl.notifyListenersAboutNewEvent(event)
}

var hdl *handler

func ListenAndServe(ctx context.Context, cancel context.CancelFunc, cfg *Config) {
	hdl = new(handler)
	internal.NewWSServer(hdl, cfg).ListenAndServe(ctx, cancel)
}

func (h *handler) RegisterRoutes(r *internal.Router) {
	r.Any("/", h.NIP11)
}

func (h *handler) Read(ctx context.Context, stream internal.WS) {
	for {
		t, msgBytes, err := stream.ReadMessage()
		if err != nil {
			closed := new(wsutil.ClosedError)
			if errors.As(err, closed) {
				if closed.Code != ws.StatusNormalClosure &&
					closed.Code != ws.StatusGoingAway &&
					closed.Code != ws.StatusAbnormalClosure &&
					closed.Code != ws.StatusNoStatusRcvd {
					log.Warn(fmt.Sprintf("unexpected close error %v: %v", closed.Code, closed.Code))
				}
			} else if !errors.Is(err, io.EOF) {
				log.Warn(fmt.Sprintf("unexpected close error %v: %v", closed.Code, closed.Code))
			}
			break
		}
		if len(msgBytes) > 0 && ws.OpCode(t) == ws.OpText {
			h.Handle(ctx, stream, msgBytes)
		}
	}
	log.Error(h.CancelSubscription(ctx, stream, nil), "failed to cancel subscriptions opened on closing ws")
}

func (h *handler) NIP11(ctx *gin.Context) {
	if ctx.GetHeader("Accept") != "application/nostr+json" {
		ctx.Status(http.StatusBadRequest)
	}
	ctx.Header("Content-Type", "application/json")
	info := nip11.RelayInformationDocument{
		Name:          "subzero",
		Description:   "subzero",
		PubKey:        "~",
		Contact:       "~",
		SupportedNIPs: []int{11, 16, 20, 33},
		Software:      "subzero",
	}
	ctx.JSON(http.StatusOK, info)
}
func (h *handler) Handle(ctx context.Context, respWriter adapters.WSWriter, msgBytes []byte) {
	input := nostr.ParseMessage(msgBytes)
	if input == nil {
		err := errors.New("failed to parse input")
		notice := nostr.NoticeEnvelope(err.Error())
		log.Error(multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())
		return
	}
	var err error
	switch e := input.(type) {
	case *nostr.EventEnvelope:
		err = h.handleEvent(ctx, &model.Event{Event: e.Event})
		if err == nil {
			log.Error(h.writeResponse(respWriter, &nostr.OKEnvelope{
				EventID: e.ID,
				OK:      true,
				Reason:  "",
			}))
		}
	case *nostr.ReqEnvelope:
		err = h.handleReq(ctx, respWriter, &model.Subscription{Filters: e.Filters, SubscriptionID: e.SubscriptionID})
	case *nostr.CloseEnvelope:
		subID := string(*e)
		err = h.CancelSubscription(ctx, respWriter, &subID)
	default:
		if input != nil {
			err = errors.Errorf("unknown message type %v", input.Label())
		}
	}
	if err != nil {
		if e, isEvent := input.(*nostr.EventEnvelope); isEvent {
			err = errors.Wrapf(err, "failed to handle EVENT %+v", e)
			log.Error(multierror.Append(err, h.writeResponse(respWriter, &nostr.OKEnvelope{
				EventID: e.ID,
				OK:      false,
				Reason:  err.Error(),
			})).ErrorOrNil())
		}
		err = errors.Wrapf(err, "failed to handle %v %+v", input.Label(), input)
		notice := nostr.NoticeEnvelope(err.Error())
		log.Error(multierror.Append(err, h.writeResponse(respWriter, &notice)).ErrorOrNil())
	}
}

func (h *handler) writeResponse(respWriter adapters.WSWriter, envelope nostr.Envelope) error {
	b, err := envelope.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "failed to serialize %+v into json", envelope)
	}
	return respWriter.WriteMessage(int(ws.OpText), b)
}
