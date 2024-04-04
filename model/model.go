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
)

var (
	ErrDuplicate = errorx.Errorf("duplicate")
)
