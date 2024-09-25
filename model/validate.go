package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
)

const (
	TagMarkerReply   string = "reply"
	TagMarkerRoot    string = "root"
	TagMarkerMention string = "mention"

	UserGeneratedContentNamespace string = "ugc"

	KindGenericRepost     int = 16
	KindReactionToWebsite int = 17
	KindLabeling          int = 1985
	KindBlogPost          int = 30024

	maxLabelSymbolLength int = 100
)

type (
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
	ErrWrongEventParams = errors.New("wrong event params")
	ErrUnsupportedTag   = errors.New("unsupported tag")

	KindSupportedTags = map[Kind][]string{
		nostr.KindProfileMetadata: {"e", "p", "a", "alt"},
		nostr.KindTextNote:        {"e", "p", "q", "l", "L"},
		nostr.KindFollowList:      {"p"},
		nostr.KindDeletion:        {"a", "e", "k"},
		nostr.KindRepost:          {"e", "p"},
		nostr.KindReaction:        {"e", "p", "a", "k"},
		KindReactionToWebsite:     {"r"},
		KindGenericRepost:         {"k", "e", "p"},
		KindLabeling:              {"L", "l", "e", "p", "a", "r", "t"},
		nostr.KindArticle:         {"a", "d", "e", "t", "title", "image", "summary", "published_at"},
		KindBlogPost:              {"a", "d", "e", "t", "title", "image", "summary", "published_at"},
	}
)

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
		return validateKindProfileMetadataEvent(e)
	case nostr.KindTextNote:
		return validateKindTextNoteEvent(e)
	case nostr.KindRepost, KindGenericRepost:
		return validateKindRepostEvent(e)
	case nostr.KindFollowList:
		if len(e.Tags) == 0 || len(e.Tags.GetAll([]string{"p"})) == 0 || e.Content != "" {
			return errorx.Withf(ErrWrongEventParams, "nip-02 params: %+v", e)
		}
		for _, tag := range e.Tags {
			if tag.Key() == "p" && tag.Value() == "" {
				return errorx.Withf(ErrWrongEventParams, "nip-02 params, no required pubkey %+v", e)
			}
		}
	case nostr.KindReaction:
		return validateKindReactionEvent(e)
	case KindReactionToWebsite:
		if e.Content != "+" && e.Content != "-" && e.Content != "" {
			return errorx.Withf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
		}
		if rTag := e.Tags.GetFirst([]string{"r"}); rTag == nil || rTag.Value() == "" {
			return errorx.Withf(ErrWrongEventParams, "nip-25, wrong r tag value: %+v", e)
		}
	case KindLabeling:
		return validateKindLabelingEvent(e)
	case nostr.KindArticle, KindBlogPost:
		if e.Content == "" || json.Valid([]byte(e.Content)) {
			return errorx.Withf(ErrWrongEventParams, "nip-23: this kind should have text markdown content: %+v", e)
		}
	}

	return nil
}

func validateKindLabelingEvent(e *Event) error {
	if e.Tags.GetFirst([]string{"e"}) == nil && e.Tags.GetFirst([]string{"p"}) == nil && e.Tags.GetFirst([]string{"a"}) == nil &&
		e.Tags.GetFirst([]string{"r"}) == nil && e.Tags.GetFirst([]string{"t"}) == nil {
		return errorx.Withf(ErrWrongEventParams, "nip-32, missing one of required tags: %+v", e)
	}

	return validateLabelTags(e)
}

func validateLabelTags(e *Event) error {
	labelTag := e.Tags.GetFirst([]string{"l"})
	labelNamespaceTag := e.Tags.GetFirst([]string{"L"})
	if labelTag == nil && labelNamespaceTag == nil && e.Kind != KindLabeling {
		return nil
	}
	if labelTag == nil || len(*labelTag) < 3 {
		return errorx.Withf(ErrWrongEventParams, "nip-32, wrong l: %+v", e)
	}
	if len(labelTag.Value()) > maxLabelSymbolLength {
		return errorx.Withf(ErrWrongEventParams, "nip-32, l tag should be shorter than %d symbols: %+v", maxLabelSymbolLength, e)
	}
	if labelNamespaceTag == nil && (*labelTag)[2] != UserGeneratedContentNamespace {
		return errorx.Withf(ErrWrongEventParams, "nip-32, empty L tag, namespace of l tag should be ugc: %+v", e)
	}
	if labelNamespaceTag != nil && (*labelTag)[2] != (*labelNamespaceTag)[1] {
		return errorx.Withf(ErrWrongEventParams, "nip-32, l -> L tag values mismatch: %+v", e)
	}

	return nil
}

func validateKindProfileMetadataEvent(e *Event) error {
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

	return nil
}

func validateKindTextNoteEvent(e *Event) error {
	if json.Valid([]byte(e.Content)) {
		return errorx.Withf(ErrWrongEventParams, "nip-01: content field should be plain text: %+v", e)
	}
	if err := validateLabelTags(e); err != nil {
		return errorx.Withf(ErrWrongEventParams, "nip-32: label tags are invalid for event: %+v", e)
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

	return nil
}
func validateKindRepostEvent(e *Event) error {
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
		if kTag := e.Tags.GetFirst([]string{"k"}); kTag == nil || kTag.Value() != fmt.Sprint(parsedContent.Kind) {
			return errorx.Withf(ErrWrongEventParams, "nip-18: wrong kind of reposted event: %+v", e)
		}
	}
	if eTag := e.Tags.GetFirst([]string{"e"}); eTag == nil || len(*eTag) < 3 || eTag.Value() != parsedContent.ID {
		return errorx.Withf(ErrWrongEventParams, "nip-18: repost must include e tag with id of the note and relay value: %+v", e)
	}
	if pTag := e.Tags.GetFirst([]string{"p"}); pTag == nil || len(*pTag) < 2 || pTag.Value() != parsedContent.PubKey {
		return errorx.Withf(ErrWrongEventParams, "nip-18: repost must include p tag with pubkey of the event being reposted: %+v", e)
	}

	return nil
}

func validateKindReactionEvent(e *Event) error {
	if e.Content != "+" && e.Content != "-" && e.Content != "" {
		return errorx.Withf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
	}
	if eTag := e.Tags.GetLast([]string{"e"}); eTag == nil || eTag.Value() == "" {
		return errorx.Withf(ErrWrongEventParams, "nip-25, wrong e tag value: %+v", e)
	}
	if pTag := e.Tags.GetLast([]string{"p"}); pTag == nil || pTag.Value() == "" {
		return errorx.Withf(ErrWrongEventParams, "nip-25, wrong p tag value: %+v", e)
	}
	if kTag := e.Tags.GetFirst([]string{"k"}); kTag != nil && kTag.Value() == "" {
		return errorx.Withf(ErrWrongEventParams, "nip-25, wrong k tag value: %+v", e)
	}
	if aTag := e.Tags.GetFirst([]string{"a"}); aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) {
		return errorx.Withf(ErrWrongEventParams, "nip-25, wrong a tag value: %+v", e)
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
