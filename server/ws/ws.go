// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/tls"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/validation"
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
	wsEventListener          EventListener
	wsSubscriptionListeners  []EventGetter
	wsBroadcastEventListener EventListener
	reqMustAuth              ReqMustAuthenticate
	eventMustAuth            EventAuthenticate
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

func RegisterWSBroadcastEventListener(cb EventListener) {
	wsBroadcastEventListener = cb
}

func NewHandler(relayURL, broadcastPublicKey string) Handler {
	return newHandler(relayURL, broadcastPublicKey)
}

func New(cfg *Config, routes internal.RegisterRoutes) Server {
	return internal.NewWSServer(routes, cfg)
}

func newHandler(relayURL, broadcastPublicKey string) *handler {
	numShards := uint32(runtime.NumCPU() * 2)

	return &handler{
		Subscriptions:      newEventMatcherStorage(numShards),
		ConnAuth:           xsync.NewMap[Writer, connAuthData](),
		RelayURL:           relayURL,
		BroadcastPublicKey: broadcastPublicKey,
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
					log.Warn().
						Str("context", "WEBSOCKET").
						Int("close_code", int(closed.Code)).
						Msg("unexpected close error")
				}
			} else if !errors.Is(err, io.EOF) {
				log.Warn().
					Str("context", "WEBSOCKET").
					Int("close_code", int(closed.Code)).
					Msg("unexpected close error")
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

func (h *handler) logOperation(respWriter adapters.WSWriter, duration time.Duration, msgf string, args ...any) {
	if duration < operationLogThreshold {
		return
	}

	logger := log.Warn().
		Str("context", "WEBSOCKET").
		Dur("duration", duration)

	if v, ok := h.ConnAuth.Load(respWriter); ok && v.Authenticated {
		logger = logger.Str("master_pubkey", v.MasterPublicKey)
		if v.UserAgent != "" {
			logger = logger.Str("user_agent", v.UserAgent)
		}
	}

	logger.Msgf(msgf, args...)
}

func (h *handler) Handle(ctx context.Context, respWriter adapters.WSWriter, msgBytes []byte) {
	input, err := nostr.ParseMessage(msgBytes, new(model.BroadcastEnvelope))
	if err != nil {
		notice := nostr.NoticeEnvelope(err.Error())
		log.Error().Err(errors.Join(err, h.writeResponse(ctx, respWriter, &notice))).Msg("failed to parse message")

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
		if err != nil && !errors.Is(err, errAuthRequired) {
			log.Error().Err(err).Str("events", model.Events(events).String()).Msg("cannot process events")
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
				log.Error().Err(wErr).Int("event_index", i).Msg("write event response")

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
		subscriptionID := (*string)(e)
		h.unlinkSubscription(respWriter, subscriptionID)
		h.logOperation(respWriter, time.Since(start), "req: close: %v", subscriptionID)
	case *model.BroadcastEnvelope:
		h.handleBroadcast(h.populateContext(ctx, respWriter), e)
		h.logOperation(respWriter, time.Since(start), "broadcast")
	default:
		err = errors.Errorf("unknown message type %v", input.Label())
	}

	if err != nil {
		err = errors.Wrapf(err, "error: failed to handle %v %+v", input.Label(), input)
		notice := nostr.NoticeEnvelope(err.Error())
		log.Error().Err(errors.Join(err, h.writeResponse(ctx, respWriter, &notice))).Msg("failed to handle message")
	}
}

func (h *handler) handleBroadcast(ctx context.Context, e *model.BroadcastEnvelope) {
	if data := model.GetUserDataFromContext(ctx); !data.Authenticated {
		log.Warn().Str("relay", e.Relay).Msg("ignoring broadcast from unauthenticated relay")
		return
	} else if data.PublicKey != h.BroadcastPublicKey {
		log.Warn().
			Str("relay", e.Relay).
			Str("got_public_key", data.PublicKey).
			Str("expected_public_key", h.BroadcastPublicKey).
			Msg("ignoring broadcast from relay due to public key mismatch")
		return
	}

	if err := validation.Validate(ctx, model.Events(e.Events),
		validation.RuleWithBroadcastMode(),
		validation.RuleWithSkipProfileMetadataProofEventsVerify(),
		validation.RuleWithSkipRootContentNFTCollectionsValidation(),
	); err != nil {
		log.Error().Err(err).Str("relay", e.Relay).Msg("validation failed for broadcast")
		return
	}

	if wsBroadcastEventListener != nil {
		wsBroadcastEventListener(ctx, e.Events...)
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
			log.Panic().Err(err)
		}
	} else {
		cert, err = tls.X509KeyPair([]byte(certOrFileName), []byte(keyOrFileName))
		if err != nil {
			log.Panic().Err(err)
		}
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}
