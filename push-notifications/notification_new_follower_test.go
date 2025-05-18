// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/validation"
)

func helperCreateFollowListEvent(t *testing.T, id string, pubKey string, followedPubKeys []string) *model.Event {
	t.Helper()

	tags := nostr.Tags{}
	for _, followedPubKey := range followedPubKeys {
		tags = append(tags, nostr.Tag{"p", followedPubKey})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    nostr.KindFollowList,
			Content: "Follow list",
			Tags:    tags,
		},
	}
}

func TestGetLastFollowerPubkey(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{}

	emptyTagsEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+uuid.NewString(),
		"follower_pubkey",
		[]string{},
	)

	recipient := pm.getLastFollowerPubkey(emptyTagsEvent, nil)
	require.Empty(t, recipient, "Recipient should be empty when no p-tags")

	singleTagEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+uuid.NewString(),
		"follower_pubkey",
		[]string{"follower_pubkey"},
	)

	recipient = pm.getLastFollowerPubkey(singleTagEvent, nil)
	require.Equal(t, "follower_pubkey", recipient, "Recipient should be the only p-tag")

	oldEvent := helperCreateFollowListEvent(
		t,
		"old_id_"+uuid.NewString(),
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2", "pubkey3"},
	)

	newReducedEvent := helperCreateFollowListEvent(
		t,
		"new_id_"+uuid.NewString(),
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2"},
	)

	recipient = pm.getLastFollowerPubkey(newReducedEvent, oldEvent)
	require.Empty(t, recipient, "Recipient should be empty when follow list reduced")

	newExtendedEvent := helperCreateFollowListEvent(
		t,
		"new_id",
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2", "pubkey3", "new_pubkey"},
	)

	recipient = pm.getLastFollowerPubkey(newExtendedEvent, oldEvent)
	require.Equal(t, "new_pubkey", recipient, "Recipient should be the last p-tag")
}

func TestCreateNewFollowerNotification(t *testing.T) {
	t.Parallel()

	testSuffix := uuid.NewString()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "target_pubkey_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device1_"+testSuffix,
		nostr.Tags{
			{"t", "ios"},
			{"token", "test_token_" + testSuffix},
		},
		filters,
	)

	require.NoError(t, query.AcceptEvents(t.Context(), followListEvent))
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent))

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))
	require.Len(t, pm.userDevicesMap, 1, "User should be added to the device map")
	require.Contains(t, pm.userDevicesMap, targetPubKey, "User should be in the device map")
	require.Len(t, pm.userDevicesMap[targetPubKey], 1, "User should have one device")

	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title(), notification.Title)
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body(), notification.Body)
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].ImageURL(), notification.ImageURL)
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")

	iterator := query.GetStoredEvents(t.Context(), &model.Subscription{
		Filters: []model.Filter{
			{
				Authors: []string{followerPubKey},
				Kinds:   []int{nostr.KindFollowList},
			},
		},
	})
	foundEvents := []*model.Event{}
	for event, err := range iterator {
		require.NoError(t, err)
		foundEvents = append(foundEvents, event)
	}
	require.NotEmpty(t, foundEvents, "Follow list event should be stored in the database")

	require.Equal(t, followerPubKey, foundEvents[0].PubKey, "Event pubkey should match")
}

func TestCreateNewFollowerNotificationMultipleDevices(t *testing.T) {
	t.Parallel()

	testSuffix := uuid.NewString()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "multi_device_target_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	require.NoError(t, query.AcceptEvents(t.Context(), followListEvent))

	deviceID1 := "device1_" + testSuffix
	deviceID2 := "device2_" + testSuffix
	deviceID3 := "device3_" + testSuffix

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		deviceID1,
		nostr.Tags{
			{"t", "ios"},
			{"d", deviceID1},
			{"token", "token1_" + testSuffix},
		},
		filters,
	)
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))

	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		deviceID2,
		nostr.Tags{
			{"t", "android"},
			{"d", deviceID2},
			{"token", "token2_" + testSuffix},
		},
		filters,
	)
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent2))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))

	deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		deviceID3,
		nostr.Tags{
			{"t", "web"},
			{"d", deviceID3},
			{"token", "token3_" + testSuffix},
		},
		filters,
	)
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent3))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))

	require.Len(t, pm.userDevicesMap, 1, "Should have one user in the device map")
	require.Contains(t, pm.userDevicesMap, targetPubKey, "User should be in the device map")
	require.Len(t, pm.userDevicesMap[targetPubKey], 3, "User should have three devices")

	require.Contains(t, pm.userDevicesMap[targetPubKey], DeviceID(deviceID1), "Device 1 should be in the map")
	require.Contains(t, pm.userDevicesMap[targetPubKey], DeviceID(deviceID2), "Device 2 should be in the map")
	require.Contains(t, pm.userDevicesMap[targetPubKey], DeviceID(deviceID3), "Device 3 should be in the map")

	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 3, "Should create three notifications")

	deviceTypeMap := make(map[string]*pn.Notification[*DeviceRegistrationEvent])
	for _, notification := range notifications {
		deviceType := notification.Target.GetTag("t").Value()
		deviceTypeMap[deviceType] = notification
	}

	require.Len(t, deviceTypeMap, 3, "Should have notifications for all device types")
	require.Contains(t, deviceTypeMap, "ios", "Should have iOS notification")
	require.Contains(t, deviceTypeMap, "android", "Should have Android notification")
	require.Contains(t, deviceTypeMap, "web", "Should have web notification")

	for _, platform := range []string{"ios", "android", "web"} {
		notification := deviceTypeMap[platform]
		require.Contains(t, notification.Data, "event", "Data should contain event")
		require.Equal(t, followListEvent.String(), notification.Data["event"], "Event should match")

		if platform == validation.DeviceTokenOSIOS || platform == validation.DeviceTokenOSWeb {
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title(), notification.Title, "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body(), notification.Body, "Body should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].ImageURL(), notification.ImageURL, "Image URL should match")
		} else if platform == validation.DeviceTokenOSAndroid {
			require.Equal(t, "", notification.Title, "Title should match")
			require.Equal(t, "", notification.Body, "Body should match")
			require.Equal(t, "", notification.ImageURL, "Image URL should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title(), notification.Data["title"], "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body(), notification.Data["body"], "Body should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].ImageURL(), notification.Data["imageUrl"], "Image URL should match")
		}
	}

	iterator := query.GetStoredEvents(t.Context(), &model.Subscription{
		Filters: []model.Filter{
			{
				Authors: []string{targetPubKey},
				Kinds:   []int{model.CustomIONKindDeviceRegistration},
			},
		},
	})

	foundEvents := []*model.Event{}
	for event, err := range iterator {
		require.NoError(t, err)
		foundEvents = append(foundEvents, event)
	}

	require.NotEmpty(t, foundEvents, "Device registration events should be stored in the database")
	require.Len(t, foundEvents, 3, "All device registration events should be stored")
}

func TestCreateNewFollowerNotificationNoDevices(t *testing.T) {
	t.Parallel()

	testSuffix := uuid.NewString()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "no_device_target_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	require.NoError(t, query.AcceptEvents(t.Context(), followListEvent))

	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey)

	require.Nil(t, notifications, "Notifications should be nil when no devices")
}

func TestHandleNewFollowerEvent(t *testing.T) {
	t.Parallel()

	t.Run("new_follower_without_old_event", func(t *testing.T) {
		t.Parallel()

		testSuffix := uuid.NewString()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		}

		followerPubKey := "follower_pubkey_" + testSuffix
		targetPubKey := "target_pubkey_" + testSuffix

		followListEvent := helperCreateFollowListEvent(
			t,
			"test_id_"+testSuffix,
			followerPubKey,
			[]string{"pubkey1", "pubkey2", targetPubKey},
		)

		filters := nostr.Filters{
			{
				Kinds: []int{nostr.KindFollowList},
			},
		}

		deviceEvent := helperCreateTestDeviceRegistrationEvent(
			t,
			targetPubKey,
			"device1_"+testSuffix,
			nostr.Tags{
				{"t", "ios"},
				{"token", "test_token_" + testSuffix},
			},
			filters,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

		notifications, err := pm.handleNewFollowerEvent(t.Context(), followListEvent)

		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 1)
	})

	t.Run("new_follower_with_old_event", func(t *testing.T) {
		t.Parallel()

		testSuffix := uuid.NewString()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		}

		followerPubKey := "follower_pubkey_" + testSuffix
		targetPubKey := "target_pubkey_" + testSuffix

		oldFollowListEvent := helperCreateFollowListEvent(
			t,
			"old_id_"+testSuffix,
			followerPubKey,
			[]string{"pubkey1", "pubkey2"},
		)

		newFollowListEvent := helperCreateFollowListEvent(
			t,
			"new_id_"+testSuffix,
			followerPubKey,
			[]string{"pubkey1", "pubkey2", targetPubKey},
		)

		filters := nostr.Filters{
			{
				Kinds: []int{nostr.KindFollowList},
			},
		}

		deviceEvent := helperCreateTestDeviceRegistrationEvent(
			t,
			targetPubKey,
			"device1_"+testSuffix,
			nostr.Tags{
				{"t", "ios"},
				{"token", "test_token_" + testSuffix},
			},
			filters,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), oldFollowListEvent))
		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

		notifications, err := pm.handleNewFollowerEvent(t.Context(), newFollowListEvent)

		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 1)
	})
}

func TestCreateNewFollowerNotificationWithRelevantEvents(t *testing.T) {
	t.Parallel()

	testSuffix := uuid.NewString()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "target_pubkey_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	profileData := struct {
		Name        string `json:"name,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
	}{
		Name:        "FollowerUsername",
		DisplayName: "Follower Display Name",
	}

	profileJSON, err := json.Marshal(profileData)
	require.NoError(t, err)

	profileEvent := &model.Event{
		Event: nostr.Event{
			ID:      "profile_id_" + testSuffix,
			PubKey:  followerPubKey,
			Kind:    nostr.KindProfileMetadata,
			Content: string(profileJSON),
		},
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device1_"+testSuffix,
		nostr.Tags{
			{"t", "ios"},
			{"token", "test_token_" + testSuffix},
		},
		filters,
	)

	pm.deviceMutex.Lock()
	pm.userDevicesMap[targetPubKey] = map[DeviceID]DeviceInfo{
		DeviceID("device1_" + testSuffix): {
			DeviceID: DeviceID("device1_" + testSuffix),
			Filters:  filters,
			Event:    deviceEvent,
		},
	}
	pm.deviceMutex.Unlock()

	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey, profileEvent)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title(), notification.Title)
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body(profileEvent), notification.Body)
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].ImageURL(), notification.ImageURL)
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "relevant_events", "Data should contain relevant events")

	relevantEvents, ok := notification.Data["relevant_events"].(string)
	require.True(t, ok, "relevant_events should be a string")

	var eventsSlice []string
	require.NoError(t, json.Unmarshal([]byte(relevantEvents), &eventsSlice))
	require.Len(t, eventsSlice, 1, "Should contain one relevant event")
	require.Equal(t, eventsSlice[0], string(profileJSON), "Relevant event should contain profile event content")
}
