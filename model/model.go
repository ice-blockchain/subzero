package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

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
	KindBlogPost      int = 30024
)

type (
	Filter    = nostr.Filter
	Filters   = nostr.Filters
	TagMap    = nostr.TagMap
	Tag       = nostr.Tag
	Tags      = nostr.Tags
	Timestamp = nostr.Timestamp
	EventType = string
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

	ProfileMetadataContent struct {
		Name        string `json:"name" example:"username"`
		About       string `json:"about" example:"about"`
		Picture     string `json:"picture" example:"https://example.com/pic.jpg"`
		DisplayName string `json:"display_name" example:"John Deer"`
		Website     string `json:"website" example:"https://ice.io"`
		Banner      string `json:"banner" example:"https://example.com/banner.jpg"`
		Bot         bool   `json:"bot" example:"false"`
	}

	RepostContent struct {
		ID     string `json:"id" example:"abcde"`
		PubKey string `json:"pubkey" example:"pubkey"`
		Kind   int    `json:"kind" example:"1"`
	}
)

var (
	ErrDuplicate        = errors.New("duplicate")
	ErrWrongEventParams = errors.New("wrong event params")
	ErrUnsupportedTag   = errors.New("unsupported tag")

	KindSupportedTags = map[Kind][]string{
		nostr.KindProfileMetadata: {"e", "p", "a", "alt"},
		nostr.KindTextNote:        {"e", "p", "q"},
		nostr.KindContactList:     {"p"},
		nostr.KindDeletion:        {"a", "e", "k"},
		nostr.KindRepost:          {"e", "p"},
		KindGenericRepost:         {"k", "e", "p"},
		nostr.KindArticle:         {"a", "d", "e", "t", "title", "image", "summary", "published_at"},
		KindBlogPost:              {"a", "d", "e", "t", "title", "image", "summary", "published_at"},
	}
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
	if !areTagsSupported(e) {
		return errorx.Withf(ErrUnsupportedTag, "unsupported tag for this event kind: %+v", e)
	}
	e.normalizeTags()
	switch e.Kind {
	case nostr.KindProfileMetadata:
		if !json.Valid([]byte(e.Content)) {
			return errorx.Withf(ErrWrongEventParams, "nip-01: content field should be stringified json: %+v", e)
		}
		var parsedContent ProfileMetadataContent
		if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
			return errorx.Withf(ErrWrongEventParams, "nip-01,nip-24: wrong json fields for: %+v", e)
		}
		if parsedContent.Name == "" || parsedContent.About == "" || parsedContent.Picture == "" {
			return errorx.Withf(ErrWrongEventParams, "nip-01: there are no required content fields: %+v", e)
		}
	case nostr.KindTextNote:
		if json.Valid([]byte(e.Content)) {
			return errorx.Withf(ErrWrongEventParams, "nip-01: content field should be plain text: %+v", e)
		}
		pTags := e.Tags.GetAll([]string{"p"})
		eTags := e.Tags.GetAll([]string{"e"})
		if len(eTags) > 0 {
			for _, tag := range eTags {
				if len(tag) < 2 {
					return errorx.Withf(ErrWrongEventParams, "nip-10: no tag required param: %+v", e)
				}
				if len(tag) >= 3 {
					if tag[3] != TagMarkerRoot && tag[3] != TagMarkerReply && tag[3] != TagMarkerMention {
						return errorx.Withf(ErrWrongEventParams, "nip-10: wrong tag marker param: %+v", e)
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
					return errorx.Withf(ErrWrongEventParams, "nip-10: p tag doesn't contain any pubkey who is involved in reply thread: %+v", e)
				}
			}
		}
	case nostr.KindRepost, KindGenericRepost:
		if !json.Valid([]byte(e.Content)) {
			return errorx.Withf(ErrWrongEventParams, "nip-18: content field should be stringified json: %+v", e)
		}
		var parsedContent RepostContent
		if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
			return errorx.Withf(ErrWrongEventParams, "nip-18: wrong json fields for: %+v", e)
		}
		if e.Kind == nostr.KindRepost {
			if parsedContent.Kind != nostr.KindTextNote {
				return errorx.Withf(ErrWrongEventParams, "nip-18: wrong kind of repost event: %+v", e)
			}
		} else {
			kTag := e.Tags.GetFirst([]string{"k"})
			if kTag == nil || kTag.Value() != fmt.Sprint(parsedContent.Kind) {
				return errorx.Withf(ErrWrongEventParams, "nip-18: wrong kind of reposted event: %+v", e)
			}
		}
		eTag := e.Tags.GetFirst([]string{"e"})
		if eTag == nil || len(*eTag) < 3 || eTag.Value() != parsedContent.ID {
			return errorx.Withf(ErrWrongEventParams, "nip-18: repost must include e tag with id of the note and relay value: %+v", e)
		}
		pTag := e.Tags.GetFirst([]string{"p"})
		if pTag == nil || len(*pTag) < 2 || pTag.Value() != parsedContent.PubKey {
			return errorx.Withf(ErrWrongEventParams, "nip-18: repost must include p tag with pubkey of the event being reposted: %+v", e)
		}
	case nostr.KindContactList:
		if len(e.Tags) == 0 || len(e.Tags.GetAll([]string{"p"})) == 0 || e.Content != "" {
			return errorx.Withf(ErrWrongEventParams, "nip-02 params: %+v", e)
		}
		for _, tag := range e.Tags {
			if tag.Key() == "p" && tag.Value() == "" {
				return errorx.Withf(ErrWrongEventParams, "nip-02 params, no required pubkey %+v", e)
			}
		}
	case nostr.KindArticle, KindBlogPost:
		if e.Content == "" || json.Valid([]byte(e.Content)) {
			return errorx.Withf(ErrWrongEventParams, "nip-23: this kind should have text markdown content: %+v", e)
		}
	}

	return nil
}

func areTagsSupported(e *Event) bool {
	supportedTags, ok := KindSupportedTags[e.Kind]
	if !ok {
		return true
	}
next:
	for _, tag := range e.Tags {
		if tag.Key() == "nonce" {
			continue next
		}
		for _, supportedTag := range supportedTags {
			if tag.Key() == supportedTag {
				continue next
			}
		}

		return false
	}

	return true
}

func (e *Event) normalizeTags() {
	for _, tag := range e.Tags {
		switch tag.Key() {
		case "t":
			if len(tag) > 1 {
				tag[1] = strings.ToLower(tag.Value()) // NIP-24.
			}
		}
	}
}
