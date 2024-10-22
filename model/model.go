// SPDX-License-Identifier: ice License 1.0

package model

import (
	"errors"

	"github.com/nbd-wtf/go-nostr"
)

type (
	TagMap       = nostr.TagMap
	Tag          = nostr.Tag
	Tags         = nostr.Tags
	Timestamp    = nostr.Timestamp
	Kind         = int
	Filter       = nostr.Filter
	Filters      = nostr.Filters
	Subscription struct {
		Filters Filters
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
	CustomIONKindAttestation = 10_100
)

const (
	CustomIONTagOnBehalfOf = "b"
)

const (
	CustomIONAttestationKindActive   = "active"
	CustomIONAttestationKindRevoked  = "revoked"
	CustomIONAttestationKindInactive = "inactive"
)
