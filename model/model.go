package model

import (
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
)

type (
	Event struct {
		nostr.Event
	}
	Subscription struct {
		Filters nostr.Filters
	}
	EventReference interface {
		Filter() nostr.Filter
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
	ErrDuplicate = errorx.Errorf("duplicate")
)
