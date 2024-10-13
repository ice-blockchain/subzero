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
	ErrDuplicate      = errors.New("duplicate")
	ErrUnsupportedAlg = errors.New("unsupported signature/key algorithm combination")
)

const (
	IceKindAttestation = 10_100
)

const (
	IceTagOnBehalfOf  = "b"
	IceTagAttestation = "p"
)

const (
	IceAttestationKindActive   = "active"
	IceAttestationKindRevoked  = "revoked"
	IceAttestationKindInactive = "inactive"
)
