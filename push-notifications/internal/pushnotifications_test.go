// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

const (
	testToken = "bogusToken"
	testTitle = "Push test title"
	testBody  = "Push test body"
	testTopic = "testing"
)

func createEncryptedToken(t *testing.T, token, privateKey, publicKey string) string {
	t.Helper()

	privKeyX25519, err := nip44.ConvertEd25519PrivateKeyToX25519(privateKey)
	require.NoError(t, err)

	conversationKey, err := nip44.GenerateConversationKeyX25519(privKeyX25519, publicKey)
	require.NoError(t, err)

	encryptedToken, err := nip44.EncryptX25519(token, conversationKey, nil)
	require.NoError(t, err)

	return encryptedToken
}

func TestCreateSingleMessage(t *testing.T) {
	t.Parallel()

	privateKey, publicKey := model.GenerateKeyPair()
	client := &notificationClient{
		privateKey: privateKey,
	}

	validToken := "valid-test-token-" + uuid.NewString()
	encryptedValidToken := createEncryptedToken(t, validToken, privateKey, publicKey)

	event1 := &model.Event{}
	event1.PubKey = publicKey
	event1.Tags = append(event1.Tags, model.Tag{"token", encryptedValidToken})
	event1.Tags = append(event1.Tags, model.Tag{"deviceId", uuid.NewString()})

	notification1 := &Notification[*DeviceRegistrationEvent]{
		Data: map[string]interface{}{
			"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString()),
			"number":   42,
		},
		Target:   event1,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	message1, err := client.createSingleMessage(notification1)
	require.NoError(t, err)
	require.NotNil(t, message1)
	require.Equal(t, validToken, message1.Token)
	require.Equal(t, testTitle, message1.Notification.Title)
	require.Contains(t, message1.Notification.Body, testBody)
	require.Equal(t, "https://example.com/image.jpg", message1.Notification.ImageURL)
	require.Contains(t, message1.Data, "deeplink")
	require.Contains(t, message1.Data, "number")
	require.Contains(t, message1.Data["number"], "42")

	event2 := &model.Event{}
	event2.PubKey = publicKey
	event2.Tags = append(event2.Tags, model.Tag{"deviceId", uuid.NewString()})

	notification2 := &Notification[*DeviceRegistrationEvent]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   event2,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	message2, err := client.createSingleMessage(notification2)
	require.NoError(t, err)
	require.Nil(t, message2)

	_, wrongPublicKey := model.GenerateKeyPair()

	event3 := &model.Event{}
	event3.PubKey = wrongPublicKey
	event3.Tags = append(event3.Tags, model.Tag{"token", encryptedValidToken})
	event3.Tags = append(event3.Tags, model.Tag{"deviceId", uuid.NewString()})

	notification3 := &Notification[*DeviceRegistrationEvent]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   event3,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	message3, err := client.createSingleMessage(notification3)
	require.Error(t, err)
	require.Nil(t, message3)
	require.Contains(t, err.Error(), "failed to decrypt token")

	event4 := &model.Event{}
	event4.PubKey = publicKey
	event4.Tags = append(event4.Tags, model.Tag{"token", encryptedValidToken})
	event4.Tags = append(event4.Tags, model.Tag{"deviceId", uuid.NewString()})

	notification4 := &Notification[*DeviceRegistrationEvent]{
		Data:   map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target: event4,
	}

	message4, err := client.createSingleMessage(notification4)
	require.NoError(t, err)
	require.NotNil(t, message4)
	require.Equal(t, validToken, message4.Token)
	require.Nil(t, message4.Notification)
	require.Contains(t, message4.Data, "deeplink")
}

func TestCreateTopicMessage(t *testing.T) {
	t.Parallel()

	client := &notificationClient{}

	notification1 := &Notification[SubscriptionTopic]{
		Data: map[string]interface{}{
			"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString()),
			"number":   42,
		},
		Target:   SubscriptionTopic(testTopic),
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	message1 := client.createTopicMessage(notification1)
	require.NotNil(t, message1)
	require.Equal(t, testTopic, message1.Topic)
	require.Equal(t, testTitle, message1.Notification.Title)
	require.Contains(t, message1.Notification.Body, testBody)
	require.Equal(t, "https://example.com/image.jpg", message1.Notification.ImageURL)
	require.Contains(t, message1.Data, "deeplink")
	require.Contains(t, message1.Data, "number")
	require.Contains(t, message1.Data["number"], "42")

	notification2 := &Notification[SubscriptionTopic]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   SubscriptionTopic(""),
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	message2 := client.createTopicMessage(notification2)
	require.NotNil(t, message2)
	require.Equal(t, "", message2.Topic)
	require.Equal(t, testTitle, message2.Notification.Title)

	notification3 := &Notification[SubscriptionTopic]{
		Data:   map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target: SubscriptionTopic(testTopic),
	}

	message3 := client.createTopicMessage(notification3)
	require.NotNil(t, message3)
	require.Equal(t, testTopic, message3.Topic)
	require.Nil(t, message3.Notification)
	require.Contains(t, message3.Data, "deeplink")
}
