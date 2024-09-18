package model

import (
	"context"
	"errors"
	"log"

	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
	nip13 "github.com/nbd-wtf/go-nostr/nip13"
)

const (
	RegularEventType                  EventType = "regular"
	ReplaceableEventType              EventType = "replaceable"
	EphemeralEventType                EventType = "ephemeral"
	ParameterizedReplaceableEventType EventType = "parameterized_replaceable"
)

type (
	Filter    = nostr.Filter
	Filters   = nostr.Filters
	TagMap    = nostr.TagMap
	Tag       = nostr.Tag
	Tags      = nostr.Tags
	Timestamp = nostr.Timestamp
	EventType = string

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
	ErrDuplicate        = errors.New("duplicate")
	ErrWrongEventParams = errors.New("wrong event params")
)

func (e *Event) EventType() EventType {
	if (1000 <= e.Kind && e.Kind < 10000) || (4 <= e.Kind && e.Kind < 45) || e.Kind == 1 || e.Kind == 2 {
		return RegularEventType
	} else if (10000 <= e.Kind && e.Kind < 20000) || e.Kind == 0 || e.Kind == 3 {
		return ReplaceableEventType
	} else if 20000 <= e.Kind && e.Kind < 30000 {
		return EphemeralEventType
	} else if 30000 <= e.Kind && e.Kind < 40000 {
		return ParameterizedReplaceableEventType
	}
	log.Printf("wrong kind: %v", e.Kind)

	return ""
}

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

func (e *Event) Validate() error {
	if e.Kind < 0 || e.Kind > 65535 {
		return errorx.New("wrong kind value")
	}
	switch e.Kind {
	case nostr.KindContactList:
		if len(e.Tags) == 0 || len(e.Tags.GetAll([]string{"p"})) == 0 || e.Content != "" {
			return errorx.Withf(ErrWrongEventParams, "wrong nip-02 params: %+v", e)
		}
		for _, tag := range e.Tags {
			if tag.Key() == "p" && tag.Value() == "" {
				return errorx.Withf(ErrWrongEventParams, "wrong nip-02 params, no required pubkey %+v", e)
			}
		}
	}

	return nil
}
