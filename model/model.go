package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	TagMarkerReply   string = "reply"
	TagMarkerRoot    string = "root"
	TagMarkerMention string = "mention"

	KindGenericRepost int = 16
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
	case nostr.KindTextNote:
		pTags := e.Tags.GetAll([]string{"p"})
		eTags := e.Tags.GetAll([]string{"e"})
		if len(eTags) > 0 {
			for _, tag := range eTags {
				if len(tag) < 2 {
					return errorx.Withf(ErrWrongEventParams, "wrong nip-10: no tag required param: %+v", e)
				}
				if len(tag) >= 3 {
					if tag[3] != TagMarkerRoot && tag[3] != TagMarkerReply && tag[3] != TagMarkerMention {
						return errorx.Withf(ErrWrongEventParams, "wrong nip-10: wrong tag marker param: %+v", e)
					}
				}
			}
		}
		if len(pTags) > 0 {
			if len(eTags) == 0 {
				return errorx.Withf(ErrWrongEventParams, "wrong nip-10: no e tags while p tag exist: %+v", e)
			}
			for _, tag := range pTags {
				if len(tag) == 1 {
					return errorx.Withf(ErrWrongEventParams, "wrong nip-10: p tag doesn't contain any pubkey who is involved in reply thread: %+v", e)
				}
			}
		}
	case nostr.KindRepost, KindGenericRepost:
		if !json.Valid([]byte(e.Content)) {
			return errorx.Withf(ErrWrongEventParams, "wrong nip-18: content field should be stringified json: %+v", e)
		}
		var parsedContent struct {
			ID     string `json:"id" example:"abcde"`
			Kind   int    `json:"kind" example:"1"`
			PubKey string `json:"pubkey" example:"pubkey"`
		}
		if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
			return errorx.Withf(ErrWrongEventParams, "wrong nip-18: wrong json fields for: %+v", e)
		}
		if e.Kind == nostr.KindRepost {
			if parsedContent.Kind != nostr.KindTextNote {
				return errorx.Withf(ErrWrongEventParams, "wrong nip-18: wrong kind of repost event: %+v", e)
			}
		} else if e.Kind == KindGenericRepost {
			kTag := e.Tags.GetFirst([]string{"k"})
			if kTag == nil || kTag.Value() != fmt.Sprint(parsedContent.Kind) {
				return errorx.Withf(ErrWrongEventParams, "wrong nip-18: wrong kind of reposted event: %+v", e)
			}
		}
		eTag := e.Tags.GetFirst([]string{"e"})
		if eTag == nil || len(*eTag) < 3 || eTag.Value() != parsedContent.ID {
			return errorx.Withf(ErrWrongEventParams, "wrong nip-18: repost must include e tag with id of the note and relay value: %+v", e)
		}
		pTag := e.Tags.GetFirst([]string{"p"})
		if pTag == nil || len(*pTag) < 2 || pTag.Value() != parsedContent.PubKey {
			return errorx.Withf(ErrWrongEventParams, "wrong nip-18: repost must include p tag with pubkey of the event being reposted: %+v", e)
		}
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
