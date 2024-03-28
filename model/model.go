package model

import "github.com/nbd-wtf/go-nostr"

type (
	Event struct {
		nostr.Event
	}
	Subscription struct {
		Filters nostr.Filters
	}
)
