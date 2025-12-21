// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	maxLabelSymbolLength int = 100
	maxTagsCount         int = 30_000

	tagStateOptional tagState = iota
	tagStateRequired
	tagStateRequiredWith
	tagStateForbidden
	tagStateOneOf
	tagStateOneOfSingle

	kindValidatorFlagContentRequired                 uint = 1 << 0
	kindValidatorFlagIONIdentityKeySignatureRequired uint = 1 << 1
	kindValidatorFlagContentEmpty                    uint = 1 << 2
)

type (
	tagState uint
	tagData  struct {
		// Additional tags.
		Tags []string
		// Tag state, one of: required, optional, forbidden, etc.
		State tagState
	}
	kindValidator struct {
		// Tag map: tag key -> tag state.
		Tags map[string]tagData
		// Additional validation function.
		Validate []func(ctx context.Context, v *eventValidator, e *model.Event) error
		// Additional flags for given kind.
		Flags uint
	}
)

var (
	ErrWrongEventParams               = errors.New("wrong event params")
	ErrPollTTLExpired                 = errors.New("expiration timestamp is in the past")
	ErrUnsupportedTag                 = errors.New("unsupported tag")
	ErrUnsupportedJob                 = errors.New("unsupported job")
	ErrUnsupportedKind                = errors.New("unsupported kind")
	ErrActionForbidden                = errors.New("forbidden")
	ErrEphemeralForbidden             = errors.New("ephemeral events are forbidden")
	ErrNotFound                       = errors.New("not found")
	ErrContentEmpty                   = errors.New("content is empty")
	ErrContentNotEmpty                = errors.New("content must be empty")
	ErrEventInvalidID                 = errors.New("event id is invalid")
	ErrEventInvalidSign               = errors.New("event signature is invalid")
	ErrSignatureByIONIdentityRequired = errors.New("event requires signature by ion identity")
	ErrWalletRequired                 = errors.New("valid wallet address is required")

	CommongTags = []string{
		"t",
		"l",
		"L",
		"nonce",
		"imeta",
		"expiration",
		model.CustomIONTagOnBehalfOf,
		"settings",
		"encrypted",
	}

	KindSupportedTags = map[model.Kind]kindValidator{
		nostr.KindProfileMetadata: tagsTable("e", "p", "a", "alt"),
		nostr.KindTextNote:        tagsTable("e", "p", "q", model.CustomIONTagPMO, model.CustomIONTagPoll, model.CustomIONTagCommunity, model.CustomIONTagRichText),
		nostr.KindDirectMessage:   tagsTable(model.CustomIONTagPoll, model.CustomIONTagRichText),
		nostr.KindFollowList:      tagsTable("p"),
		nostr.KindDeletion:        newKindValidatorBuilderEmpty().Optional("e", "p", "a", "k", "nonce", model.CustomIONTagOnBehalfOf).Build(),
		nostr.KindRepost:          newKindValidatorBuilder().Optional(model.CustomIONTagCommunity, "k").Required("p").OneOf("e", "a").Build(),
		nostr.KindReaction: newKindValidatorBuilder().
			Required("p", "k").
			OneOf("e", "a").
			Validate(func(_ context.Context, v *eventValidator, e *model.Event) error {
				kTag := e.GetTag("k").Value()
				kValue, err := strconv.Atoi(kTag)
				if err != nil {
					return errors.Wrap(ErrWrongEventParams, "tag k: value should be an integer")
				}
				switch kValue {
				case nostr.KindTextNote, nostr.KindArticle, model.CustomIONKindEditableTextNote:
					if e.Content != "+" {
						return errors.Wrapf(ErrWrongEventParams, "%q: for text notes and articles only '+' reactions are allowed", e.Content)
					}
				}
				return nil
			}).
			Build(),
		nostr.KindBadgeAward:        newKindValidatorBuilder().Optional("a", "p").RequireIONIdentitySignature().Build(),
		nostr.KindGenericRepost:     newKindValidatorBuilder().Optional(model.CustomIONTagCommunity).Required("p", "k").OneOf("e", "a").Build(),
		nostr.KindReactionToWebsite: tagsTable("r"),
		nostr.KindMuteList:          tagsTable("p", "t", "word", "e"),
		nostr.KindPinList:           tagsTable("e"),
		nostr.KindBookmarkList:      tagsTable("e", "a", "t", "r"),
		nostr.KindCommunityList:     tagsTable("a"),
		nostr.KindPublicChatList:    tagsTable("e"),
		nostr.KindBlockedRelayList:  tagsTable("relay", "r"),
		nostr.KindSearchRelayList:   tagsTable("relay", "r"),
		nostr.KindSimpleGroupList:   tagsTable("group"),
		nostr.KindInterestList:      tagsTable("t", "a"),
		nostr.KindEmojiList:         tagsTable("emoji", "a"),
		nostr.KindDMRelayList:       tagsTable("relay", "r"),
		nostr.KindGiftWrap: newKindValidatorBuilder().
			Required("p", "k").
			Optional("expiration", "payload-compression").
			Validate(validateKindGiftWrapEvent).
			Build(),
		nostr.KindGoodWikiAuthorList:    tagsTable("p"),
		nostr.KindGoodWikiRelayList:     tagsTable("relay", "r"),
		nostr.KindCategorizedPeopleList: tagsTable("p", "d", "title", "image", "description"),
		nostr.KindRelaySets:             tagsTable("relay", "r", "d", "title", "image", "description"),
		nostr.KindBookmarkSets:          tagsTable("e", "a", "t", "r", "d", "title", "image", "description", model.CustomIONTagCommunity),
		nostr.KindCuratedSets:           tagsTable("a", "e", "d", "title", "image", "description"),
		nostr.KindCuratedVideoSets:      tagsTable("a", "d", "title", "image", "description"),
		nostr.KindMuteSets:              tagsTable("p", "d", "title", "image", "description"),
		nostr.KindInterestSets:          tagsTable("t", "d", "title", "image", "description"),
		nostr.KindEmojiSets:             tagsTable("emoji", "d", "title", "image", "description"),
		nostr.KindReleaseArtifactSets:   tagsTable("e", "i", "version", "d", "title", "image", "description"),
		nostr.KindLabel:                 tagsTable("e", "p", "a", "r", "t"),
		nostr.KindRelayListMetadata:     tagsTable("r"),
		nostr.KindProfileBadges:         tagsTable("d", "a", "e"),
		nostr.KindBadgeDefinition:       newKindValidatorBuilder().Optional("d", "p", "name", "image", "description", "thumb").RequireIONIdentitySignature().Build(),
		nostr.KindArticle:               tagsTable("p", "a", "d", "e", "t", "title", "image", "summary", "editing_ended_at", "published_at", model.CustomIONTagPMO, model.CustomIONTagRichText, model.CustomIONTagAddressableQ, model.CustomIONTagPoll, model.CustomIONTagCommunity),
		nostr.KindDraftArticle:          tagsTable("p", "a", "d", "e", "t", "title", "image", "summary", "editing_ended_at", "published_at", model.CustomIONTagPMO, model.CustomIONTagRichText, model.CustomIONTagAddressableQ, model.CustomIONTagPoll, model.CustomIONTagCommunity),

		// --- Jobs
		model.KindJobTextExtraction:            tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobSummarization:             tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobTranslation:               tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobTextGeneration:            tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobImageGeneration:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobVideoConversion:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobVideoTranslation:          tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobImageToVideoConversion:    tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobTextToSpeechGeneration:    tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrContentDiscovery:     tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrPeopleDiscovery:      tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrContentSearch:        tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrPeopleSearch:         tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrEventCount:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobMalwareScanning:           tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrEventTimeStamping:    tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobOpReturnCreation:          tagsTable("i", "output", "param", "bid", "relays", "p"),
		model.KindJobNostrEventPublishSchedule: tagsTable("i", "output", "param", "bid", "relays", "p", "encrypted"),
		nostr.KindJobFeedback:                  tagsTable("status", "amount", "e", "p"),

		// Community
		model.CustomIONKindCommunityDefinition:            tagsTable(model.CustomIONTagCommunity, "d", "name", "description", "public", "private", "open", "closed", "p", "a"),
		model.CustomIONKindCommunityOwnershipTransferring: tagsTable(model.CustomIONTagCommunity, "a", "p"),
		model.CustomIONKindCommunityJoin:                  tagsTable(model.CustomIONTagCommunity, "p", "authorization"),
		model.CustomIONKindCommunityBanUser:               tagsTable(model.CustomIONTagCommunity, "p"),
		model.CustomIONKindCommunityChangeDefinition:      tagsTable(model.CustomIONTagCommunity, "name", "description", "public", "private", "open", "closed", "p"),

		model.CustomIONKindEditableTextNote: newKindValidatorBuilder().
			Optional("a", "e", "d", "p", "q", "p",
				"editing_ended_at",
				model.CustomIONTagPMO,
				model.CustomIONTagPoll,
				model.CustomIONTagCommunity,
				model.CustomIONTagAddressableQ,
				model.CustomIONTagRichText,
			).
			Required("published_at").
			Build(),

		model.CustomIONKindPollVote: newKindValidatorBuilder().OneOfSingle("e", "a").Forbidden("expiration").Build(),

		model.CustomIONKindFundReceive: newKindValidatorBuilderEmpty().
			ContentNotEmpty().
			Optional("asset_address").
			OneOf("p", "l").
			Required(model.CustomIONTagOnBehalfOf, "network", "asset_class").
			RequiredWith("l", "L").
			Validate(func(_ context.Context, v *eventValidator, e *model.Event) error {
				return validateKindFundReceive(e)
			}).
			Build(),

		model.CustomIONKindFundSendNotify: newKindValidatorBuilderEmpty().
			ContentNotEmpty().
			Optional("request", "asset_address").
			OneOf("p", "l").
			Required(model.CustomIONTagOnBehalfOf, "network", "asset_class").
			RequiredWith("l", "L").
			Validate(func(_ context.Context, v *eventValidator, e *model.Event) error {
				return validateKindFundSendNotify(e)
			}).
			Build(),

		model.CustomIONKindDeviceRegistration: newKindValidatorBuilder().
			ContentNotEmpty().
			Required("d", "t", "relay", "token").
			Build(),

		model.CustomIONKindAttestation: newKindValidatorBuilderEmpty().
			Required(model.TagAttestationName).
			Optional("nonce").
			Build(),

		model.CustomIONKindTokenizedCommunityDefinition: newKindValidatorBuilder().
			OneOfSingle("e", "a", "h").
			Optional("p", "platform").
			Required("k").
			Forbidden("expiration").
			ContentEmpty().
			Validate(validateInternalTopicTC).
			Validate(validateTokenizedCommunityFirstBuy).
			Build(),

		model.CustomIONKindTokenizedCommunityAction: newKindValidatorBuilder().
			OneOfSingle("e", "a").
			Required(
				"network",
				"bonding_curve_address",
				"token_address",
				"tx_address",
				"tx_type",
				"tx_amount",
			).
			Optional("p").
			Forbidden("expiration").
			ContentEmpty().
			Validate(validateInternalTopicTC).
			Build(),
	}

	// Allow multiple `p` tags for given kinds that point to the same user.
	kindAllowMultipleTagsP = map[model.Kind]struct{}{
		model.CustomIONKindAttestation: {}, // Could be multiple attestations, like active, revoked, etc.
	}
)

func validateATags(e *model.Event, expectedKinds ...int) error {
	return validateAddressableTag(e, "a", expectedKinds...)
}

func validateAddressableTag(e *model.Event, tagName string, expectedKinds ...int) error {
	for _, tag := range e.GetTags(tagName) {
		if tag.Value() == "" {
			return errors.Wrapf(ErrWrongEventParams, "tag %v: empty value", tag.Key())
		}

		parts := strings.Split(tag.Value(), ":")
		if len(parts) != 3 {
			return errors.Wrapf(ErrWrongEventParams, "tag %v: value should have 3 parts, but got %d: %v", tag.Key(), len(parts), tag.Value())
		}

		kind, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return errors.Wrapf(ErrWrongEventParams, "tag %v: value should have kind as first part, but got %q: %v", tag.Key(), parts[0], err)
		}

		if len(expectedKinds) > 0 {
			if !slices.Contains(expectedKinds, int(kind)) {
				return errors.Wrapf(ErrWrongEventParams, "tag %v: value should have one of the expected kinds '%v', but got %d", tag.Key(), expectedKinds, kind)
			}
		}
	}
	return nil
}

func extractTagValueFromPairs(tag model.Tag, key string) (value string, err error) {
	if len(tag) < 2 {
		return "", errors.Wrapf(ErrWrongEventParams, "tag %q is empty", tag.Key())
	}

	for _, part := range tag[1:] {
		parts := strings.SplitN(part, " ", 2)
		if len(parts) != 2 {
			return "", errors.Wrapf(ErrWrongEventParams, "invalid tag value: %q: want key value", part)
		}

		if key == strings.TrimSpace(parts[0]) {
			return strings.TrimSpace(parts[1]), nil
		}
	}

	return "", errors.Wrapf(ErrWrongEventParams, "tag %q does not have key %q", tag.Key(), key)
}

func (ev *eventValidator) validateKindEphemeralEmbeddingEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	var wrappedEvent model.Event

	if model.GetUserDataFromContext(ctx).Authoritative {
		return errors.Wrap(ErrEphemeralForbidden, "authoritative relays do not allow ephemeral embedding events")
	}

	err := wrappedEvent.UnmarshalJSON([]byte(e.Content))
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal wrapped event")
	}

	return ev.validate(ctx, rules, batch, &wrappedEvent)
}

func (ev *eventValidator) validate(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	if !e.CheckID() {
		return ErrEventInvalidID
	}
	if ok, err := e.CheckSignature(); err != nil {
		return errors.Wrap(err, "signature check failed")
	} else if !ok {
		return ErrEventInvalidSign
	}
	if ev.Config != nil && ev.Config.NIP13MinLeadingZeroBits > 0 {
		if err := e.CheckNIP13Difficulty(ev.Config.NIP13MinLeadingZeroBits); err != nil {
			return errors.Wrap(err, "wrong event difficulty")
		}
	}
	if e.Kind < 0 || e.Kind > math.MaxUint16 {
		return errors.Wrapf(ErrUnsupportedKind, "kind: %d", e.Kind)
	}
	if len(e.Tags) > maxTagsCount {
		return errors.Wrapf(ErrWrongEventParams, "too many tags: %d", len(e.Tags))
	}
	if err := validateEventTags(e, KindSupportedTags); err != nil {
		return errors.Wrapf(err, "event: %+v", e)
	}
	var contentSize int
	if e.Content != "" {
		contentSize = len(e.Content)
	} else {
		contentSize = len(model.ExtractRichTextContent(e))
	}
	if maxSize := ev.Config.MaxContentSizeOf(e.Kind); maxSize > 0 && contentSize > maxSize {
		return errors.Wrapf(ErrWrongEventParams, "content is too long %d, max is %d", contentSize, maxSize)
	}
	if v, ok := KindSupportedTags[e.Kind]; ok {
		if err := v.Execute(ctx, ev, e); err != nil {
			return errors.Wrap(ErrWrongEventParams, err.Error())
		}
	}
	switch e.Kind {
	case model.CustomIONKindAttestation:
		return ev.validateKindAttestationEvent(ctx, rules, batch, e)
	case nostr.KindProfileMetadata:
		return ev.validateKindProfileMetadataEvent(ctx, rules, batch, e)
	case nostr.KindTextNote:
		return ev.validateKindTextNoteEvent(ctx, rules, batch, e)
	case nostr.KindDeletion:
		return ev.validateKindDeletionEvent(ctx, e)
	case nostr.KindRepost, nostr.KindGenericRepost:
		return ev.validateKindRepostEvent(ctx, rules, batch, e)
	case nostr.KindFollowList:
		return validateFollowListEvent(e)
	case nostr.KindBadgeAward:
		return ev.validateKindBadgeAwardEvent(ctx, rules, batch, e)
	case nostr.KindDirectMessage, nostr.KindSeal:
		return errors.Wrapf(ErrUnsupportedKind, "kind: %d", e.Kind)
	case nostr.KindReactionToWebsite:
		return validateKindReactionToWebsiteEvent(e)
	case model.CustomIONKindPollVote:
		return ev.validatePollVote(ctx, e)
	case nostr.KindCommunityList:
		return validateATags(e, model.CustomIONKindCommunityDefinition)
	case nostr.KindInterestList:
		return validateATags(e, nostr.KindInterestSets)
	case nostr.KindEmojiList:
		return validateATags(e, nostr.KindEmojiSets)
	case nostr.KindBookmarkSets, nostr.KindCuratedSets, nostr.KindCuratedVideoSets:
		var rules = map[int][]int{
			nostr.KindBookmarkSets:     {}, // Any.
			nostr.KindCuratedSets:      {nostr.KindArticle, nostr.KindTextNote},
			nostr.KindCuratedVideoSets: {nostr.KindVideoEvent, nostr.KindShortVideoEvent},
		}
		return validateATags(e, rules[e.Kind]...)
	case nostr.KindReporting:
		return validateKindReportEvent(e)
	case nostr.KindLabel:
		return validateKindLabelingEvent(e)
	// --- Jobs
	case model.KindJobTextExtraction:
		return validateKindTextExtractionJob(e)
	case model.KindJobSummarization:
		return validateKindSummarizationJob(e)
	case model.KindJobTranslation:
		return validateKindTranslationJob(e)
	case model.KindJobTextGeneration:
		return validateKindTextGenerationJob(e)
	case model.KindJobImageGeneration:
		return validateKindImageGenerationJob(e)
	case model.KindJobVideoConversion:
		return validateKindVideoConversionJob(e)
	case model.KindJobVideoTranslation:
		return validateKindVideoTranslationJob(e)
	case model.KindJobImageToVideoConversion:
		return validateKindImageToVideoConversionJob(e)
	case model.KindJobTextToSpeechGeneration:
		return validateKindTextToSpeechGenerationJob(e)
	case model.KindJobNostrContentDiscovery:
		return validateKindNostrContentDiscoveryJob(e)
	case model.KindJobNostrPeopleDiscovery:
		return validateKindNostrPeopleDiscoveryJob(e)
	case model.KindJobNostrContentSearch:
		return validateKindNostrContentSearchJob(e)
	case model.KindJobNostrPeopleSearch:
		return validateKindNostrPeopleSearchJob(e)
	case model.KindJobNostrEventCount:
		return validateKindNostrEventCountJob(e)
	case model.KindJobMalwareScanning:
		return validateKindMalwareScanningJob(e)
	case model.KindJobNostrEventTimeStamping:
		return validateKindNostrEventTimeStampingJob(e)
	case model.KindJobOpReturnCreation:
		return validateKindOpReturnCreationJob(e)
	case model.KindJobNostrEventPublishSchedule:
		return validateKindNostrEventPublishScheduleJob(e)
	case nostr.KindJobFeedback:
		return validateKindFeedbackJob(e)
	// --- End jobs
	case nostr.KindRelayListMetadata:
		return validateKindRelayListMetadataEvent(e)
	case nostr.KindProfileBadges:
		return ev.validateKindProfileBadgesEvent(ctx, rules, batch, e)
	case nostr.KindBadgeDefinition:
		return validateKindBadgeDefinitionEvent(e)
	case nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		return ev.validateTextNote(ctx, rules, batch, e)
	case model.CustomIONKindCommunityDefinition, model.CustomIONKindCommunityChangeDefinition:
		return validateCustomIONKindCommunityDefinitionEvent(ctx, e)
	case model.CustomIONKindCommunityJoin:
		return validateCustomIONKindCommunityJoinEvent(ctx, e)
	case model.CustomIONKindCommunityOwnershipTransferring:
		return validateCustomIONKindCommunityOwnershipTransferringEvent(ctx, e)
	case model.CustomIONKindCommunityBanUser:
		return validateCustomIONKindCommunityBanUserEvent(ctx, e)
	case model.CustomIONKindDeviceRegistration:
		return ev.validateKindDeviceRegistration(ctx, rules, batch, e)
	case model.CustomIONKindEphemeralEmbedding:
		wrappedRules := &ruleSet{
			SkipKindProfileProofEventsVerify: true,
		}
		if rules != nil {
			wrappedRules.SkipKindAttestationProofDevicesVerify = rules.SkipKindAttestationProofDevicesVerify
			wrappedRules.BroadcastMode = rules.BroadcastMode
			wrappedRules.SkipRootContentNFTCollectionsValidation = rules.SkipRootContentNFTCollectionsValidation
			wrappedRules.SkipRootContentReplyValidation = rules.SkipRootContentReplyValidation
		}
		return ev.validateKindEphemeralEmbeddingEvent(ctx, wrappedRules, batch, e)
	default:
		if e.IsJobResponse() {
			return validateKindJobResult(e)
		}
	}

	return nil
}

func validateEventTags(e *model.Event, rules map[model.Kind]kindValidator) error {
	var bTag string
	currentTags := make(map[string]int)
	pTags := make(map[string]int)
	kindValidator, known := rules[e.Kind]
	for _, tag := range e.Tags {
		if data, ok := kindValidator.Tags[tag.Key()]; known && !ok {
			switch {
			case tag.Key() == "d" && e.IsAddressable():
				// Allow `d` tag for addressable events even if it's not explicitly listed.
			default:
				return errors.Wrapf(ErrUnsupportedTag, "tag: %v", tag)
			}
		} else if data.State == tagStateForbidden {
			return errors.Wrapf(ErrUnsupportedTag, "tag: %v: cannot be used with this kind", tag)
		}

		switch tag.Key() {
		case "imeta":
			if err := validateIMetaTag(tag); err != nil {
				return errors.Join(ErrUnsupportedTag, err)
			}
		case "a", model.CustomIONTagAddressableQ:
			if err := validateAddressableTag(e, tag.Key()); err != nil {
				return err
			}
		case model.CustomIONTagCommunity:
			if e.Kind != model.CustomIONKindTokenizedCommunityDefinition {
				if val, err := uuid.Parse(tag.Value()); err != nil {
					return errors.Wrapf(ErrWrongEventParams, "tag %v: error: %q: %v", model.CustomIONTagCommunity, tag.Value(), err)
				} else if version := val.Version(); version != 0x7 {
					return errors.Wrapf(ErrWrongEventParams, "tag %v: wrong UUID version: %#02x, expected %#02x", model.CustomIONTagCommunity, version, 0x7)
				}
			}
		case model.CustomIONTagPoll:
			if err := validatePollTag(tag); err != nil {
				return err
			}
		case "expiration", "published_at", "editing_ended_at":
			v, err := nostr.ParseTimestamp(tag.Value())
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "tag: %s: should be uint: %v", tag.Key(), err)
			} else if v < 0 {
				return errors.Wrapf(ErrWrongEventParams, "tag: %s: should be positive: %d", tag.Key(), v)
			}
		case "settings":
			if err := validateSettingsTag(e, tag); err != nil {
				return errors.Join(ErrUnsupportedTag, err)
			}
		case "k":
			kTag := tag.Value()
			kValue, err := strconv.Atoi(kTag)
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "tag %s: value should be an integer", tag.Key())
			} else if kValue < 0 || kValue > math.MaxUint16 {
				return errors.Wrapf(ErrWrongEventParams, "tag %s: value should be between 0 and %d", tag.Key(), math.MaxUint16)
			}
		case model.CustomIONTagOnBehalfOf:
			if bTag != "" {
				return errors.Wrapf(ErrWrongEventParams, "tag %q: cannot be used more than once", tag.Key())
			}
			bTag = tag.Value()
		case "p":
			pTags[tag.Value()]++
		}
		currentTags[tag.Key()]++
	}

	if _, allow := kindAllowMultipleTagsP[e.Kind]; !allow {
		for val, count := range pTags {
			if count > 1 {
				return errors.Wrapf(ErrWrongEventParams, "tag %q: %v used more than once", "p", val)
			}
		}
	}

	for key, data := range kindValidator.Tags {
		switch data.State {
		case tagStateRequiredWith:
			if _, ok := currentTags[key]; !ok {
				continue
			}
			for _, dependentTag := range data.Tags {
				if _, ok := currentTags[dependentTag]; !ok {
					return errors.Wrapf(ErrWrongEventParams, "tag %q marked as required with %q: not found", key, dependentTag)
				}
			}
		case tagStateRequired:
			if _, ok := currentTags[key]; !ok {
				return errors.Wrapf(ErrWrongEventParams, "tag %q marked as required: not found", key)
			}
		case tagStateOneOfSingle:
			found := map[string]int{}
			for _, tag := range data.Tags {
				if v, ok := currentTags[tag]; ok {
					found[tag] = v
				}
			}
			if len(found) == 0 {
				return errors.Wrapf(ErrWrongEventParams, "one of tags %v must be present", data.Tags)
			} else if len(found) > 1 {
				keys := make([]string, 0, len(found))
				for key := range found {
					keys = append(keys, key)
				}
				return errors.Wrapf(ErrWrongEventParams, "only one of tags %v must be present, found %v", data.Tags, keys)
			} else {
				for tag, count := range found {
					if count > 1 {
						return errors.Wrapf(ErrWrongEventParams, "tag %q: used more than once: %v", tag, count)
					}
				}
			}
		case tagStateOneOf:
			found := map[string]struct{}{}
			for _, tag := range data.Tags {
				if _, ok := currentTags[tag]; ok {
					found[tag] = struct{}{}
				}
			}
			if len(found) == 0 {
				return errors.Wrapf(ErrWrongEventParams, "one of tags %v must be present", data.Tags)
			} else if len(found) > 1 {
				keys := make([]string, 0, len(found))
				for key := range maps.Keys(found) {
					keys = append(keys, key)
				}
				return errors.Wrapf(ErrWrongEventParams, "only one of tags %v must be present, found %v", data.Tags, keys)
			}
		}
	}

	if e.IsAddressable() && e.GetTag("d").Value() == "" {
		return errors.Wrap(ErrWrongEventParams, "addressable event must have non-empty d tag")
	}

	return nil
}

func tagsTable(tags ...string) kindValidator {
	return newKindValidatorBuilder().Optional(tags...).Build()
}

type kindValidatorBuilder struct {
	Validator kindValidator
}

func newKindValidatorBuilder() *kindValidatorBuilder {
	t := newKindValidatorBuilderEmpty()
	for _, tag := range CommongTags {
		t = t.Optional(tag)
	}
	return t
}

func newKindValidatorBuilderEmpty() *kindValidatorBuilder {
	return &kindValidatorBuilder{
		Validator: kindValidator{
			Tags: make(map[string]tagData),
		},
	}
}

func (t *kindValidatorBuilder) ContentNotEmpty() *kindValidatorBuilder {
	t.Validator.Flags |= kindValidatorFlagContentRequired
	return t
}

func (t *kindValidatorBuilder) ContentEmpty() *kindValidatorBuilder {
	t.Validator.Flags |= kindValidatorFlagContentEmpty
	return t
}

func (t *kindValidatorBuilder) RequireIONIdentitySignature() *kindValidatorBuilder {
	t.Validator.Flags |= kindValidatorFlagIONIdentityKeySignatureRequired
	return t
}

func (t *kindValidatorBuilder) Optional(tags ...string) *kindValidatorBuilder {
	for _, tag := range tags {
		t.Validator.Tags[tag] = tagData{State: tagStateOptional}
	}
	return t
}

func (t *kindValidatorBuilder) Required(tags ...string) *kindValidatorBuilder {
	for _, tag := range tags {
		t.Validator.Tags[tag] = tagData{State: tagStateRequired}
	}
	return t
}

func (t *kindValidatorBuilder) Forbidden(tags ...string) *kindValidatorBuilder {
	for _, tag := range tags {
		t.Validator.Tags[tag] = tagData{State: tagStateForbidden}
	}
	return t
}

func (t *kindValidatorBuilder) RequiredWith(root string, tags ...string) *kindValidatorBuilder {
	t.Validator.Tags[root] = tagData{Tags: tags, State: tagStateRequiredWith}

	return t.Optional(tags...)
}

func (t *kindValidatorBuilder) OneOf(tags ...string) *kindValidatorBuilder {
	data := tagData{Tags: tags, State: tagStateOneOf}
	for _, tag := range tags {
		t.Validator.Tags[tag] = data
	}
	return t
}

func (t *kindValidatorBuilder) OneOfSingle(tags ...string) *kindValidatorBuilder {
	data := tagData{Tags: tags, State: tagStateOneOfSingle}
	for _, tag := range tags {
		t.Validator.Tags[tag] = data
	}
	return t
}

func (t *kindValidatorBuilder) Validate(f ...func(ctx context.Context, v *eventValidator, e *model.Event) error) *kindValidatorBuilder {
	t.Validator.Validate = append(t.Validator.Validate, f...)
	return t
}

func (t *kindValidatorBuilder) Build() kindValidator {
	return t.Validator
}

func (v *kindValidator) Execute(ctx context.Context, ev *eventValidator, e *model.Event) (err error) {
	if v.Flags&kindValidatorFlagContentRequired != 0 && e.Content == "" {
		err = errors.Join(err, ErrContentEmpty)
	}
	if v.Flags&kindValidatorFlagContentEmpty != 0 && e.Content != "" {
		err = errors.Join(err, ErrContentNotEmpty)
	}
	if v.Flags&kindValidatorFlagIONIdentityKeySignatureRequired != 0 && !slices.Contains(ev.IONIdentityPublicKeys(), e.PubKey) {
		err = errors.Join(err, ErrSignatureByIONIdentityRequired)
	}
	for _, f := range v.Validate {
		err = errors.Join(err, f(ctx, ev, e))
	}
	return err
}
