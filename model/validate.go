// SPDX-License-Identifier: ice License 1.0

package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
)

const (
	TagMarkerReply   string = "reply"
	TagMarkerRoot    string = "root"
	TagMarkerMention string = "mention"

	TagReportTypeNudity        string = "nudity"
	TagReportTypeMalware       string = "malware"
	TagReportTypeProfanity     string = "profanity"
	TagReportTypeIllegal       string = "illegal"
	TagReportTypeSpam          string = "spam"
	TagReportTypeImpersonation string = "impersonation"
	TagReportTypeOther         string = "other"

	RelayListReadMarker  = "read"
	RelayListWriteMarker = "write"

	UserGeneratedContentNamespace string = "ugc"
	ProfileBadgesIdentifier       string = "profile_badges"

	KindBadgeAward           int = 8
	KindGenericRepost        int = 16
	KindReactionToWebsite    int = 17
	KindReport               int = 1984
	KindLabeling             int = 1985
	KindBookmarks            int = 10003
	KindCommunities          int = 10004
	KindPublicChats          int = 10005
	KindBlockedRelays        int = 10006
	KindSearchRelay          int = 10007
	KindSimpleGroups         int = 10009
	KindInterests            int = 10015
	KindEmojis               int = 10030
	KindDMRelays             int = 10050
	KindGoodWikiAuthors      int = 10101
	KindGoodWikiRelays       int = 10102
	KindRelaySets            int = 30002
	KindBookmarksSets        int = 30003
	KindCurationSets1        int = 30004
	KindCurationSets2        int = 30005
	KindMuteSets             int = 30007
	KindInterestSets         int = 30015
	KindEmojiSets            int = 30030
	KindReleaseArtefactSets  int = 30063
	KindBlogPost             int = 30024
	KindVideo                int = 34235
	KindCommunityDefinitions int = 34550

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
		nostr.KindProfileMetadata:       {"e", "p", "a", "alt"},
		nostr.KindTextNote:              {"e", "p", "q", "l", "L"},
		nostr.KindFollowList:            {"p"},
		nostr.KindDeletion:              {"a", "e", "k"},
		nostr.KindRepost:                {"e", "p"},
		nostr.KindReaction:              {"e", "p", "a", "k"},
		KindBadgeAward:                  {"a", "p"},
		KindGenericRepost:               {"k", "e", "p"},
		KindReactionToWebsite:           {"r"},
		nostr.KindMuteList:              {"p", "t", "word", "e"},
		nostr.KindPinList:               {"e"},
		KindBookmarks:                   {"e", "a", "t", "r"},
		KindCommunities:                 {"a"},
		KindPublicChats:                 {"e"},
		KindBlockedRelays:               {"relay"},
		KindSearchRelay:                 {"relay"},
		KindSimpleGroups:                {"group"},
		KindInterests:                   {"t", "a"},
		KindEmojis:                      {"emoji", "a"},
		KindDMRelays:                    {"relay"},
		KindGoodWikiAuthors:             {"p"},
		KindGoodWikiRelays:              {"relay"},
		nostr.KindCategorizedPeopleList: {"p", "d", "title", "image", "description"},
		KindRelaySets:                   {"relay", "d", "title", "image", "description"},
		KindBookmarksSets:               {"e", "a", "t", "r", "d", "title", "image", "description"},
		KindCurationSets1:               {"a", "e", "d", "title", "image", "description"},
		KindCurationSets2:               {"a", "d", "title", "image", "description"},
		KindMuteSets:                    {"p", "d", "title", "image", "description"},
		KindInterestSets:                {"t", "d", "title", "image", "description"},
		KindEmojiSets:                   {"emoji", "d", "title", "image", "description"},
		KindReleaseArtefactSets:         {"e", "i", "version", "d", "title", "image", "description"},
		KindLabeling:                    {"L", "l", "e", "p", "a", "r", "t"},
		nostr.KindRelayListMetadata:     {"r"},
		nostr.KindProfileBadges:         {"d", "a", "e"},
		nostr.KindBadgeDefinition:       {"d", "name", "image", "description", "thumb"},
		nostr.KindArticle:               {"a", "d", "e", "t", "title", "image", "summary", "published_at"},
		KindBlogPost:                    {"a", "d", "e", "t", "title", "image", "summary", "published_at"},
	}
)

func (e *Event) Validate() error {
	if e.Kind < 0 || e.Kind > 65535 {
		return errors.New("wrong kind value")
	}
	if !areTagsSupported(e) {
		return errors.Wrapf(ErrUnsupportedTag, "unsupported tag for this event kind: %+v", e)
	}
	e.normalizeTags()
	switch e.Kind {
	case nostr.KindProfileMetadata:
		return validateKindProfileMetadataEvent(e)
	case nostr.KindTextNote:
		return validateKindTextNoteEvent(e)
	case nostr.KindDeletion:
		return validateKindDeletionEvent(e)
	case nostr.KindRepost, KindGenericRepost:
		return validateKindRepostEvent(e)
	case nostr.KindFollowList:
		if len(e.Tags) == 0 || len(e.Tags.GetAll([]string{"p"})) == 0 || e.Content != "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-02 params: %+v", e)
		}
		for _, tag := range e.Tags {
			if tag.Key() == "p" && tag.Value() == "" {
				return errors.Wrapf(ErrWrongEventParams, "nip-02 params, no required pubkey %+v", e)
			}
		}
	case nostr.KindReaction:
		return validateKindReactionEvent(e)
	case KindBadgeAward:
		return validateKindBadgeAwardEvent(e)
	case KindReactionToWebsite:
		if e.Content != "+" && e.Content != "-" && e.Content != "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
		}
		if rTag := e.Tags.GetFirst([]string{"r"}); rTag == nil || rTag.Value() == "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong r tag value: %+v", e)
		}
	case KindBookmarks:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindArticle) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindCommunities:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(KindCommunityDefinitions) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindInterests:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(KindInterestSets) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindEmojis:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(KindEmojiSets) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindBookmarksSets:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindArticle) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindCurationSets1:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindTextNote) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindCurationSets2:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(KindVideo) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case KindReport:
		return validateKindReportEvent(e)
	case KindLabeling:
		return validateKindLabelingEvent(e)
	case nostr.KindRelayListMetadata:
		return validateKindRelayListMetadataEvent(e)
	case nostr.KindProfileBadges:
		return validateKindProfileBadgesEvent(e)
	case nostr.KindBadgeDefinition:
		return validateKindBadgeDefinitionEvent(e)

	case nostr.KindArticle, KindBlogPost:
		if e.Content == "" || json.Valid([]byte(e.Content)) {
			return errors.Wrapf(ErrWrongEventParams, "nip-23: this kind should have text markdown content: %+v", e)
		}
	}

	return nil
}

func validateKindRelayListMetadataEvent(e *Event) error {
	rTags := e.Tags.GetAll([]string{"r"})
	if len(rTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-65, no required r tags: %+v", e)
	}
	if e.Content != "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-65, content is not used: %+v", e)
	}
	for _, tag := range rTags {
		if len(tag) < 2 || (len(tag) > 2 && (tag[2] != "" && tag[2] != RelayListReadMarker && tag[2] != RelayListWriteMarker)) {
			return errors.Wrapf(ErrWrongEventParams, "nip-65, wrong read/write marker for r tag: %+v", e)
		}
	}

	return nil
}

func validateKindBadgeDefinitionEvent(e *Event) error {
	if dTag := e.Tags.GetD(); dTag == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, no required d tag: %+v", e)
	}

	return nil
}

func validateKindBadgeAwardEvent(e *Event) error {
	if aTag := e.Tags.GetFirst([]string{"a"}); aTag == nil || aTag.Value() == "" ||
		len(strings.Split(aTag.Value(), ":")) != 3 || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindBadgeDefinition) {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, no required a tag: %+v", e)
	}
	if pTags := e.Tags.GetAll([]string{"p"}); len(pTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, no required p tags: %+v", e)
	}

	return nil
}

func validateKindProfileBadgesEvent(e *Event) error {
	if dTag := e.Tags.GetD(); dTag != ProfileBadgesIdentifier {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, no required d tag/wrong value: %+v", e)
	}
	aTags := e.Tags.GetAll([]string{"a"})
	for _, aTag := range aTags {
		if aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3 || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindBadgeDefinition) {
			return errors.Wrapf(ErrWrongEventParams, "nip-58, wrong a tag: %+v", e)
		}
	}
	if len(aTags) != len(e.Tags.GetAll([]string{"e"})) {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, e/a tag mismatch: %+v", e)
	}

	return nil
}

func validateKindDeletionEvent(e *Event) error {
	eTags := e.Tags.GetAll([]string{"e"})
	aTags := e.Tags.GetAll([]string{"a"})
	if len(eTags) == 0 && len(aTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-09, no required e/a tags: %+v", e)
	}
	if len(eTags) != 0 && len(eTags) != len(e.Tags.GetAll([]string{"k"})) {
		return errors.Wrapf(ErrWrongEventParams, "nip-09, deletion request should include k tag for the kind of each event being requested for deletion: %+v", e)
	}

	return nil
}

func validateKindReportEvent(e *Event) error {
	if err := validateLabelTags(e); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, wrong label tags: %+v", e)
	}
	pTag := e.Tags.GetFirst([]string{"p"})
	if pTag == nil || pTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, missing p tag: %+v", e)
	}
	eTag := e.Tags.GetFirst([]string{"e"})
	if eTag != nil && (len(*eTag) < 3 || !reportTypeSupported((*eTag)[2]) || len(*pTag) > 2) {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, wrong e tag report type value/wrong p tag report type value: %+v", e)
	}
	if eTag == nil && (len(*pTag) < 3 || !reportTypeSupported((*pTag)[2])) {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, wrong p tag report type value:%+v", e)
	}

	return nil
}

func reportTypeSupported(reportType string) bool {
	return reportType == "" || reportType == TagReportTypeNudity || reportType == TagReportTypeMalware ||
		reportType == TagReportTypeProfanity || reportType == TagReportTypeIllegal || reportType == TagReportTypeSpam ||
		reportType == TagReportTypeImpersonation || reportType == TagReportTypeOther
}

func validateKindLabelingEvent(e *Event) error {
	if e.Tags.GetFirst([]string{"e"}) == nil && e.Tags.GetFirst([]string{"p"}) == nil && e.Tags.GetFirst([]string{"a"}) == nil &&
		e.Tags.GetFirst([]string{"r"}) == nil && e.Tags.GetFirst([]string{"t"}) == nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, missing one of required tags: %+v", e)
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
		return errors.Wrapf(ErrWrongEventParams, "nip-32, wrong l: %+v", e)
	}
	if len(labelTag.Value()) > maxLabelSymbolLength {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, l tag should be shorter than %d symbols: %+v", maxLabelSymbolLength, e)
	}
	if labelNamespaceTag == nil && (*labelTag)[2] != UserGeneratedContentNamespace {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, empty L tag, namespace of l tag should be ugc: %+v", e)
	}
	if labelNamespaceTag != nil && (*labelTag)[2] != (*labelNamespaceTag)[1] {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, l -> L tag values mismatch: %+v", e)
	}

	return nil
}

func validateKindProfileMetadataEvent(e *Event) error {
	if !json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: content field should be stringified json: %+v", e)
	}
	var parsedContent ProfileMetadataContent
	if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-01,nip-24: wrong json fields for: %+v", e)
	}
	if parsedContent.Name == "" || parsedContent.About == "" || parsedContent.Picture == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: there are no required content fields: %+v", e)
	}

	return nil
}

func validateKindTextNoteEvent(e *Event) error {
	if json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: content field should be plain text: %+v", e)
	}
	if err := validateLabelTags(e); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-32: label tags are invalid for event: %+v", e)
	}
	pTags := e.Tags.GetAll([]string{"p"})
	eTags := e.Tags.GetAll([]string{"e"})
	if len(eTags) > 0 {
		for _, tag := range eTags {
			if len(tag) < 2 {
				return errors.Wrapf(ErrWrongEventParams, "nip-10: no tag required param: %+v", e)
			}
			if len(tag) >= 3 {
				if tag[3] != TagMarkerRoot && tag[3] != TagMarkerReply && tag[3] != TagMarkerMention {
					return errors.Wrapf(ErrWrongEventParams, "nip-10: wrong tag marker param: %+v", e)
				}
			}
		}
	}
	if len(pTags) > 0 {
		if len(eTags) == 0 {
			return errors.Wrapf(ErrWrongEventParams, "wrong nip-10: no e tags while p tag exist: %+v", e)
		}
		for _, tag := range pTags {
			if len(tag) == 1 {
				return errors.Wrapf(ErrWrongEventParams, "nip-10: p tag doesn't contain any pubkey who is involved in reply thread: %+v", e)
			}
		}
	}

	return nil
}
func validateKindRepostEvent(e *Event) error {
	if !json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: content field should be stringified json: %+v", e)
	}
	var parsedContent RepostContent
	if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong json fields for: %+v", e)
	}
	if e.Kind == nostr.KindRepost {
		if parsedContent.Kind != nostr.KindTextNote {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong kind of repost event: %+v", e)
		}
	} else {
		if kTag := e.Tags.GetFirst([]string{"k"}); kTag == nil || kTag.Value() != fmt.Sprint(parsedContent.Kind) {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong kind of reposted event: %+v", e)
		}
	}
	if eTag := e.Tags.GetFirst([]string{"e"}); eTag == nil || len(*eTag) < 3 || eTag.Value() != parsedContent.ID {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include e tag with id of the note and relay value: %+v", e)
	}
	if pTag := e.Tags.GetFirst([]string{"p"}); pTag == nil || len(*pTag) < 2 || pTag.Value() != parsedContent.PubKey {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include p tag with pubkey of the event being reposted: %+v", e)
	}

	return nil
}

func validateKindReactionEvent(e *Event) error {
	if e.Content != "+" && e.Content != "-" && e.Content != "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
	}
	if eTag := e.Tags.GetLast([]string{"e"}); eTag == nil || eTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong e tag value: %+v", e)
	}
	if pTag := e.Tags.GetLast([]string{"p"}); pTag == nil || pTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong p tag value: %+v", e)
	}
	if kTag := e.Tags.GetFirst([]string{"k"}); kTag != nil && kTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong k tag value: %+v", e)
	}
	if aTag := e.Tags.GetFirst([]string{"a"}); aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong a tag value: %+v", e)
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
		if tag.Key() == "nonce" || tag.Key() == "expiration" {
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
