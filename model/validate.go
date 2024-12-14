// SPDX-License-Identifier: ice License 1.0

package model

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
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

	maxLabelSymbolLength int = 100

	KindJobTextExtraction            = 5000
	KindJobSummarization             = 5001
	KindJobTranslation               = 5002
	KindJobTextGeneration            = 5050
	KindJobImageGeneration           = 5100
	KindJobVideoConversion           = 5200
	KindJobVideoTranslation          = 5201
	KindJobImageToVideoConversion    = 5202
	KindJobTextToSpeechGeneration    = 5250
	KindJobNostrContentDiscovery     = 5300
	KindJobNostrPeopleDiscovery      = 5301
	KindJobNostrContentSearch        = 5302
	KindJobNostrPeopleSearch         = 5303
	KindJobNostrEventCount           = 5400
	KindJobMalwareScanning           = 5500
	KindJobNostrEventTimeStamping    = 5900
	KindJobOpReturnCreation          = 5901
	KindJobNostrEventPublishSchedule = 5905

	JobFeedbackStatusPaymentRequired JobFeedbackStatus = "payment-required"
	JobFeedbackStatusProcessing      JobFeedbackStatus = "processing"
	JobFeedbackStatusError           JobFeedbackStatus = "error"
	JobFeedbackStatusSuccess         JobFeedbackStatus = "success"
	JobFeedbackStatusPartial         JobFeedbackStatus = "partial"
)

type (
	JobFeedbackStatus      = string
	ProfileMetadataContent struct {
		Name        string `json:"name" example:"username"`
		About       string `json:"about" example:"about"`
		Picture     string `json:"picture" example:"https://example.com/pic.jpg"`
		DisplayName string `json:"display_name" example:"John Deer"`
		Website     string `json:"website" example:"https://ice.io"`
		Banner      string `json:"banner" example:"https://example.com/banner.jpg"`
		Bot         bool   `json:"bot" example:"false"`
	}
)

var (
	ErrWrongEventParams = errors.New("wrong event params")
	ErrUnsupportedTag   = errors.New("unsupported tag")
	ErrUnsupportedJob   = errors.New("unsupported job")
	ErrUnsupportedKind  = errors.New("unsupported kind")
	CommongTags         = tagsTable("nonce", "expiration", "imeta", CustomIONTagOnBehalfOf)
	KindSupportedTags   = map[Kind]map[string]struct{}{
		nostr.KindProfileMetadata:       tagsTable("e", "p", "a", "alt"),
		nostr.KindTextNote:              tagsTable("e", "p", "q", "l", "L"),
		nostr.KindFollowList:            tagsTable("p"),
		nostr.KindDeletion:              tagsTable("a", "e", "k"),
		nostr.KindRepost:                tagsTable("e", "p"),
		nostr.KindReaction:              tagsTable("e", "p", "a", "k"),
		nostr.KindBadgeAward:            tagsTable("a", "p"),
		nostr.KindGenericRepost:         tagsTable("k", "e", "p"),
		nostr.KindReactionToWebsite:     tagsTable("r"),
		nostr.KindMuteList:              tagsTable("p", "t", "word", "e"),
		nostr.KindPinList:               tagsTable("e"),
		nostr.KindBookmarkList:          tagsTable("e", "a", "t", "r"),
		nostr.KindCommunityList:         tagsTable("a"),
		nostr.KindPublicChatList:        tagsTable("e"),
		nostr.KindBlockedRelayList:      tagsTable("relay"),
		nostr.KindSearchRelayList:       tagsTable("relay"),
		nostr.KindSimpleGroupList:       tagsTable("group"),
		nostr.KindInterestList:          tagsTable("t", "a"),
		nostr.KindEmojiList:             tagsTable("emoji", "a"),
		nostr.KindDMRelayList:           tagsTable("relay"),
		nostr.KindGoodWikiAuthorList:    tagsTable("p"),
		nostr.KindGoodWikiRelayList:     tagsTable("relay"),
		nostr.KindCategorizedPeopleList: tagsTable("p", "d", "title", "image", "description"),
		nostr.KindRelaySets:             tagsTable("relay", "d", "title", "image", "description"),
		nostr.KindBookmarkSets:          tagsTable("e", "a", "t", "r", "d", "title", "image", "description"),
		nostr.KindCuratedSets:           tagsTable("a", "e", "d", "title", "image", "description"),
		nostr.KindCuratedVideoSets:      tagsTable("a", "d", "title", "image", "description"),
		nostr.KindMuteSets:              tagsTable("p", "d", "title", "image", "description"),
		nostr.KindInterestSets:          tagsTable("t", "d", "title", "image", "description"),
		nostr.KindEmojiSets:             tagsTable("emoji", "d", "title", "image", "description"),
		nostr.KindReleaseArtifactSets:   tagsTable("e", "i", "version", "d", "title", "image", "description"),
		nostr.KindLabel:                 tagsTable("L", "l", "e", "p", "a", "r", "t"),
		nostr.KindRelayListMetadata:     tagsTable("r"),
		nostr.KindProfileBadges:         tagsTable("d", "a", "e"),
		nostr.KindBadgeDefinition:       tagsTable("d", "name", "image", "description", "thumb"),
		nostr.KindArticle:               tagsTable("a", "d", "e", "t", "title", "image", "summary", "published_at"),
		nostr.KindDraftArticle:          tagsTable("a", "d", "e", "t", "title", "image", "summary", "published_at"),

		// --- Jobs
		KindJobTextExtraction:            tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobSummarization:             tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobTranslation:               tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobTextGeneration:            tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobImageGeneration:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobVideoConversion:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobVideoTranslation:          tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobImageToVideoConversion:    tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobTextToSpeechGeneration:    tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrContentDiscovery:     tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrPeopleDiscovery:      tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrContentSearch:        tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrPeopleSearch:         tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrEventCount:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobMalwareScanning:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrEventTimeStamping:    tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobOpReturnCreation:          tagsTable("i", "output", "param", "bid", "relays", "p"),
		KindJobNostrEventPublishSchedule: tagsTable("i", "output", "param", "bid", "relays", "p", "encrypted"),
		nostr.KindJobFeedback:            tagsTable("status", "amount", "e", "p"),
	}

	// Tag name -> required.
	SupportedIMetaKeys = map[string]bool{
		"url":      true,
		"m":        true,
		"x":        false,
		"ox":       false,
		"size":     false,
		"dim":      false,
		"magnet":   false,
		"i":        true,
		"blurhash": false,
		"thumb":    false,
		"image":    false,
		"summary":  false,
		"alt":      true,
		"fallback": false,
	}

	JobFeedbackStatusValues = map[string]struct{}{
		JobFeedbackStatusPaymentRequired: {},
		JobFeedbackStatusProcessing:      {},
		JobFeedbackStatusError:           {},
		JobFeedbackStatusSuccess:         {},
		JobFeedbackStatusPartial:         {},
	}
)

func (e *Event) Validate() error {
	if e.Kind < 0 || e.Kind > 65535 {
		return errors.Wrapf(ErrUnsupportedKind, "kind: %d", e.Kind)
	}
	if err := validateEventTags(e); err != nil {
		return errors.Wrapf(err, "event: %+v", e)
	}
	e.normalizeTags()
	switch e.Kind {
	case nostr.KindProfileMetadata:
		return validateKindProfileMetadataEvent(e)
	case nostr.KindTextNote:
		return validateKindTextNoteEvent(e)
	case nostr.KindDeletion:
		return validateKindDeletionEvent(e)
	case nostr.KindRepost, nostr.KindGenericRepost:
		return validateKindRepostEvent(e)
	case nostr.KindFollowList:
		for _, tag := range e.Tags {
			if tag.Key() == "p" && tag.Value() == "" {
				return errors.Wrapf(ErrWrongEventParams, "nip-02 params, no required pubkey %+v", e)
			}
		}
	case nostr.KindReaction:
		return validateKindReactionEvent(e)
	case nostr.KindBadgeAward:
		return validateKindBadgeAwardEvent(e)
	case nostr.KindReactionToWebsite:
		if e.Content != "+" && e.Content != "-" && e.Content != "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
		}
		if rTag := e.Tags.GetFirst([]string{"r"}); rTag == nil || rTag.Value() == "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong r tag value: %+v", e)
		}
	case nostr.KindBookmarkList:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindArticle) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindCommunityList:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindCommunityDefinition) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindInterestList:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindInterestSets) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindEmojiList:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindEmojiSets) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindBookmarkSets:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindArticle) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindCuratedSets:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindTextNote) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindCuratedVideoSets:
		for _, aTag := range e.Tags.GetAll([]string{"a"}) {
			if aTag != nil && (aTag.Value() == "" || len(strings.Split(aTag.Value(), ":")) != 3) || strings.Split(aTag.Value(), ":")[0] != fmt.Sprint(nostr.KindVideoEvent) {
				return errors.Wrapf(ErrWrongEventParams, "nip-51, wrong a tag value: %+v", e)
			}
		}
	case nostr.KindReporting:
		return validateKindReportEvent(e)
	case nostr.KindLabel:
		return validateKindLabelingEvent(e)
	// --- Jobs
	case KindJobTextExtraction:
		return validateKindTextExtractionJob(e)
	case KindJobSummarization:
		return validateKindSummarizationJob(e)
	case KindJobTranslation:
		return validateKindTranslationJob(e)
	case KindJobTextGeneration:
		return validateKindTextGenerationJob(e)
	case KindJobImageGeneration:
		return validateKindImageGenerationJob(e)
	case KindJobVideoConversion:
		return validateKindVideoConversionJob(e)
	case KindJobVideoTranslation:
		return validateKindVideoTranslationJob(e)
	case KindJobImageToVideoConversion:
		return validateKindImageToVideoConversionJob(e)
	case KindJobTextToSpeechGeneration:
		return validateKindTextToSpeechGenerationJob(e)
	case KindJobNostrContentDiscovery:
		return validateKindNostrContentDiscoveryJob(e)
	case KindJobNostrPeopleDiscovery:
		return validateKindNostrPeopleDiscoveryJob(e)
	case KindJobNostrContentSearch:
		return validateKindNostrContentSearchJob(e)
	case KindJobNostrPeopleSearch:
		return validateKindNostrPeopleSearchJob(e)
	case KindJobNostrEventCount:
		return validateKindNostrEventCountJob(e)
	case KindJobMalwareScanning:
		return validateKindMalwareScanningJob(e)
	case KindJobNostrEventTimeStamping:
		return validateKindNostrEventTimeStampingJob(e)
	case KindJobOpReturnCreation:
		return validateKindOpReturnCreationJob(e)
	case KindJobNostrEventPublishSchedule:
		return validateKindNostrEventPublishScheduleJob(e)
	case nostr.KindJobFeedback:
		return validateKindFeedbackJob(e)
	// --- End jobs
	case nostr.KindRelayListMetadata:
		return validateKindRelayListMetadataEvent(e)
	case nostr.KindProfileBadges:
		return validateKindProfileBadgesEvent(e)
	case nostr.KindBadgeDefinition:
		return validateKindBadgeDefinitionEvent(e)
	case nostr.KindArticle, nostr.KindDraftArticle:
		if e.Content == "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-23: this kind should have text markdown content: %+v", e)
		}
	default:
		if e.Kind >= 6000 && e.Kind <= 6999 {
			return validateKindJobResult(e)
		}
	}

	return nil
}

func validateKindTextExtractionJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5301 job text extraction: %+v", e)
}

func validateKindSummarizationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job summarization: %+v", e)
}

func validateKindTranslationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5303 job translation: %+v", e)
}

func validateKindTextGenerationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5304 job text generation: %+v", e)
}

func validateKindImageGenerationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5305 job image generation: %+v", e)
}

func validateKindVideoConversionJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5306 job video conversion: %+v", e)
}

func validateKindVideoTranslationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5307 job video translation: %+v", e)
}

func validateKindImageToVideoConversionJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5308 job image to video conversion: %+v", e)
}

func validateKindTextToSpeechGenerationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5309 job text to speech generation: %+v", e)
}

func validateKindNostrContentDiscoveryJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5301 job nostr content discovery: %+v", e)
}

func validateKindNostrPeopleDiscoveryJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job nostr people discovery: %+v", e)
}

func validateKindNostrContentSearchJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job nostr content search: %+v", e)
}

func validateKindNostrPeopleSearchJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5303 job nostr people search: %+v", e)
}

func validateKindNostrEventCountJob(e *Event) error {
	if len(e.Tags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "kind:5400 job nostr event count, no tags: %+v", e)
	}
	if e.Content == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:5400 job nostr event count, no content: %+v", e)
	}
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "relay" && !nostr.IsValidRelayURL(tag[2]) {
			return errors.Wrapf(ErrWrongEventParams, "kind:5400 wrong relay tag param: %+v", e)
		}
	}

	return nil
}

func validateKindMalwareScanningJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5401 job malware scanning: %+v", e)
}

func validateKindNostrEventTimeStampingJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5402 job nostr event timestamping: %+v", e)
}

func validateKindOpReturnCreationJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5901 job nostr event timestamping: %+v", e)
}

func validateKindNostrEventPublishScheduleJob(e *Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5902 job nostr event publish schedule: %+v", e)
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
	if labelTag == nil && labelNamespaceTag == nil && e.Kind != nostr.KindLabel {
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
	if parsedContent.Name == "" || parsedContent.DisplayName == "" {
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
	var repostedEvent Event

	if !json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: content field should be stringified json: %q", e.Content)
	}
	if err := repostedEvent.UnmarshalJSON([]byte(e.Content)); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong json fields: %v", err)
	} else if err := repostedEvent.Validate(); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: invalid reposted event: %v", err)
	}

	if e.Kind == nostr.KindRepost {
		if repostedEvent.Kind != nostr.KindTextNote {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong kind of reposted event: found %d, expected %d", repostedEvent.Kind, nostr.KindTextNote)
		}
	} else {
		if kTag := e.GetTag("k"); kTag.Value() != strconv.Itoa(repostedEvent.Kind) {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong kind of generic reposted event: found %q, expected %d", kTag.Value(), repostedEvent.Kind)
		}
	}

	if eTag := e.GetTag("e"); eTag.Value() != repostedEvent.ID {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include e tag with id of the note: found %q, expected %q", eTag.Value(), repostedEvent.ID)
	}

	if pTag := e.GetTag("p"); pTag.Value() != repostedEvent.GetMasterPublicKey() {
		return errors.Wrapf(ErrWrongEventParams,
			"nip-18: repost must include p tag with pubkey of the event being reposted: found %q, expected %q",
			pTag.Value(), repostedEvent.GetMasterPublicKey())
	}
	return nil
}

func validateKindReactionEvent(e *Event) error {
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

func validateKindJobResult(e *Event) error {
	if e.Content == "" {
		return errors.Wrap(ErrWrongEventParams, "kind:6xxx job result: content is empty")
	}
	if jobRequestTag := e.GetTag("request"); jobRequestTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:6xxx job result: no job request tag or it is empty: %+v", jobRequestTag)
	}
	if jobRequestIDTag := e.GetTag("e"); jobRequestIDTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:6xxx job result: no job request ID tag or it is empty: %+v", jobRequestIDTag)
	}
	if customerPubkeyTag := e.GetTag("p"); customerPubkeyTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:6xxx job result, no customer pubkey tag or it is empty: %+v", customerPubkeyTag)
	}
	return nil
}

func validateKindFeedbackJob(e *Event) error {
	statusTag := e.Tags.GetFirst([]string{"status"})
	if statusTag == nil || len(*statusTag) < 2 {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, no status tag: %+v", e)
	}
	if _, ok := JobFeedbackStatusValues[statusTag.Value()]; !ok {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, wrong status tag: %+v", e)
	}
	jobRequestIDTag := e.Tags.GetFirst([]string{"e"})
	if jobRequestIDTag == nil || len(*jobRequestIDTag) != 2 {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, no job request ID tag: %+v", e)
	}
	customerPubkeyTag := e.Tags.GetFirst([]string{"p"})
	if customerPubkeyTag == nil || len(*customerPubkeyTag) != 2 {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, no customer pubkey tag: %+v", e)
	}

	return nil
}

func validateIMetaTag(tag nostr.Tag) error {
	if tag == nil {
		return nil
	}

	values := make(map[string]string)
	// Parse tag values and check for all unsupported values.
	for _, val := range tag[1:] {
		parts := strings.Split(val, " ")
		if len(parts) < 2 {
			return errors.Wrapf(ErrWrongEventParams, "wrong imeta tag: %+v", tag)
		} else if _, ok := SupportedIMetaKeys[parts[0]]; !ok {
			return errors.Wrapf(ErrWrongEventParams, "not supported imeta value: %s", parts[0])
		} else if _, ok := values[parts[0]]; ok {
			return errors.Wrapf(ErrWrongEventParams, "duplicate imeta value: %s", parts[0])
		}
		values[parts[0]] = parts[1]
	}

	// Check for all required values.
	for key, required := range SupportedIMetaKeys {
		if required && values[key] == "" {
			return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: %s", key)
		}
	}

	// Either x or ox should be present and they should be hex.
	if values["x"] == "" && values["ox"] == "" {
		return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: x or ox")
	}

	// Check for values correctness.
	for key, value := range values {
		switch key {
		case "x", "ox":
			if _, err := hex.DecodeString(value); err != nil {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be hex", key)
			}
		case "url":
			if !strings.HasPrefix(value, "http") {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be url", key)
			}
		case "m":
			if strings.ToLower(value) != value {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be lowercase", key)
			} else if strings.HasPrefix(value, "video") {
				for _, videoKey := range []string{"thumb", "image", "dim"} {
					if values[videoKey] == "" {
						return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: %s for video content", videoKey)
					}
				}
			}
		case "dim":
			if len(strings.Split(value, "x")) != 2 {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be in format: 123x123", key)
			}
		}
	}

	return nil
}

func validateEventTags(e *Event) error {
	supportedTags, ok := KindSupportedTags[e.Kind]
	if !ok {
		return nil
	}

	for _, tag := range e.Tags {
		_, isCommon := CommongTags[tag.Key()]
		if _, ok := supportedTags[tag.Key()]; !ok && !isCommon {
			return errors.Wrapf(ErrUnsupportedTag, "tag: %v", tag)
		}

		if tag.Key() == "imeta" {
			if err := validateIMetaTag(tag); err != nil {
				return errors.Join(ErrUnsupportedTag, err)
			}
		}
	}

	return nil
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

func tagsTable(tags ...string) map[string]struct{} {
	table := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		table[tag] = struct{}{}
	}
	return table
}
