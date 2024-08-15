package model

import (
	"errors"

	"github.com/nbd-wtf/go-nostr"
)

type (
	Filter    = nostr.Filter
	Filters   = nostr.Filters
	TagMap    = nostr.TagMap
	Tag       = nostr.Tag
	Tags      = nostr.Tags
	Timestamp = nostr.Timestamp

	Event struct {
		nostr.Event
	}
	Subscription struct {
		Filters Filters
	}
	EventReference interface {
		Filter() Filter
	}
	ReplaceableEventReference struct {
		Kind   int
		PubKey string
		dTag   string
	}
	PlainEventReference struct {
		EventIDs []string
	}
)

var (
	ErrDuplicate = errors.New("duplicate")
)
