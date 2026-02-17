// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func helperCreateFollowListEvent(t *testing.T, id string, pubKey string, followedPubKeys []string) *model.Event {
	t.Helper()

	tags := model.Tags{}
	for _, followedPubKey := range followedPubKeys {
		tags = append(tags, model.Tag{"p", followedPubKey})
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

func TestGetNewlyFollowedPubkeys(t *testing.T) {
	t.Parallel()

	emptyTagsEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+rand.Text(),
		"follower_pubkey",
		[]string{},
	)

	recipients := model.GetNewlyFollowedPubkeys(emptyTagsEvent, nil)
	require.Empty(t, recipients, "Recipients should be empty when no p-tags")

	singleTagEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+rand.Text(),
		"follower_pubkey",
		[]string{"follower_pubkey"},
	)

	recipients = model.GetNewlyFollowedPubkeys(singleTagEvent, nil)
	require.Len(t, recipients, 1, "Should return one recipient when there's only one p-tag and no old event")
	require.Equal(t, "follower_pubkey", recipients[0], "Recipient should be the only p-tag")

	oldEvent := helperCreateFollowListEvent(
		t,
		"old_id_"+rand.Text(),
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2", "pubkey3"},
	)

	newReducedEvent := helperCreateFollowListEvent(
		t,
		"new_id_"+rand.Text(),
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2"},
	)

	recipients = model.GetNewlyFollowedPubkeys(newReducedEvent, oldEvent)
	require.Empty(t, recipients, "Recipients should be empty when follow list reduced")

	newExtendedEvent := helperCreateFollowListEvent(
		t,
		"new_id",
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2", "pubkey3", "new_pubkey"},
	)

	recipients = model.GetNewlyFollowedPubkeys(newExtendedEvent, oldEvent)
	require.Len(t, recipients, 1, "Should return one new recipient")
	require.Equal(t, "new_pubkey", recipients[0], "Recipient should be the new pubkey")

	multipleNewEvent := helperCreateFollowListEvent(
		t,
		"multi_new_id",
		"follower_pubkey",
		[]string{"pubkey1", "new_pubkey1", "pubkey2", "new_pubkey2", "pubkey3"},
	)

	recipients = model.GetNewlyFollowedPubkeys(multipleNewEvent, oldEvent)
	require.Len(t, recipients, 2, "Should return all new recipients")
	require.Contains(t, recipients, "new_pubkey1", "Should contain first new pubkey")
	require.Contains(t, recipients, "new_pubkey2", "Should contain second new pubkey")
}

func TestCreateNewFollowerNotification(t *testing.T) {
	t.Parallel()

	testSuffix := rand.Text()

	pm := helperNewManager(t)

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "target_pubkey_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	filters := model.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device1-"+testSuffix,
		model.Tags{
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

	notifications, err := pm.createNewFollowerNotification(followListEvent, targetPubKey)
	require.NoError(t, err)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications.Local, 1, "Should create one notification")

	notification := notifications.Local[0]
	require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Title, notification.Title)
	require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Body, notification.Body)
	require.Equal(t, defaultTranslations[NotificationTypeNewFollower].ImageURL, notification.ImageURL)
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")

	iterator := query.GetStoredEvents(t.Context(), model.Filter{
		Authors: []string{followerPubKey},
		Kinds:   []int{nostr.KindFollowList},
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

	testSuffix := rand.Text()

	pm := helperNewManager(t)

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "multi_device_target_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	filters := model.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	require.NoError(t, query.AcceptEvents(t.Context(), followListEvent))

	deviceID1 := "device1-" + testSuffix
	deviceID2 := "device2-" + testSuffix
	deviceID3 := "device3-" + testSuffix

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		deviceID1,
		model.Tags{
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
		model.Tags{
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
		model.Tags{
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

	require.Contains(t, pm.userDevicesMap[targetPubKey], deviceID1, "Device 1 should be in the map")
	require.Contains(t, pm.userDevicesMap[targetPubKey], deviceID2, "Device 2 should be in the map")
	require.Contains(t, pm.userDevicesMap[targetPubKey], deviceID3, "Device 3 should be in the map")

	notifications, err := pm.createNewFollowerNotification(followListEvent, targetPubKey)
	require.NoError(t, err)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications.Local, 3, "Should create three notifications")

	deviceTypeMap := make(map[string]*pn.Notification[*model.Event])
	for _, notification := range notifications.Local {
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

		compressedEvent, ok := notification.Data["event"].(string)
		require.True(t, ok, "event should be a string")

		decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
		require.Equal(t, followListEvent.String(), decompressedEvent, "Decompressed event should match original")
		require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")

		switch platform {
		case model.DeviceTokenOSIOS, model.DeviceTokenOSWeb:
			require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Title, notification.Title, "Title should match")
			require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Body, notification.Body, "Body should match")
			require.Equal(t, defaultTranslations[NotificationTypeNewFollower].ImageURL, notification.ImageURL, "Image URL should match")
		case model.DeviceTokenOSAndroid:
			require.Equal(t, "", notification.Title, "Title should match")
			require.Equal(t, "", notification.Body, "Body should match")
			require.Equal(t, "", notification.ImageURL, "Image URL should match")
		default:
			t.Fatalf("unknown platform: %s", platform)
		}
	}

	iterator := query.GetStoredEvents(t.Context(), model.Filter{
		Authors: []string{targetPubKey},
		Kinds:   []int{model.CustomIONKindDeviceRegistration},
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

	testSuffix := rand.Text()

	pm := helperNewManager(t)

	followerPubKey := "follower_pubkey_" + testSuffix
	targetPubKey := "no_device_target_" + testSuffix

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id_"+testSuffix,
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	require.NoError(t, query.AcceptEvents(t.Context(), followListEvent))

	notifications, err := pm.createNewFollowerNotification(followListEvent, targetPubKey)
	require.NoError(t, err)

	require.Empty(t, notifications, "Notifications should be nil when no devices")
}

func TestHandleNewFollowerEvent(t *testing.T) {
	t.Parallel()

	t.Run("new_follower_without_old_event", func(t *testing.T) {
		testSuffix := rand.Text()

		pm := helperNewManager(t)

		followerPubKey := "follower_pubkey_" + testSuffix
		targetPubKey1 := "target_pubkey1_" + testSuffix
		targetPubKey2 := "target_pubkey2_" + testSuffix

		followListEvent := helperCreateFollowListEvent(
			t,
			"test_id_"+testSuffix,
			followerPubKey,
			[]string{"pubkey1", targetPubKey1, "pubkey2", targetPubKey2},
		)

		filters := model.Filters{
			{
				Kinds: []int{nostr.KindFollowList},
			},
		}

		deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
			t,
			targetPubKey1,
			"device1-"+testSuffix,
			model.Tags{
				{"t", "ios"},
				{"token", "test_token1_" + testSuffix},
			},
			filters,
		)

		deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
			t,
			targetPubKey2,
			"device2-"+testSuffix,
			model.Tags{
				{"t", "android"},
				{"token", "test_token2_" + testSuffix},
			},
			filters,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent1))
		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent2))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))

		notifications, err := pm.handleNewFollowerEvent(followListEvent)

		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications.Local, 2, "Should have notifications for both targets")
	})

	t.Run("new_follower_with_old_event", func(t *testing.T) {
		testSuffix := rand.Text()

		pm := helperNewManager(t)

		followerPubKey := "follower_pubkey_" + testSuffix
		targetPubKey1 := "target_pubkey1_" + testSuffix
		targetPubKey2 := "target_pubkey2_" + testSuffix

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
			[]string{"pubkey1", "pubkey2", targetPubKey1, targetPubKey2},
		)

		filters := model.Filters{
			{
				Kinds: []int{nostr.KindFollowList},
			},
		}

		deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
			t,
			targetPubKey1,
			"device1-"+testSuffix,
			model.Tags{
				{"t", "ios"},
				{"token", "test_token1_" + testSuffix},
			},
			filters,
		)

		deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
			t,
			targetPubKey2,
			"device2-"+testSuffix,
			model.Tags{
				{"t", "android"},
				{"token", "test_token2_" + testSuffix},
			},
			filters,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), oldFollowListEvent))
		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent1))
		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent2))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))

		notifications, err := pm.handleNewFollowerEvent(newFollowListEvent)

		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications.Local, 2, "Should have notifications for both new targets")
	})
}

func TestCreateNewFollowerNotificationWithRelevantEvents(t *testing.T) {
	t.Parallel()

	testSuffix := rand.Text()

	pm := helperNewManager(t)

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

	filters := model.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device1_"+testSuffix,
		model.Tags{
			{"t", "ios"},
			{"token", "test_token_" + testSuffix},
		},
		filters,
	)

	pm.deviceMutex.Lock()
	pm.userDevicesMap[targetPubKey] = map[string]DeviceInfo{
		"device1_" + testSuffix: {
			Filters: filters,
			Event:   deviceEvent,
		},
	}
	pm.deviceMutex.Unlock()

	notifications, err := pm.createNewFollowerNotification(followListEvent, targetPubKey, profileEvent)
	require.NoError(t, err)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications.Local, 1, "Should create one notification")

	notification := notifications.Local[0]
	require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Title, notification.Title)
	require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Body, notification.Body)
	require.Equal(t, defaultTranslations[NotificationTypeNewFollower].ImageURL, notification.ImageURL)
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "relevant_events", "Data should contain relevant events")

	relevantEventsCompressed, ok := notification.Data["relevant_events"].(string)
	require.True(t, ok, "relevant_events should be a string")

	decompressed := helperDecompressZlibAndDecodeBase64(t, relevantEventsCompressed)
	require.Equal(t, `[`+string(profileJSON)+`]`, decompressed, "Decompressed relevant event should match profile event content")
	require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
}

func TestHandleNewFollowerEventWithOldEvents(t *testing.T) {
	t.Parallel()

	testSuffix := rand.Text()

	pm := helperNewManager(t)

	privKey, followListAuthorPubKey := model.GenerateKeyPair()
	recipientPubKey1 := "recipient1_" + testSuffix
	recipientPubKey2 := "recipient2_" + testSuffix
	recipientPubKey3 := "recipient3_" + testSuffix

	profileData := model.ProfileMetadataContent{
		Name:        "TestFollower",
		DisplayName: "Test Follower Display Name",
	}
	profileJSON, err := json.Marshal(profileData)
	require.NoError(t, err)

	followListAuthorProfileEvent := &model.Event{
		Event: nostr.Event{
			Kind:      nostr.KindProfileMetadata,
			CreatedAt: nostr.Now(),
			Content:   string(profileJSON),
		},
	}
	require.NoError(t, followListAuthorProfileEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), followListAuthorProfileEvent))
	followListAuthorAttestationEvent := &model.Event{
		Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{"p", followListAuthorPubKey, "", "active:" + nostr.Now().String() + ":1,7"},
			},
			Content: "",
		},
	}
	require.NoError(t, followListAuthorAttestationEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), followListAuthorAttestationEvent))

	pm.relayURL = "wss://test-follower-relay.example.com"
	followListAuthorRelayListEvent := &model.Event{
		Event: nostr.Event{
			Kind:      nostr.KindRelayListMetadata,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{"r", pm.relayURL, "read"},
				{"r", pm.relayURL, "write"},
			},
			Content: "",
		},
	}
	require.NoError(t, followListAuthorRelayListEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), followListAuthorRelayListEvent))

	filters := model.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey1,
		"device1_"+testSuffix,
		model.Tags{
			{"t", "ios"},
			{"d", "device1-" + testSuffix},
			{"relay", pm.relayURL},
			{"token", "token1_" + testSuffix},
		},
		filters,
	)
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))

	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey2,
		"device2_"+testSuffix,
		model.Tags{
			{"t", "android"},
			{"d", "device2-" + testSuffix},
			{"relay", pm.relayURL},
			{"token", "token2_" + testSuffix},
		},
		filters,
	)
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent2))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))

	deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey3,
		"device3_"+testSuffix,
		model.Tags{
			{"t", "web"},
			{"d", "device3-" + testSuffix},
			{"relay", pm.relayURL},
			{"token", "token3_" + testSuffix},
		},
		filters,
	)
	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent3))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))

	require.Len(t, pm.userDevicesMap, 3, "Should have three users in device map")
	require.Contains(t, pm.userDevicesMap, recipientPubKey1, "Should have recipient1 in device map")
	require.Contains(t, pm.userDevicesMap, recipientPubKey2, "Should have recipient2 in device map")
	require.Contains(t, pm.userDevicesMap, recipientPubKey3, "Should have recipient3 in device map")

	initialEvent := &model.Event{
		Event: nostr.Event{
			PubKey:    followListAuthorPubKey,
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: model.Tags{
				{"p", "existing_follower1"},
				{"p", recipientPubKey1},
			},
			Content: "initial follow list",
		},
	}
	require.NoError(t, initialEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), initialEvent))

	updatedEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: model.Tags{
				{"p", "existing_follower1"},
				{"p", recipientPubKey1},
				{"p", recipientPubKey2},
				{"p", recipientPubKey3},
				{"p", "existing_follower2"},
			},
			Content: "updated follow list with new followers",
		},
	}
	require.NoError(t, updatedEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), updatedEvent))
	require.NotNil(t, updatedEvent.Previous, "Updated event should have previous version")
	require.Equal(t, initialEvent.Event, updatedEvent.Previous.Event, "Previous event should match initial event")

	notifications, err := pm.processEvent(t.Context(), updatedEvent)
	require.NoError(t, err, "processEvent should not return error")

	require.Len(t, notifications.Local, 2, "Should have two notifications for the two new followers")

	deviceTargets := make(map[string]*model.Event)
	platformCounts := make(map[string]int)

	for _, notification := range notifications.Local {
		targetPubKey := notification.Target.GetMasterPublicKey()
		deviceTargets[targetPubKey] = notification.Target
		deviceType := notification.Target.GetTag("t").Value()
		platformCounts[deviceType]++

		if deviceType == model.DeviceTokenOSAndroid {
			require.Equal(t, "", notification.Title, "Android notifications should have empty title")
			require.Equal(t, "", notification.Body, "Android notifications should have empty body")
			require.Equal(t, "", notification.ImageURL, "Android notifications should have empty image URL")
		} else {
			require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Title, notification.Title)
			require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Body, notification.Body,
				"Should use default translation")
			require.Equal(t, defaultTranslations[NotificationTypeNewFollower].ImageURL, notification.ImageURL)
		}
		require.Contains(t, notification.Data, "event", "Data should contain event")
		require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Should use zlib compression")

		compressedEvent, ok := notification.Data["event"].(string)
		require.True(t, ok, "Event should be a compressed string")

		decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
		require.Equal(t, updatedEvent.String(), decompressedEvent, "Decompressed event should match original")
	}
	require.Contains(t, deviceTargets, recipientPubKey2, "Should have notification for recipient2 (new follower)")
	require.Contains(t, deviceTargets, recipientPubKey3, "Should have notification for recipient3 (new follower)")
	require.NotContains(t, deviceTargets, recipientPubKey1, "Should NOT have notification for recipient1 (already existed)")

	require.Equal(t, deviceEvent2, deviceTargets[recipientPubKey2], "Notification for recipient2 should target correct device")
	require.Equal(t, deviceEvent3, deviceTargets[recipientPubKey3], "Notification for recipient3 should target correct device")

	require.Equal(t, 1, platformCounts["android"], "Should have one Android notification")
	require.Equal(t, 1, platformCounts["web"], "Should have one Web notification")
	require.Equal(t, 0, platformCounts["ios"], "Should have no iOS notifications (recipient1 with iOS was already following)")
}
