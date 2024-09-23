package model

import (
	"context"
	"errors"
	"log"

	"github.com/nbd-wtf/go-nostr"
	nip13 "github.com/nbd-wtf/go-nostr/nip13"
)

type (
	Filter    = nostr.Filter
	Filters   = nostr.Filters
	TagMap    = nostr.TagMap
	Tag       = nostr.Tag
	Tags      = nostr.Tags
	Timestamp = nostr.Timestamp
	Kind      = int

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
		PubKey string
		DTag   string
		Kind   int
	}
	PlainEventReference struct {
		EventIDs []string
	}
)

var (
	ErrDuplicate = errors.New("duplicate")
)

func (e *Event) CheckNIP13Difficulty(minLeadingZeroBits int) error {
	if minLeadingZeroBits == 0 {
		return nil
	}
	if err := nip13.Check(e.GetID(), minLeadingZeroBits); err != nil {
		log.Printf("difficulty: %v < %v, id:%v", nip13.Difficulty(e.GetID()), minLeadingZeroBits, e.GetID())

		return err
	}

	return nil
}

func (e *Event) GenerateNIP13(ctx context.Context, minLeadingZeroBits int) error {
	if minLeadingZeroBits == 0 {
		return nil
	}
	tag, err := nip13.DoWork(ctx, e.Event, minLeadingZeroBits)
	if err != nil {
		log.Printf("can't do mining by the provided difficulty:%v", minLeadingZeroBits)

		return err
	}
	e.Tags = append(e.Tags, tag)

	return nil
}
