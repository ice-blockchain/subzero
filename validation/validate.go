// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

const (
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

	tagStateOptional tagState = iota
	tagStateRequired
	tagStateRequiredWith
	tagStateForbidden
	tagStateOneOf

	kindValidatorFlagContentRequired uint = 1 << 0

	DeviceTokenOSAndroid = "android"
	DeviceTokenOSIOS     = "ios"
	DeviceTokenOSWeb     = "web"
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

	tagState uint
	tagData  struct {
		// Tag state, one of: required, optional, forbidden, etc.
		State tagState
		// Additional tags.
		Tags []string
	}
	kindValidator struct {
		// Tag map: tag key -> tag state.
		Tags map[string]tagData
		// Additional flags for given kind.
		Flags uint
		// Additional validation function.
		Validate func(e *model.Event) error
	}
)

var (
	ErrWrongEventParams = errors.New("wrong event params")
	ErrUnsupportedTag   = errors.New("unsupported tag")
	ErrUnsupportedJob   = errors.New("unsupported job")
	ErrUnsupportedKind  = errors.New("unsupported kind")
	ErrActionForbidden  = errors.New("forbidden")
	ErrNotFound         = errors.New("not found")
	ErrContentEmpty     = errors.New("content is empty")

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
		nostr.KindProfileMetadata:       tagsTable("e", "p", "a", "alt"),
		nostr.KindTextNote:              tagsTable("e", "p", "q", model.CustomIONTagPoll, model.CustomIONTagCommunity, model.CustomIONTagRichText),
		nostr.KindDirectMessage:         tagsTable(model.CustomIONTagPoll, model.CustomIONTagRichText),
		nostr.KindFollowList:            tagsTable("p"),
		nostr.KindDeletion:              newKindValidatorBuilderEmpty().Optional("e", "p", "a", "k", "nonce", model.CustomIONTagOnBehalfOf).Build(),
		nostr.KindRepost:                newKindValidatorBuilder().Optional(model.CustomIONTagCommunity, "k").Required("p").OneOf("e", "a").Build(),
		nostr.KindReaction:              newKindValidatorBuilder().Required("p", "k").OneOf("e", "a").Build(),
		nostr.KindBadgeAward:            tagsTable("a", "p"),
		nostr.KindGenericRepost:         newKindValidatorBuilder().Optional(model.CustomIONTagCommunity).Required("p", "k").OneOf("e", "a").Build(),
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
		nostr.KindGiftWrap:              tagsTableRequired("p", "k", "expiration"),
		nostr.KindGoodWikiAuthorList:    tagsTable("p"),
		nostr.KindGoodWikiRelayList:     tagsTable("relay"),
		nostr.KindCategorizedPeopleList: tagsTable("p", "d", "title", "image", "description"),
		nostr.KindRelaySets:             tagsTable("relay", "d", "title", "image", "description"),
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
		nostr.KindBadgeDefinition:       tagsTable("d", "name", "image", "description", "thumb"),
		nostr.KindArticle:               tagsTable("a", "d", "e", "t", "title", "image", "summary", "editing_ended_at", "published_at", model.CustomIONTagRichText, model.CustomIONTagAddressableQ, model.CustomIONTagPoll, model.CustomIONTagCommunity),
		nostr.KindDraftArticle:          tagsTable("a", "d", "e", "t", "title", "image", "summary", "editing_ended_at", "published_at", model.CustomIONTagRichText, model.CustomIONTagAddressableQ, model.CustomIONTagPoll, model.CustomIONTagCommunity),

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
			Optional("a", "e", "d", "p", "q",
				"editing_ended_at",
				model.CustomIONTagPoll,
				model.CustomIONTagCommunity,
				model.CustomIONTagAddressableQ,
				model.CustomIONTagRichText,
			).
			Required("published_at").
			Build(),

		model.CustomIONKindPollVote: newKindValidatorBuilder().OneOf("e", "a").Forbidden("expiration").Build(),

		model.CustomIONKindFundReceive: newKindValidatorBuilderEmpty().
			ContentNotEmpty().
			Optional("asset_address").
			OneOf("p", "l").
			Required(model.CustomIONTagOnBehalfOf, "network", "asset_class").
			RequiredWith("l", "L").
			Validate(func(e *model.Event) error {
				if e.GetTag("p").Value() != "" {
					return nil
				}

				var address struct {
					From string `json:"from"`
				}
				if label := e.GetTag("L").Value(); label != "wallet.address" {
					return errors.Errorf("fund receive: invalid L tag value: %q", label)
				}

				if err := json.Unmarshal([]byte(e.Content), &address); err != nil {
					return errors.Errorf("fund receive: invalid content: %v", err)
				} else if address.From == "" {
					return errors.Errorf("fund receive: empty address in content")
				} else if addr := e.GetTag("l"); address.From != addr.Value() {
					return errors.Errorf("fund receive: address in content %q does not match tag l %q", address.From, addr.Value())
				} else if len(addr) < 3 || addr[2] != "wallet.address" {
					return errors.Errorf("fund send notify: invalid alias in tag l %q", addr.Value())
				}

				return nil
			}).
			Build(),

		model.CustomIONKindFundSendNotify: newKindValidatorBuilderEmpty().
			ContentNotEmpty().
			Optional("request", "asset_address").
			OneOf("p", "l").
			Required(model.CustomIONTagOnBehalfOf, "network", "asset_class").
			RequiredWith("l", "L").
			Validate(func(e *model.Event) error {
				if e.GetTag("p").Value() != "" {
					return nil
				}

				var address struct {
					To string `json:"to"`
				}
				if label := e.GetTag("L").Value(); label != "wallet.address" {
					return errors.Errorf("fund send notify: invalid L tag value: %q", label)
				}

				if err := json.Unmarshal([]byte(e.Content), &address); err != nil {
					return errors.Errorf("fund send notify: invalid content: %v", err)
				} else if address.To == "" {
					return errors.Errorf("fund send notify: empty address in content")
				} else if addr := e.GetTag("l"); address.To != addr.Value() {
					return errors.Errorf("fund send notify: address in content %q does not match tag l %q", address.To, addr.Value())
				} else if len(addr) < 3 || addr[2] != "wallet.address" {
					return errors.Errorf("fund send notify: invalid alias in tag l %q", addr.Value())
				}

				return nil
			}).
			Build(),

		model.CustomIONKindDeviceRegistration: newKindValidatorBuilder().
			ContentNotEmpty().
			Required("d", "t", "relay", "token").
			Validate(func(e *model.Event) error {
				tTag := e.GetTag("t").Value()
				if tTag != DeviceTokenOSAndroid && tTag != DeviceTokenOSIOS && tTag != DeviceTokenOSWeb {
					return errors.Wrapf(ErrWrongEventParams, "wrong t tag value: %v", tTag)
				}
				relayTag := e.GetTag("relay").Value()
				if relayTag != globalConfig.RelayURL {
					return errors.Wrapf(ErrWrongEventParams, "relay tag value %q does not match configured relay URL %q", relayTag, globalConfig.RelayURL)
				}

				var filters model.Filters
				if err := json.Unmarshal([]byte(e.Content), &filters); err != nil {
					return errors.Wrapf(ErrWrongEventParams, "wrong content JSON value: %v", err)
				}
				for _, filter := range filters {
					if filter.Kinds == nil {
						return errors.Wrapf(ErrWrongEventParams, "filter must have kinds defined: %v", filter)
					}
					if filter.Tags != nil && filter.Tags.HasValues(model.CustomIONTagCommunity) {
						hasValidCommunityKind := false
						for _, kind := range filter.Kinds {
							if kind == nostr.KindTextNote || kind == model.CustomIONKindEditableTextNote || kind == nostr.KindGenericRepost {
								hasValidCommunityKind = true

								break
							}
						}
						if !hasValidCommunityKind {
							return errors.Wrapf(ErrWrongEventParams, "filter with h tag must contain at least one of these kinds: %v",
								[]int{nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindGenericRepost})
						}

						continue
					}
					hasAllowedKind := false
					for _, kind := range filter.Kinds {
						switch kind {
						case nostr.KindTextNote, model.CustomIONKindEditableTextNote,
							nostr.KindRepost, nostr.KindGenericRepost, nostr.KindFollowList:
							hasAllowedKind = true
						case nostr.KindGiftWrap:
							hasAllowedKind = true
							if filter.Tags == nil {
								return errors.Wrapf(ErrWrongEventParams, "filter with KindGiftWrap kind must have tags: %v", filter)
							}
							kTagValues := filter.Tags["k"]
							if kTagValues == nil || len(kTagValues) == 0 {
								return errors.Wrapf(ErrWrongEventParams, "filter with KindGiftWrap kind must have k tag: %v", filter)
							}

							validKTagValue := false
							for _, kValues := range kTagValues {
								for _, kValuePtr := range kValues {
									if kValuePtr == nil {
										continue
									}

									kInt, err := strconv.Atoi(*kValuePtr)
									if err == nil {
										switch kInt {
										case model.CustomIONKindFundReceive, model.CustomIONKindFundSendNotify,
											nostr.KindDirectMessage, model.CustomIONDirectMessage, nostr.KindReaction:
											validKTagValue = true
											break
										}
									}
								}
							}

							if !validKTagValue {
								return errors.Wrapf(ErrWrongEventParams, "filter with KindGiftWrap kind has invalid k tag value: %v", filter)
							}
						}
					}

					if !hasAllowedKind {
						return errors.Wrapf(ErrWrongEventParams, "filter must contain at least one allowed kind: %v", filter)
					}
				}

				return nil
			}).
			Build(),
	}

	SupportedIMetaKeys = map[string]tagState{
		"url":      tagStateRequired,
		"m":        tagStateRequired,
		"x":        tagStateOptional,
		"ox":       tagStateOptional,
		"size":     tagStateOptional,
		"dim":      tagStateOptional,
		"magnet":   tagStateOptional,
		"i":        tagStateRequired,
		"blurhash": tagStateOptional,
		"thumb":    tagStateOptional,
		"image":    tagStateOptional,
		"summary":  tagStateOptional,
		"alt":      tagStateRequired,
		"fallback": tagStateOptional,
		"duration": tagStateOptional,
	}

	JobFeedbackStatusValues = map[string]struct{}{
		model.JobFeedbackStatusPaymentRequired: {},
		model.JobFeedbackStatusProcessing:      {},
		model.JobFeedbackStatusError:           {},
		model.JobFeedbackStatusSuccess:         {},
		model.JobFeedbackStatusPartial:         {},
	}

	// Allow multiple `p` tags for given kinds that point to the same user.
	kindAllowMultipleTagsP = map[model.Kind]struct{}{
		model.CustomIONKindAttestation: {}, // Could be multiple attestations, like active, revoked, etc.
	}
)

func validatePollTag(tag model.Tag) error {
	var rules = map[string]int{
		"type":    0,
		"ttl":     0,
		"title":   0,
		"options": 0,
	}
	for _, part := range tag[1:] {
		parts := strings.SplitN(part, " ", 2)
		if len(parts) != 2 {
			return errors.Wrapf(ErrWrongEventParams, "poll: invalid tag value: %q: want key value", part)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "type":
			if value != "single" && value != "multi" {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid type value: %q, want single or multi", value)
			}
		case "ttl":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid ttl value: %q, want unix time: %v", value, err)
			} else if v < 0 {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid ttl value: %q, want unix time", value)
			} else if v > 0 && time.Unix(v, 0).Before(time.Now()) {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid ttl value: %q, want unix time in the future", value)
			}
		case "title":
			if value == "" {
				return errors.Wrap(ErrWrongEventParams, "poll: title is empty")
			}
		case "options":
			var options []string

			if err := json.Unmarshal([]byte(value), &options); err != nil {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid options value: %q: %v", value, err)
			} else if len(options) == 0 {
				return errors.Wrap(ErrWrongEventParams, "poll: options are empty")
			}
		default:
			return errors.Wrapf(ErrWrongEventParams, "poll: unknown key: %q", key)
		}
		rules[key]++
	}
	for key, count := range rules {
		if count == 0 {
			return errors.Wrapf(ErrWrongEventParams, "poll: missing required key: %q", key)
		}
	}
	return nil
}

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

func validatePollVote(ctx context.Context, e *model.Event) error {
	events := e.GetTags("e")
	if len(events) != 1 {
		return errors.Wrapf(ErrWrongEventParams, "vote: expected one e tag, but got %d", len(events))
	}

	var poll *model.Event
	for ev, err := range query.GetStoredEvents(ctx, &model.Subscription{Filters: model.Filters{model.Filter{IDs: events[0]}}}) {
		if err != nil {
			return errors.Wrap(err, "vote: failed to get poll event")
		}
		poll = ev
	}
	if poll == nil {
		return errors.Wrap(ErrWrongEventParams, "vote: poll event not found")
	}

	pollTag := poll.GetTag(model.CustomIONTagPoll)
	if pollTag == nil {
		return errors.Wrap(ErrWrongEventParams, "vote: poll event does not have poll tag")
	}

	deadlineStr, _ := extractTagValueFromPairs(pollTag, "ttl")
	deadline, err := strconv.ParseInt(deadlineStr, 10, 64)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: invalid ttl value: %q: %v", deadlineStr, err)
	} else if time.Now().Unix() > deadline {
		return errors.Wrapf(ErrWrongEventParams, "vote: poll is expired")
	}

	var options []int
	err = json.Unmarshal([]byte(e.Content), &options)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: invalid options value: %q: %v", e.Content, err)
	} else if len(options) == 0 {
		return errors.Wrap(ErrWrongEventParams, "vote: options are empty")
	}

	pollType, _ := extractTagValueFromPairs(pollTag, "type")
	pollOptionsStr, _ := extractTagValueFromPairs(pollTag, "options")

	var pollOptions []string
	err = json.Unmarshal([]byte(pollOptionsStr), &pollOptions)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: invalid poll options value: %q: %v", pollOptionsStr, err)
	}

	for _, option := range options {
		if option < 0 || option >= len(pollOptions) {
			return errors.Wrapf(ErrWrongEventParams, "vote: invalid option index: %d", option)
		}
	}

	if pollType == "single" && len(options) > 1 {
		return errors.Wrapf(ErrWrongEventParams, "vote: single poll can have only one option")
	}

	choises := make(map[int]int)
	for _, option := range options {
		choises[option]++
		if choises[option] > 1 {
			return errors.Wrapf(ErrWrongEventParams, "vote: duplicate option: %d", option)
		}
	}

	return nil
}

func validateKindGiftWrapEvent(e *model.Event) error {
	val := e.GetTag("expiration").Value()
	ts, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "gift wrap: invalid expiration value: %q: %v", val, err)
	}
	if globalConfig != nil && globalConfig.MaxWrappedEventExpiration > 0 && time.Unix(ts, 0).After(time.Now().Add(globalConfig.MaxWrappedEventExpiration)) {
		return errors.Wrapf(ErrWrongEventParams, "gift wrap: expiration is too far in the future, max is %s", globalConfig.MaxWrappedEventExpiration)
	}
	return nil
}

func validateFollowListEvent(e *model.Event) error {
	keys := make(map[string]struct{})
	master := e.GetMasterPublicKey()
	for _, tag := range e.GetTags("p") {
		v := tag.Value()
		switch v {
		case "":
			return errors.Wrap(ErrWrongEventParams, "nip-02: missing public key")
		case master, e.PubKey:
			return errors.Wrapf(ErrWrongEventParams, "tag %q: cannot have the same value as public key or %q", "p", model.CustomIONTagOnBehalfOf)
		}
		keys[v] = struct{}{}
	}

	return nil
}

func Validate(ctx context.Context, e *model.Event) error {
	if e.Kind < 0 || e.Kind > 65535 {
		return errors.Wrapf(ErrUnsupportedKind, "kind: %d", e.Kind)
	}
	if err := validateEventTags(e); err != nil {
		return errors.Wrapf(err, "event: %+v", e)
	}
	if actualSize, maxSize := len(e.Content), globalConfig.MaxContentSizeOf(e.Kind); maxSize > 0 && actualSize > maxSize {
		return errors.Wrapf(ErrWrongEventParams, "content is too long %d, max is %d", actualSize, maxSize)
	}
	if v, ok := KindSupportedTags[e.Kind]; ok {
		if err := v.Execute(e); err != nil {
			return errors.Wrap(ErrWrongEventParams, err.Error())
		}
	}
	switch e.Kind {
	case nostr.KindProfileMetadata:
		return validateKindProfileMetadataEvent(e)
	case nostr.KindTextNote:
		return validateKindTextNoteEvent(ctx, e)
	case nostr.KindDeletion:
		return validateKindDeletionEvent(ctx, e)
	case nostr.KindRepost, nostr.KindGenericRepost:
		return validateKindRepostEvent(ctx, e)
	case nostr.KindFollowList:
		return validateFollowListEvent(e)
	case nostr.KindBadgeAward:
		return validateKindBadgeAwardEvent(e)
	case nostr.KindDirectMessage, nostr.KindSeal:
		return errors.Wrapf(ErrUnsupportedKind, "kind: %d", e.Kind)
	case nostr.KindReactionToWebsite:
		if e.Content != "+" && e.Content != "-" && e.Content != "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
		}
		if rTag := e.Tags.GetFirst([]string{"r"}); rTag == nil || rTag.Value() == "" {
			return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong r tag value: %+v", e)
		}
	case nostr.KindGiftWrap:
		return validateKindGiftWrapEvent(e)
	case model.CustomIONKindPollVote:
		return validatePollVote(ctx, e)
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
		return validateKindProfileBadgesEvent(e)
	case nostr.KindBadgeDefinition:
		return validateKindBadgeDefinitionEvent(e)
	case nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		if len(e.Content) < 1 {
			pubAt := e.GetTag("published_at").Value()
			if val, err := strconv.ParseInt(pubAt, 10, 64); err != nil || val == int64(e.CreatedAt) {
				return errors.Wrap(ErrWrongEventParams, "content is empty or too short")
			}
			for _, tag := range e.Tags {
				switch tag.Key() {
				case "a", model.CustomIONTagOnBehalfOf, "d", "e", "published_at":
				default:
					return errors.Wrapf(ErrWrongEventParams, "tag %q is not allowed", tag.Key())
				}
			}
			// This is a `soft delete`, accept empty content.
		} else {
			if err := validatePostCommunityEvent(ctx, e); err != nil {
				return err
			}
			if err := validateWhoCanReplySettings(ctx, e); err != nil {
				return err
			}
		}
	case model.CustomIONKindCommunityDefinition, model.CustomIONKindCommunityChangeDefinition:
		return validateCustomIONKindCommunityDefinitionEvent(ctx, e)
	case model.CustomIONKindCommunityJoin:
		return validateCustomIONKindCommunityJoinEvent(ctx, e)
	case model.CustomIONKindCommunityOwnershipTransferring:
		return validateCustomIONKindCommunityOwnershipTransferringEvent(ctx, e)
	case model.CustomIONKindCommunityBanUser:
		return validateCustomIONKindCommunityBanUserEvent(ctx, e)
	default:
		if e.IsJobResponse() {
			return validateKindJobResult(e)
		}
	}

	return nil
}

func validateKindTextExtractionJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5301 job text extraction: %+v", e)
}

func validateKindSummarizationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job summarization: %+v", e)
}

func validateKindTranslationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5303 job translation: %+v", e)
}

func validateKindTextGenerationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5304 job text generation: %+v", e)
}

func validateKindImageGenerationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5305 job image generation: %+v", e)
}

func validateKindVideoConversionJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5306 job video conversion: %+v", e)
}

func validateKindVideoTranslationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5307 job video translation: %+v", e)
}

func validateKindImageToVideoConversionJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5308 job image to video conversion: %+v", e)
}

func validateKindTextToSpeechGenerationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5309 job text to speech generation: %+v", e)
}

func validateKindNostrContentDiscoveryJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5301 job nostr content discovery: %+v", e)
}

func validateKindNostrPeopleDiscoveryJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job nostr people discovery: %+v", e)
}

func validateKindNostrContentSearchJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job nostr content search: %+v", e)
}

func validateKindNostrPeopleSearchJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5303 job nostr people search: %+v", e)
}

func validateKindNostrEventCountJob(e *model.Event) error {
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

func validateKindMalwareScanningJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5401 job malware scanning: %+v", e)
}

func validateKindNostrEventTimeStampingJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5402 job nostr event timestamping: %+v", e)
}

func validateKindOpReturnCreationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5901 job nostr event timestamping: %+v", e)
}

func validateKindNostrEventPublishScheduleJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5902 job nostr event publish schedule: %+v", e)
}

func validateKindRelayListMetadataEvent(e *model.Event) error {
	rTags := e.Tags.GetAll([]string{"r"})
	if len(rTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-65, no required r tags: %+v", e)
	}
	if e.Content != "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-65, content is not used: %+v", e)
	}
	for _, tag := range rTags {
		if len(tag) < 2 || (len(tag) > 2 && (tag[2] != "" && tag[2] != model.RelayListReadMarker && tag[2] != model.RelayListWriteMarker)) {
			return errors.Wrapf(ErrWrongEventParams, "nip-65, wrong read/write marker for r tag: %+v", e)
		}
	}

	return nil
}

func validateKindBadgeDefinitionEvent(e *model.Event) error {
	if dTag := e.Tags.GetD(); dTag == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, no required d tag: %+v", e)
	}

	return nil
}

func validateKindBadgeAwardEvent(e *model.Event) error {
	if len(e.Tags.GetAll([]string{"a"})) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: a tag is required")
	} else if err := validateATags(e, nostr.KindBadgeDefinition); err != nil {
		return errors.Wrap(err, "nip-58")
	}
	if len(e.Tags.GetAll([]string{"p"})) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: p tag is required")
	}
	return nil
}

func validateKindProfileBadgesEvent(e *model.Event) error {
	if dTag := e.Tags.GetD(); dTag != model.ProfileBadgesIdentifier {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: no required d tag/wrong value: expected %q, got %q", model.ProfileBadgesIdentifier, dTag)
	}
	if err := validateATags(e, nostr.KindBadgeDefinition); err != nil {
		return errors.Wrap(err, "nip-58")
	}
	if alen, elen := len(e.Tags.GetAll([]string{"a"})), len(e.Tags.GetAll([]string{"e"})); alen != elen {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: e/a tag mismatch: a len %d, e len %d", alen, elen)
	}
	return nil
}

func validateKindDeletionEvent(ctx context.Context, e *model.Event) error {
	if len(e.Tags) == 0 {
		// Account deletion request.
		return nil
	}

	if eTags, kTags := e.GetTags("e"), e.GetTags("k"); len(eTags) != len(kTags) {
		return errors.Wrapf(ErrWrongEventParams, "nip-09: deletion request should include k tag for the each event: found %d e tags and %d k tags", len(eTags), len(kTags))
	}

	if err := validateDeleteCommunityEvents(ctx, e); err != nil {
		return err
	}

	return nil
}

func validateKindReportEvent(e *model.Event) error {
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
	return reportType == "" || reportType == model.TagReportTypeNudity || reportType == model.TagReportTypeMalware ||
		reportType == model.TagReportTypeProfanity || reportType == model.TagReportTypeIllegal || reportType == model.TagReportTypeSpam ||
		reportType == model.TagReportTypeImpersonation || reportType == model.TagReportTypeOther
}

func validateKindLabelingEvent(e *model.Event) error {
	if e.Tags.GetFirst([]string{"e"}) == nil && e.Tags.GetFirst([]string{"p"}) == nil && e.Tags.GetFirst([]string{"a"}) == nil &&
		e.Tags.GetFirst([]string{"r"}) == nil && e.Tags.GetFirst([]string{"t"}) == nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, missing one of required tags: %+v", e)
	}

	return validateLabelTags(e)
}

func validateLabelTags(e *model.Event) error {
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
	if labelNamespaceTag == nil && (*labelTag)[2] != model.UserGeneratedContentNamespace {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, empty L tag, namespace of l tag should be ugc: %+v", e)
	}
	if labelNamespaceTag != nil && (*labelTag)[2] != (*labelNamespaceTag)[1] {
		return errors.Wrapf(ErrWrongEventParams, "nip-32, l -> L tag values mismatch: %+v", e)
	}

	return nil
}

func validateKindProfileMetadataEvent(e *model.Event) error {
	if !json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: content field should be stringified json: %+v", e)
	}
	var parsedContent model.ProfileMetadataContent
	if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-01,nip-24: wrong json fields for: %+v", e)
	}
	if parsedContent.Name == "" || parsedContent.DisplayName == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: there are no required content fields: %+v", e)
	}

	return nil
}

func validateKindTextNoteEvent(ctx context.Context, e *model.Event) error {
	if json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: content field should be plain text: %q", e.Content)
	}

	if err := validateLabelTags(e); err != nil {
		return errors.Wrap(err, "nip-32: label tags are invalid for event")
	}

	pTags := e.GetTags("p")
	eTags := e.GetTags("e")
	if len(eTags) > 0 {
		for _, tag := range eTags {
			if len(tag) < 2 {
				return errors.Wrap(ErrWrongEventParams, "nip-10: 'e' tag does not contain any event id")
			}
			if len(tag) >= 3 {
				if tag[3] != model.TagMarkerRoot && tag[3] != model.TagMarkerReply && tag[3] != model.TagMarkerMention {
					return errors.Wrapf(ErrWrongEventParams, "nip-10: wrong tag marker param: %v, want root/reply/mention", tag[3])
				}
			}
		}
	}
	if len(pTags) > 0 {
		if len(eTags) == 0 {
			return errors.Wrap(ErrWrongEventParams, "wrong nip-10: no 'e' tags while p tag exist")
		}
		for _, tag := range pTags {
			if len(tag) == 1 {
				return errors.Wrap(ErrWrongEventParams, "nip-10: 'p' tag does not contain any pubkey who is involved in reply thread")
			}
		}
	}
	if err := validatePostCommunityEvent(ctx, e); err != nil {
		return err
	}
	if err := validateWhoCanReplySettings(ctx, e); err != nil {
		return err
	}

	return nil
}

func validateWhoCanReplySettings(ctx context.Context, e *model.Event) error {
	rootPost, err := findRootPost(ctx, e)
	if err != nil {
		return err
	}
	if rootPost == nil {
		return nil
	}

	settingsTag := getLatestSettingsTag(rootPost, model.WhoCanReplySettings)
	if settingsTag == nil || (*settingsTag)[1] != model.WhoCanReplySettings {
		return nil
	}

	values := strings.Split((*settingsTag)[2], ",")
	passed := false

	for _, value := range values {
		if passed, err = checkWhoCanReplySettings(ctx, value, rootPost, e); err != nil {
			return err
		}
		if passed {
			break
		}
	}

	if !passed {
		return errors.Wrapf(ErrActionForbidden, "reply can be added only by users with settings %+v for event: %v", settingsTag, e.ID)
	}
	return nil
}

func checkWhoCanReplySettings(ctx context.Context, value string, rootPost, e *model.Event) (bool, error) {
	switch {
	case value == model.FollowingWhoCanReplySettings:
		return checkFollowingWhoCanReplySettings(ctx, rootPost, e)

	case value == model.MentionWhoCanReplySettings:
		return checkMentionWhoCanReplySettings(rootPost, e)

	case strings.HasPrefix(value, model.BadgeWhoCanReplySettingsPrefix):
		return checkBadgeWhoCanReplySettings(ctx, value, e)

	default:
		return false, nil
	}
}

func checkFollowingWhoCanReplySettings(ctx context.Context, rootPost, e *model.Event) (bool, error) {
	events := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []nostr.Filter{
			{
				Authors: []string{rootPost.GetMasterPublicKey()},
				Kinds:   []int{nostr.KindFollowList},
				Tags:    model.TagMap{}.SetLiterals("p", e.GetMasterPublicKey()),
			},
		},
	})
	for _, err := range events {
		if err != nil {
			return false, err
		}

		return true, nil
	}
	return false, nil
}

func checkMentionWhoCanReplySettings(rootPost, e *model.Event) (bool, error) {
	words := strings.Split(rootPost.Content, " ")
	for _, word := range words {
		if !strings.HasPrefix(word, "npub") {
			continue
		}
		prefix, pubkey, err := nip19.Decode(word)
		if err != nil {
			return false, errors.Wrapf(ErrWrongEventParams, "can't decode the content: %v", e.Content)
		}
		if prefix == "npub" && pubkey.(string) == e.GetMasterPublicKey() {
			return true, nil
		}
	}
	return false, nil
}

func checkBadgeWhoCanReplySettings(ctx context.Context, value string, e *model.Event) (bool, error) {
	splitted := strings.Split(value, "|")
	if len(splitted) != 2 {
		return false, errors.Wrapf(ErrWrongEventParams, "wrong badge who can reply settings: %v", value)
	}
	events := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []nostr.Filter{
			{
				Authors: []string{e.GetMasterPublicKey()},
				Kinds:   []int{nostr.KindProfileBadges},
				Tags:    model.TagMap{}.SetLiterals("a", splitted[1]),
			},
		},
	})
	for _, err := range events {
		if err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func findRootPost(ctx context.Context, e *model.Event) (*model.Event, error) {
	filter, err := createRootPostFilter(e)
	if err != nil {
		return nil, err
	}
	if filter == nil {
		return nil, nil
	}

	rootPosts := query.GetStoredEvents(ctx, &model.Subscription{Filters: nostr.Filters{*filter}})
	for ev, err := range rootPosts {
		if err != nil {
			return nil, err
		}
		if e.Kind == ev.Kind {
			return ev, nil
		}
	}

	return nil, nil
}

func createRootPostFilter(e *model.Event) (*nostr.Filter, error) {
	for _, tag := range e.GetTags("a") {
		if !isRootTag(tag) {
			continue
		}
		parts := strings.Split(tag.Value(), ":")
		if len(parts) != 3 {
			return nil, errors.Wrapf(ErrWrongEventParams, "invalid tag value: %v", tag.Value())
		}
		kind, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}

		return &nostr.Filter{
			Kinds:   []int{kind},
			Authors: []string{parts[1]},
			Tags:    nostr.TagMap{}.SetLiterals("d", parts[2]),
		}, nil

	}
	for _, tag := range e.GetTags("e") {
		if !isRootTag(tag) {
			continue
		}

		return &nostr.Filter{
			IDs:   []string{tag.Value()},
			Kinds: []int{nostr.KindTextNote, nostr.KindArticle, nostr.KindDraftArticle, nostr.KindReply, nostr.KindRepost},
		}, nil
	}
	return nil, nil
}

func isRootTag(tag nostr.Tag) bool {
	return len(tag) >= 4 && (tag)[3] == model.TagMarkerRoot
}

func validateKindRepostEvent(ctx context.Context, e *model.Event) error {
	var repostedEvent model.Event

	if !json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: content field should be stringified json: %q", e.Content)
	}
	if err := repostedEvent.UnmarshalJSON([]byte(e.Content)); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong json fields: %v", err)
	} else if err := Validate(ctx, &repostedEvent); err != nil {
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

	if repostedEvent.IsAddressable() || repostedEvent.IsReplaceable() {
		if eTag := e.GetTag("a"); eTag.Value() != repostedEvent.Address() {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include a tag with address of the note: found %q, expected %q", eTag.Value(), repostedEvent.Address())
		}
	} else {
		if eTag := e.GetTag("e"); eTag.Value() != repostedEvent.ID {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include e tag with id of the note: found %q, expected %q", eTag.Value(), repostedEvent.ID)
		}
	}

	if pTag := e.GetTag("p"); pTag.Value() != repostedEvent.GetMasterPublicKey() {
		return errors.Wrapf(ErrWrongEventParams,
			"nip-18: repost must include p tag with pubkey of the event being reposted: found %q, expected %q",
			pTag.Value(), repostedEvent.GetMasterPublicKey())
	}
	if err := validatePostCommunityEvent(ctx, e); err != nil {
		return err
	}
	if err := validateWhoCanReplySettings(ctx, e); err != nil {
		return err
	}

	return nil
}

func validateKindJobResult(e *model.Event) error {
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

func validateKindFeedbackJob(e *model.Event) error {
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

func validateCustomIONKindCommunityDefinitionEvent(ctx context.Context, e *model.Event) error {
	hTag := e.GetTag(model.CustomIONTagCommunity)
	if hTag == nil {
		return errors.Wrap(ErrWrongEventParams, "community must have h tag")
	}

	pTags := e.GetTags("p")
	for _, tag := range pTags {
		if len(tag) < 4 || (tag[3] != string(model.ModeratorRole) && tag[3] != string(model.AdminRole) && tag[3] != "") {
			return errors.Wrapf(ErrWrongEventParams, "p tag must specify a valid role (moderator or admin): %v", tag)
		}
	}

	if e.GetTag("open") != nil && e.GetTag("closed") != nil {
		return errors.Wrap(ErrWrongEventParams, "community cannot be open and closed at the same time")
	}

	if e.GetTag("public") != nil && e.GetTag("private") != nil {
		return errors.Wrap(ErrWrongEventParams, "community cannot be public and private at the same time")
	}

	for _, aTag := range e.GetTags("a") {
		if splitted := strings.Split(aTag.Value(), ":"); len(splitted) != 3 || splitted[0] != strconv.Itoa(model.CustomIONKindCommunityDefinition) {
			return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid a tag: %v", aTag)
		}
	}

	// Here we have two possible cases:
	// - CustomIONKindCommunityDefinition -- addressable community definition, only owner can create/modify it.
	// - CustomIONKindCommunityChangeDefinition -- regular event, with some updates to the already existing community definition, moderators/admins can send it.
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if e.Kind == model.CustomIONKindCommunityDefinition {
		// Must not exist OR must be owned by the same pubkey/master pubkey.
		if err == nil {
			// Existing community definition found, check if the owner is the same.
			if communityDefinitionEvent.GetMasterPublicKey() != e.GetMasterPublicKey() {
				return errors.Wrap(ErrActionForbidden, "community already exists")
			}
		} else if errors.Is(err, ErrNotFound) {
			// Not found, allow the creation.
			return nil
		}
		return err
	}
	// Change definition event, community must exist.
	if err != nil {
		return err
	}
	authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent)
	if authorRole == model.RegularRole {
		return errors.Wrap(ErrActionForbidden, "only admin, owner or moderator can change community definition")
	}
	if authorRole == model.ModeratorRole {
		for _, pTag := range pTags {
			if pTag.Key() == "p" {
				if model.Role(pTag[3]) == model.AdminRole {
					return errors.Wrap(ErrActionForbidden, "moderator can't promote user to admin")
				}
				for _, tag := range communityDefinitionEvent.Tags.GetAll([]string{"p"}) {
					if tag.Key() == "p" && tag.Value() == pTag.Value() && model.Role(tag[3]) == model.AdminRole {
						return errors.Wrap(ErrActionForbidden, "moderator can't demote admin")
					}
				}
			}
		}
		if name := e.GetTag("name"); name != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the name of the community")
		}
		if description := e.GetTag("description"); description != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the description of the community")
		}
		if closed := e.GetTag("closed"); closed != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the open/closed status of the community")
		}
		if open := e.GetTag("open"); open != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the open/closed status of the community")
		}
		if public := e.GetTag("public"); public != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the public/private status of the community")
		}
		if private := e.GetTag("private"); private != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the public/private status of the community")
		}
		if imeta := e.GetTag("imeta"); imeta != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the picture of the community")
		}
		if settings := e.Tags.GetAll([]string{"settings"}); settings != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the settings of the community")
		}
	}

	return nil
}

func validateCustomIONKindCommunityJoinEvent(ctx context.Context, e *model.Event) error {
	var (
		hTag             = e.GetTag(model.CustomIONTagCommunity)
		authorizationTag = e.GetTag("authorization")
	)
	if hTag == nil {
		return errors.Wrapf(ErrWrongEventParams, "community join must have h tag: %+v", e)
	}
	if authorizationTag != nil {
		var parsedContent model.Event
		if err := json.Unmarshal([]byte(authorizationTag.Value()), &parsedContent); err != nil {
			return errors.Wrapf(ErrWrongEventParams, "wrong authorization content: %+v", e)
		}
		if parsedContent.Kind != model.CustomIONKindCommunityJoin {
			return errors.Wrapf(ErrWrongEventParams, "wrong authorization content kind: %+v", e)
		}
		expirationTag := parsedContent.GetTag("expiration")
		if expirationTag == nil {
			return errors.Wrapf(ErrWrongEventParams, "community join must have an expiration tag for authorization event: %+v", e)
		}
		expirationTime, err := strconv.ParseInt(expirationTag.Value(), 10, 64)
		if err != nil {
			return errors.Wrapf(ErrWrongEventParams, "wrong expiration tag value, %v", err.Error())
		}
		currentTime := time.Now().Unix()
		if currentTime > expirationTime {
			return errors.Wrapf(ErrWrongEventParams, "authorization event has expired: %v", expirationTime)
		}
		if ok, err := parsedContent.CheckSignature(); err != nil || !ok {
			return errors.Wrapf(ErrWrongEventParams, "wrong authorization signature: %v", err.Error())
		}
	}
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	openTag := communityDefinitionEvent.GetTag("open")
	publicTag := communityDefinitionEvent.GetTag("public")
	if openTag != nil && publicTag != nil && e.GetTag("authorization") != nil {
		return errors.Wrap(ErrWrongEventParams, "wrong join event, authorization tag is not required for public open community")
	}
	if closedTag := communityDefinitionEvent.GetTag("closed"); closedTag != nil {
		if authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent); authorRole != model.RegularRole {
			return nil
		}
		if authorizationTag == nil {
			return errors.Wrap(ErrActionForbidden, "can't join closed community")
		}
		var parsedAuthorizationEvent model.Event
		if err := json.Unmarshal([]byte(authorizationTag.Value()), &parsedAuthorizationEvent); err != nil {
			return errors.Wrap(ErrActionForbidden, "wrong authorization event")
		}
		if err := Validate(ctx, &parsedAuthorizationEvent); err != nil {
			return err
		}
		if authorizationRole := model.GetCommunityRoleByPubkey(parsedAuthorizationEvent.GetMasterPublicKey(), communityDefinitionEvent); authorizationRole == model.RegularRole {
			return errors.Wrap(ErrActionForbidden, "user not authorized to join this community")
		}
	}

	return nil
}

func validateCustomIONKindCommunityOwnershipTransferringEvent(ctx context.Context, e *model.Event) error {
	var (
		aTags         = e.Tags.GetAll([]string{"a"})
		hTag          = e.GetTag(model.CustomIONTagCommunity)
		pTags         = e.Tags.GetAll([]string{"p"})
		expirationTag = e.GetTag("expiration")
		currentTime   = time.Now().Unix()
	)
	for _, aTag := range aTags {
		if len(aTag) < 2 {
			return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid a tag: %+v", e)
		}
		if splitted := strings.Split(aTag.Value(), ":"); len(splitted) != 3 || splitted[0] != fmt.Sprint(model.CustomIONKindCommunityDefinition) {
			return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid a tag: %+v", e)
		}
	}
	if hTag == nil {
		return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid h tag: %+v", e)
	}
	if len(pTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "community ownership must have at least one p tag: %+v", e)
	}
	if expirationTag == nil {
		return errors.Wrapf(ErrWrongEventParams, "community ownership must have an expiration tag: %+v", e)
	}
	expirationTime, err := strconv.ParseInt(expirationTag.Value(), 10, 64)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong expiration tag value: %+v", e)
	}
	if currentTime > expirationTime {
		return errors.Wrapf(ErrWrongEventParams, "community ownership transferring event has expired: %+v", e)
	}
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	if authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent); authorRole != model.OwnerRole {
		return errors.Wrap(ErrActionForbidden, "only owner of the community can transfer ownership")
	}

	return nil
}

func validateCustomIONKindCommunityBanUserEvent(ctx context.Context, e *model.Event) error {
	var (
		hTag  = e.GetTag(model.CustomIONTagCommunity)
		pTags = e.Tags.GetAll([]string{"p"})
	)
	if hTag == nil {
		return errors.Wrap(ErrWrongEventParams, "community ban must have h tag")
	}
	if len(pTags) == 0 {
		return errors.Wrap(ErrWrongEventParams, "community ban must have at least one p tag")
	}
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent)
	if authorRole == model.RegularRole {
		return errors.Wrap(ErrActionForbidden, "only admin, owner or moderator can ban user")
	}
	for _, pTag := range pTags {
		if pTag.Key() == "p" {
			if pTag.Value() == e.GetMasterPublicKey() {
				return errors.Wrap(ErrActionForbidden, "admin/moderator can't ban himself")
			}
			if pTag.Value() == communityDefinitionEvent.GetMasterPublicKey() {
				return errors.Wrap(ErrActionForbidden, "owner of the community can't be banned")
			}
			toBanRole := model.GetCommunityRoleByPubkey(pTag.Value(), communityDefinitionEvent)
			if toBanRole == model.AdminRole && authorRole == model.ModeratorRole {
				return errors.Wrap(ErrActionForbidden, "moderator can't ban admin")
			}
		}
	}

	return nil
}

func validateIMetaTag(tag nostr.Tag) error {
	if tag == nil {
		return nil
	}

	values, err := model.ParseIMeta(tag)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "invalid imeta: %v", err.Error())
	}
	// Check for all required values.
	for key, state := range SupportedIMetaKeys {
		if state == tagStateRequired && values[key] == "" {
			return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: %s", key)
		}
	}

	// Either x or ox should be present and they should be hex.
	if values["x"] == "" && values["ox"] == "" {
		return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: x or ox")
	}

	// Check for values correctness.
	for key, value := range values {
		if _, ok := SupportedIMetaKeys[key]; !ok {
			return errors.Wrapf(ErrWrongEventParams, "not supported imeta value: %s", key)
		}
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

func validateSettingsTag(kind int, tag nostr.Tag) error {
	if len(tag) < 4 {
		return errors.Wrapf(ErrWrongEventParams, "settings tag is incomplete: %+v", tag)
	}
	settingType := tag[1]
	value := tag[2]
	timestamp := tag[3]
	if _, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		return errors.Wrapf(err, "invalid timestamp in settings tag: %+v", tag)
	}
	switch settingType {
	case model.CommentsEnabledSettings:
		if kind != model.CustomIONKindCommunityDefinition && kind != model.CustomIONKindCommunityChangeDefinition {
			return errors.Wrapf(ErrWrongEventParams, "comments_enabled can be set only for 31750 kind: %+v", tag)
		}
		if value != "true" && value != "false" {
			return errors.Wrapf(ErrWrongEventParams, "comments_enabled must be true or false: %+v", tag)
		}
	case model.RoleRequiredForPostingSettings:
		if kind != model.CustomIONKindCommunityDefinition && kind != model.CustomIONKindCommunityChangeDefinition {
			return errors.Wrapf(ErrWrongEventParams, "role_required_for_posting can be set only for 31750 kind: %+v", tag)
		}
		if value != string(model.AdminRole) && value != string(model.ModeratorRole) && value != "" {
			return errors.Wrapf(ErrWrongEventParams, "role_required_for_posting must be admin or moderator: %+v", tag)
		}
	case model.WhoCanReplySettings:
		accept := map[int]struct{}{
			nostr.KindTextNote:                  {},
			nostr.KindArticle:                   {},
			nostr.KindDraftArticle:              {},
			model.CustomIONKindEditableTextNote: {},
		}
		if _, ok := accept[kind]; !ok {
			return errors.Wrapf(ErrWrongEventParams, "who_can_reply cannot be set for kind: %d: %+v", kind, tag)
		}
		values := strings.Split(value, ",")
		for _, v := range values {
			if !strings.HasPrefix(v, "following") && !strings.HasPrefix(v, "mentioned") && !strings.HasPrefix(v, "badge|") {
				return errors.Wrapf(ErrWrongEventParams, "who_can_reply contains invalid value: %s", v)
			}
			if strings.HasPrefix(v, "badge|") {
				parts := strings.Split(v, "|")
				if len(parts) != 2 {
					return errors.Wrapf(ErrWrongEventParams, "invalid badge format in who_can_reply: %s", v)
				}
			}
		}
	default:
		return errors.Wrapf(ErrUnsupportedTag, "unsupported settings tag: %s", settingType)
	}

	return nil
}

func validateEventTags(e *model.Event) error {
	var bTag string
	currentTags := make(map[string]int)
	pTags := make(map[string]int)
	kindValidator, known := KindSupportedTags[e.Kind]
	for _, tag := range e.Tags {
		if data, ok := kindValidator.Tags[tag.Key()]; known && !ok {
			return errors.Wrapf(ErrUnsupportedTag, "tag: %v", tag)
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
			if val, err := uuid.Parse(tag.Value()); err != nil {
				return errors.Wrapf(ErrWrongEventParams, "tag %v: error: %q: %v", model.CustomIONTagCommunity, tag.Value(), err)
			} else if version := val.Version(); version != 0x7 {
				return errors.Wrapf(ErrWrongEventParams, "tag %v: wrong UUID version: %#02x, expected %#02x", model.CustomIONTagCommunity, version, 0x7)
			}
		case model.CustomIONTagPoll:
			if err := validatePollTag(tag); err != nil {
				return err
			}
		case "expiration", "published_at", "editing_ended_at":
			v, err := strconv.ParseInt(tag.Value(), 10, 64)
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "tag: %s: should be uint: %v", tag.Key(), err)
			} else if v < 0 {
				return errors.Wrapf(ErrWrongEventParams, "tag: %s: should be positive: %d", tag.Key(), v)
			}
		case "settings":
			if err := validateSettingsTag(e.Kind, tag); err != nil {
				return errors.Join(ErrUnsupportedTag, err)
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

func tagsTableRequired(tags ...string) kindValidator {
	return newKindValidatorBuilder().Required(tags...).Build()
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

func (t *kindValidatorBuilder) Validate(f func(e *model.Event) error) *kindValidatorBuilder {
	t.Validator.Validate = f

	return t
}

func (t *kindValidatorBuilder) Build() kindValidator {
	return t.Validator
}

func (v *kindValidator) Execute(e *model.Event) (err error) {
	if v.Flags&kindValidatorFlagContentRequired != 0 && e.Content == "" {
		err = errors.Join(err, ErrContentEmpty)
	}
	if v.Validate != nil {
		err = errors.Join(err, v.Validate(e))
	}
	return err
}
