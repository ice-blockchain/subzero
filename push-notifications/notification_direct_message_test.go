// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func helperCreateDirectMessageEvent(t *testing.T, id string, senderPubKey string, recipientPubKey string, content string) *model.Event {
	t.Helper()

	tags := nostr.Tags{}
	if recipientPubKey != "" {
		tags = append(tags, nostr.Tag{"p", recipientPubKey})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  senderPubKey,
			Kind:    nostr.KindGiftWrap,
			Content: content,
			Tags:    tags,
		},
	}
}

func TestHandleDirectMessageNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	senderPubKey := "sender_pubkey"
	recipientPubKey := "recipient_pubkey"
	messageContent := "Hello, this is a direct message!"

	event := helperCreateDirectMessageEvent(
		t,
		"test_id",
		senderPubKey,
		recipientPubKey,
		messageContent,
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))
	notifications := pm.handleDirectMessageNotification(event)
	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeDirectMessage].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeDirectMessage].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should match the device event")

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
	require.Equal(t, string(NotificationTypeDirectMessage), notification.Data["notificationType"], "NotificationType should match")
}

func TestHandleDirectMessageNotificationWithoutRecipient(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := helperCreateDirectMessageEvent(
		t,
		"test_id",
		"sender_pubkey",
		"",
		"Message without recipient",
	)

	notifications := pm.handleDirectMessageNotification(event)

	require.Nil(t, notifications, "Should not create notifications when there's no recipient")
}

func TestHandleDirectMessageNotificationSelfSent(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	senderPubKey := "sender_pubkey"

	event := helperCreateDirectMessageEvent(
		t,
		"test_id",
		senderPubKey,
		senderPubKey,
		"Message to self",
	)
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		senderPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))
	notifications := pm.handleDirectMessageNotification(event)
	require.Empty(t, notifications, "Should not create notifications for self-sent messages")
}

func TestHandleDirectMessageNotificationNoValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}
	event := helperCreateDirectMessageEvent(
		t,
		"test_id",
		"sender_pubkey",
		"recipient_without_devices",
		"Message for recipient without devices",
	)
	notifications := pm.handleDirectMessageNotification(event)
	require.Empty(t, notifications, "Should not create notifications when recipient has no valid devices")
}

func TestHandleDirectMessageNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	senderPubKey := "sender_pubkey"
	recipientPubKey := "multi_device_recipient"
	messageContent := "Message for multiple devices"

	event := helperCreateDirectMessageEvent(
		t,
		"test_id",
		senderPubKey,
		recipientPubKey,
		messageContent,
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
		},
	}

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey,
		"device1",
		[]string{"t", "ios", "token", "token1"},
		filters,
	)

	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey,
		"device2",
		[]string{"t", "android", "token", "token2"},
		filters,
	)

	deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey,
		"device3",
		[]string{"t", "web", "token", "token3"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))

	notifications := pm.handleDirectMessageNotification(event)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 3, "Should create three notifications (one for each device)")

	for _, notification := range notifications {
		require.Contains(t, notification.Data, "event", "Data should contain event")
		require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
		require.Equal(t, string(NotificationTypeDirectMessage), notification.Data["notificationType"], "NotificationType should match")

		platform := event.GetTag("t").Value()
		if platform == validation.DeviceTokenOSIOS || platform == validation.DeviceTokenOSWeb {
			require.Equal(t, DefaultTranslations[NotificationTypeDirectMessage].Title, notification.Title, "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeDirectMessage].Body, notification.Body, "Body should match")
		} else if platform == validation.DeviceTokenOSAndroid {
			require.Equal(t, DefaultTranslations[NotificationTypeDirectMessage].Title, notification.Data["title"], "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeDirectMessage].Body, notification.Data["body"], "Body should match")
			require.Equal(t, "", notification.Title, "Title should match")
			require.Equal(t, "", notification.Body, "Body should match")
		}

		deviceID := notification.Target.GetTag("d").Value()
		require.Contains(t, []string{"device1", "device2", "device3"}, deviceID, "Device ID should be valid")
	}
}
