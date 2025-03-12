// SPDX-License-Identifier: ice License 1.0

package model

import (
	"errors"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

type (
	Tag          = nostr.Tag
	Tags         = nostr.Tags
	TagMap       = nostr.TagMap
	TagValues    = nostr.TagValues
	Timestamp    = nostr.Timestamp
	Filter       = nostr.Filter
	Filters      = nostr.Filters
	Kind         = int
	Subscription struct {
		SubscriptionID string
		Filters        Filters
		Reduce         func(*Event) (skip bool)
		OneShot        bool
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
	ErrUnsupportedAlg       = errors.New("unsupported signature/key algorithm combination")
	ErrOnBehalfAccessDenied = errors.New("on-behalf access denied")
	ErrNotAuthorized        = errors.New("unauthorized")
)

const (
	CustomIONKindPollVote          = 1754
	CustomIONKindFundReceive       = 1755
	CustomIONKindFundSendNotify    = 1756
	CustomIONKindAttestation       = 10_100
	CustomIONKindRelayListMetadata = 20_002
	CustomIONKindEditableTextNote  = 30_175

	KindDVMCountResponse = 6400
)

const (
	CustomIONTagOnBehalfOf   = "b"
	CustomIONTagPoll         = "poll"
	CustomIONTagAddressableQ = "Q"
	CustomIONTagCommunity    = "h"
)

const (
	CustomIONAttestationKindActive   = "active"
	CustomIONAttestationKindRevoked  = "revoked"
	CustomIONAttestationKindInactive = "inactive"
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

	CommentsEnabledSettings        string = "comments_enabled"
	RoleRequiredForPostingSettings string = "role_required_for_posting"
	WhoCanReplySettings            string = "who_can_reply"

	FollowingWhoCanReplySettings   string = "following"
	MentionWhoCanReplySettings     string = "mentioned"
	BadgeWhoCanReplySettingsPrefix string = "badge"

	ExtensionTextMRF = `most relevant followers`

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
	Role                   string
	ProfileMetadataContent struct {
		RegisteredAt            Timestamp         `json:"registered_at" `
		Name                    string            `json:"name" example:"username"`
		About                   string            `json:"about" example:"about"`
		Picture                 string            `json:"picture" example:"https://example.com/pic.jpg"`
		DisplayName             string            `json:"display_name" example:"John Deer"`
		Website                 string            `json:"website" example:"https://ice.io"`
		Banner                  string            `json:"banner" example:"https://example.com/banner.jpg"`
		Location                string            `json:"location" example:"New York, USA"`
		Category                string            `json:"category" example:"Crypto"`
		WhoCanMessageYou        string            `json:"who_can_message_you" example:"friends"`
		WhoCanInviteYouToGroups string            `json:"who_can_invite_you_to_groups" example:"friends"`
		Wallets                 map[string]string `json:"wallets"`
		Bot                     bool              `json:"bot" example:"false"`
	}
)

const (
	DVMJobResultExpiration = 5 * time.Minute
)
