// SPDX-License-Identifier: ice License 1.0

package model

import (
	"errors"
	"math"
	"time"

	"github.com/goccy/go-json"
	"github.com/nbd-wtf/go-nostr"
)

type (
	Tag            = nostr.Tag
	Tags           = nostr.Tags
	Timestamp      = nostr.Timestamp
	Kind           = int
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
	CustomIONKindPollVote                     = 1754
	CustomIONKindFundReceive                  = 1755
	CustomIONKindFundSendNotify               = 1756
	CustomIONKindUserBlock                    = 1757
	CustomIONKindArchiveConversation          = 2175
	CustomIONKindMute                         = 3175
	CustomIONKindAttestation                  = 10_100
	CustomIONKindRelayListMetadata            = 20_002
	CustomIONKindEphemeralEmbedding           = 21_750
	CustomIONKindDirectMessage                = 30_014
	CustomIONKindEditableTextNote             = 30_175
	CustomIONKindDeviceRegistration           = 31_751
	CustomIONKindTokenizedCommunityDefinition = 31_175
	CustomIONKindTokenizedCommunityAction     = 1175
	KindDVMCountResponse                      = 6400

	CustomIONKindRepostOfArticle                      = 16_30023
	CustomIONKindRepostOfEditableTextNote             = 16_30175
	CustomIONKindRepostOfTokenizedCommunityDefination = 16_31175
	CustomIONKindRepostOfTokenizedCommunityAction     = 16_1175

	// TODO: change to proper value.
	CustomIONSystemMessage = 999_999

	KindAny = math.MaxUint16
)

const (
	CustomIONTagOnBehalfOf   = "b"
	CustomIONTagPoll         = "poll"
	CustomIONTagAddressableQ = "Q"
	CustomIONTagCommunity    = "h"
	CustomIONTagRichText     = "rich_text"
	CustomIONTagPMO          = "pmo" // Positional markdown override.
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

	TagSuffixUsernameProof = `username_proof_of_ownership`

	QuillDeltaProtocol string = "quill_delta"

	LangISO = `ISO-639-1`

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

const (
	ConsensusReplayCtxKey = "replay"
)

const (
	DeviceTokenOSAndroid = "android"
	DeviceTokenOSIOS     = "ios"
	DeviceTokenOSWeb     = "web"
)

type (
	JobFeedbackStatus      string
	Role                   string
	ProfileMetadataContent struct {
		IONContentNFTCollections map[IONContentNFTCollectionName]IONContentNFTCollectionMetadata `json:"ion_content_nft_collections" `
		Wallets                  map[string]string                                               `json:"wallets"`
		Name                     string                                                          `json:"name" example:"username"`
		About                    string                                                          `json:"about" example:"about"`
		Picture                  string                                                          `json:"picture" example:"https://example.com/pic.jpg"`
		DisplayName              string                                                          `json:"display_name" example:"John Deer"`
		Website                  string                                                          `json:"website" example:"https://ice.io"`
		Banner                   string                                                          `json:"banner" example:"https://example.com/banner.jpg"`
		Location                 string                                                          `json:"location" example:"New York, USA"`
		Category                 string                                                          `json:"category" example:"Crypto"`
		WhoCanMessageYou         string                                                          `json:"who_can_message_you" example:"friends"`
		WhoCanInviteYouToGroups  string                                                          `json:"who_can_invite_you_to_groups" example:"friends"`
		RegisteredAt             Timestamp                                                       `json:"registered_at" `
		Bot                      bool                                                            `json:"bot" example:"false"`
	}
	IONContentNFTCollectionName     string
	IONContentNFTCollectionMetadata struct {
		Address   string `json:"address" example:"0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290"`
		CreatedBy string `json:"created_by" example:"0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4"`
	}
)

const (
	DVMJobResultExpiration = 5 * time.Minute
)

func (meta ProfileMetadataContent) String() string {
	data, err := json.Marshal(meta)
	if err != nil {
		return err.Error()
	}
	return string(data)
}
