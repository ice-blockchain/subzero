// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"crypto/rand"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestHandleTokenizedCommunityEvent(t *testing.T) {
	t.Parallel()

	deviceTagsOwner := model.Tags{
		{"t", "ios"},
		{"d", rand.Text()},
		{"token", "testToken"},
	}

	deviceTagsSubscriber := model.Tags{
		{"t", "ios"},
		{"d", rand.Text()},
		{"token", "testToken2"},
	}

	t.Run("CreatorTokenCreated", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 31175 with t=community_token_action and k=0 (KindProfileMetadata) -> CreatorTokenCreated
		event := &model.Event{
			Event: nostr.Event{
				ID:     "creatorTokenCreatedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityDefinition,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"t", "community_token_action"},
					{"k", strconv.Itoa(nostr.KindProfileMetadata)},
					{"p", targetPubKey},
					{"a", "0:" + targetPubKey + ":"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeCreatorTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeCreatorTokenCreated].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneCreatorTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneCreatorTokenCreated].Body, notification.Body)
			}
		}
	})

	t.Run("ContentTokenCreated", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 31175 with t=community_token_action and k=1 (KindTextNote) -> ContentTokenCreated
		event := &model.Event{
			Event: nostr.Event{
				ID:     "contentTokenCreatedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityDefinition,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"t", "community_token_action"},
					{"k", strconv.Itoa(nostr.KindTextNote)},
					{"p", targetPubKey},
					{"a", "1:" + targetPubKey + ":something"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenCreated].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenCreated].Body, notification.Body)
			}
		}
	})

	t.Run("CreatorTokenSwapped", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityAction}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 1175 with tx_type=buy and a tag starting with "0:" (KindProfileMetadata) -> CreatorTokenSwapped
		event := &model.Event{
			Event: nostr.Event{
				ID:     "creatorTokenSwappedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityAction,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"a", strconv.Itoa(nostr.KindProfileMetadata) + ":" + targetPubKey + ":"},
					{"tx_type", "buy"},
					{"network", "ethereum"},
					{"bonding_curve_address", "0xD76b5c2A23ef78368d8E34288B5b65D616B746aE"},
					{"token_address", "0xb302472D9526AE979F3134097C873d85E4a4502c"},
					{"tx_address", "0xae805fDC38dFf250b2597bC2b20ee9Bc2390D156"},
					{"token_symbol", "JDOE"},
					{"tx_amount", "23630", "ION"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeCreatorTokenSwapped].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeCreatorTokenSwapped].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneCreatorTokenSwapped].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneCreatorTokenSwapped].Body, notification.Body)
			}
		}
	})

	t.Run("ContentTokenSwapped", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityAction}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 1175 with tx_type=buy and a tag starting with "1:" (KindTextNote) -> ContentTokenSwapped
		event := &model.Event{
			Event: nostr.Event{
				ID:     "contentTokenSwappedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityAction,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"a", strconv.Itoa(nostr.KindTextNote) + ":" + targetPubKey + ":something"},
					{"tx_type", "buy"},
					{"network", "ethereum"},
					{"bonding_curve_address", "0xD76b5c2A23ef78368d8E34288B5b65D616B746aE"},
					{"token_address", "0xb302472D9526AE979F3134097C873d85E4a4502c"},
					{"tx_address", "0xae805fDC38dFf250b2597bC2b20ee9Bc2390D156"},
					{"token_symbol", "CONTENT"},
					{"tx_amount", "1000", "ION"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)
	})

	t.Run("ContentTokenCreated for kind 1 comment", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 31175 with t=community_token_action and k=1 for a comment (has e root tag)
		event := &model.Event{
			Event: nostr.Event{
				ID:     "commentTokenCreatedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityDefinition,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"t", "community_token_action"},
					{"k", strconv.Itoa(nostr.KindTextNote)},
					{"p", targetPubKey},
					{"e", "comment_event_id"},
				},
			},
		}
		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenCreated].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenCreated].Body, notification.Body)
			}
		}
	})

	t.Run("ContentTokenCreated for kind 30175 comment", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 31175 with t=community_token_action and k=30175 for an editable comment (has a root tag)
		event := &model.Event{
			Event: nostr.Event{
				ID:     "editableCommentTokenCreatedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityDefinition,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"t", "community_token_action"},
					{"k", strconv.Itoa(model.CustomIONKindEditableTextNote)},
					{"p", targetPubKey},
					{"a", strconv.Itoa(model.CustomIONKindEditableTextNote) + ":" + targetPubKey + ":comment-d-tag"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenCreated].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenCreated].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenCreated].Body, notification.Body)
			}
		}
	})

	t.Run("ContentTokenSwapped for kind 1 comment", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityAction}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 1175 with tx_type=buy for a comment token
		event := &model.Event{
			Event: nostr.Event{
				ID:     "commentTokenSwappedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityAction,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"a", strconv.Itoa(nostr.KindTextNote) + ":" + targetPubKey + ":comment_id"},
					{"tx_type", "buy"},
					{"network", "bsc"},
					{"bonding_curve_address", "0xD76b5c2A23ef78368d8E34288B5b65D616B746aE"},
					{"token_address", "0xb302472D9526AE979F3134097C873d85E4a4502c"},
					{"tx_address", "0xae805fDC38dFf250b2597bC2b20ee9Bc2390D156"},
					{"token_symbol", "COMMENT"},
					{"tx_amount", "500", "ION"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenSwapped].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenSwapped].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenSwapped].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenSwapped].Body, notification.Body)
			}
		}
	})

	t.Run("ContentTokenSwapped for kind 30175 comment", func(t *testing.T) {
		_, targetPubKey := model.GenerateKeyPair()
		_, senderPubKey := model.GenerateKeyPair()
		deviceID := rand.Text()

		filters := model.Filters{
			{Kinds: []int{model.CustomIONKindTokenizedCommunityAction}},
		}

		pm := helperNewManager(t)
		ownerDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, targetPubKey, deviceID, deviceTagsOwner, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), ownerDeviceEvent))

		_, subscriberPubKey := model.GenerateKeyPair()
		subscriberDeviceID := rand.Text()
		subscriberDeviceEvent := helperCreateTestDeviceRegistrationEvent(t, subscriberPubKey, subscriberDeviceID, deviceTagsSubscriber, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), subscriberDeviceEvent))

		// Kind 1175 with tx_type=buy for an editable comment token
		event := &model.Event{
			Event: nostr.Event{
				ID:     "editableCommentTokenSwappedEvent",
				Kind:   model.CustomIONKindTokenizedCommunityAction,
				PubKey: senderPubKey,
				Tags: model.Tags{
					{"a", strconv.Itoa(model.CustomIONKindEditableTextNote) + ":" + targetPubKey + ":comment_d_tag"},
					{"tx_type", "buy"},
					{"network", "bsc"},
					{"bonding_curve_address", "0xD76b5c2A23ef78368d8E34288B5b65D616B746aE"},
					{"token_address", "0xb302472D9526AE979F3134097C873d85E4a4502c"},
					{"tx_address", "0xae805fDC38dFf250b2597bC2b20ee9Bc2390D156"},
					{"token_symbol", "EDITCOMMENT"},
					{"tx_amount", "750", "ION"},
				},
			},
		}

		notifications, err := pm.handleTokenizedCommunityEvent(t.Context(), event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			if notification.Target == ownerDeviceEvent {
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenSwapped].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeContentTokenSwapped].Body, notification.Body)
			} else {
				require.Equal(t, subscriberDeviceEvent, notification.Target)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenSwapped].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeSomeoneContentTokenSwapped].Body, notification.Body)
			}
		}
	})
}
