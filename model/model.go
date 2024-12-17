// SPDX-License-Identifier: ice License 1.0

package model

import (
	"errors"

	"github.com/nbd-wtf/go-nostr"
)

type (
	Tag          = nostr.Tag
	Tags         = nostr.Tags
	TagMap       = nostr.TagMap
	TagValues    = nostr.TagValues
	Timestamp    = nostr.Timestamp
	Filter       = nostr.Filter
	Filters      = nostr.Filters
	Kind         = int
	Subscription struct {
		SubscriptionID string
		Filters        Filters
	}
	EventReference interface {
		Filter() Filter
	}
	ReplaceableEventReference struct {
		PubKey string
		DTag   string
		Kind   int
	}
	PlainEventReference struct {
		EventIDs []string
	}
)

var (
	ErrUnsupportedAlg       = errors.New("unsupported signature/key algorithm combination")
	ErrOnBehalfAccessDenied = errors.New("on-behalf access denied")
)

const (
	CustomIONKindAttestation       = 10_100
	CustomIONKindRelayListMetadata = 20_002

	KindDVMCountResponse = 6400
)

const (
	CustomIONTagOnBehalfOf = "b"
)

const (
	CustomIONAttestationKindActive   = "active"
	CustomIONAttestationKindRevoked  = "revoked"
	CustomIONAttestationKindInactive = "inactive"
)
