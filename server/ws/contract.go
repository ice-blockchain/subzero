// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"errors"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Writer           = adapters.WSWriter
	Config           = config.Config
	EventBroadcaster interface {
		BroadcastNewEvents(ctx context.Context, events ...*model.Event) int
	}
	Handler interface {
		adapters.WSHandler
		EventBroadcaster
	}
	Server         = internal.Server
	RegisterRoutes = internal.RegisterRoutes
	Router         = internal.Router
)

const (
	connMetadataChallengeKey = "conn_auth_challenge" // Holds the challenge string for the connection.
	connMetadataAuthKey      = "conn_auth_data"      // Holds the authentication data (model.UserDataContext) for the connection.
)

var (
	WithWS = internal.WithWS
)

type (
	subscription struct {
		Source     *model.Subscription
		Writer     Writer
		MasterKeys []string
		Kinds      []model.Kind
	}
	handler struct {
		Subscriptions      *eventMatcherStorage
		RelayURL           string
		BroadcastPublicKey string
	}
)

var (
	errAuthRequired  = errors.New("auth-required: please authenticate first by sending AUTH message")
	errRelayReadOnly = errors.New("relay-is-read-only: read only")
	errDuplicate     = errors.New("duplicate: event with that data already exists")
)
