// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
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

	pubKeyX25519, err := nip44.ConvertEd25519PublicKeyToX25519(publicKey)
	require.NoError(t, err)

	conversationKey, err := nip44.GenerateConversationKeyX25519(privKeyX25519, pubKeyX25519)
	require.NoError(t, err)

	encryptedToken, err := nip44.EncryptX25519(token, conversationKey, nil)
	require.NoError(t, err)

	return encryptedToken
}

func TestCreateSingleMessage(t *testing.T) {
	t.Parallel()

	privateKey, publicKey := model.GenerateKeyPair()
	privKeyX25519, err := nip44.ConvertEd25519PrivateKeyToX25519(privateKey)
	require.NoError(t, err)

	client := &notificationClient{
		privateKey: privKeyX25519,
	}

	validToken := "valid-test-token-" + uuid.NewString()
	encryptedValidToken := createEncryptedToken(t, validToken, privateKey, publicKey)

	event1 := &model.Event{}
	event1.Tags = append(event1.Tags, model.Tag{"token", encryptedValidToken})
	event1.Tags = append(event1.Tags, model.Tag{"deviceId", uuid.NewString()})
	require.NoError(t, event1.SignWithAlg(privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

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

	event5 := &model.Event{}
	event5.Tags = append(event5.Tags, model.Tag{"token", encryptedValidToken})
	event5.Tags = append(event5.Tags, model.Tag{"deviceId", uuid.NewString()})
	require.NoError(t, event5.SignWithAlg(privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	jsonData, err := json.Marshal([]string{
		`{"id":"event1","content":"Sample content 1"}`,
		`{"id":"event2","content":"Sample content 2"}`,
	})
	require.NoError(t, err)
	jsonStr := string(jsonData)

	notification5 := &Notification[*DeviceRegistrationEvent]{
		Data: map[string]interface{}{
			"relevant_events": jsonStr,
		},
		Target:   event5,
		Title:    testTitle,
		Body:     testBody,
		ImageURL: "https://example.com/image.jpg",
	}

	message5, err := client.createSingleMessage(notification5)
	require.NoError(t, err)
	require.NotNil(t, message5)
	require.Contains(t, message5.Data, "relevant_events")

	require.Equal(t, jsonStr, message5.Data["relevant_events"])

	var parsedEvents []string
	require.NoError(t, json.Unmarshal([]byte(message5.Data["relevant_events"]), &parsedEvents), "relevant_events should be a valid JSON array")
	require.Len(t, parsedEvents, 2)
	require.Equal(t, `{"id":"event1","content":"Sample content 1"}`, parsedEvents[0])
	require.Equal(t, `{"id":"event2","content":"Sample content 2"}`, parsedEvents[1])

	event2 := &model.Event{}
	event2.Tags = append(event2.Tags, model.Tag{"deviceId", uuid.NewString()})
	require.NoError(t, event2.SignWithAlg(privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
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
	event4.Tags = append(event4.Tags, model.Tag{"token", encryptedValidToken})
	event4.Tags = append(event4.Tags, model.Tag{"deviceId", uuid.NewString()})
	require.NoError(t, event4.SignWithAlg(privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

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

	jsonData, err := json.Marshal([]string{
		`{"id":"event1","content":"Sample content 1"}`,
		`{"id":"event2","content":"Sample content 2"}`,
	})
	require.NoError(t, err)
	jsonStr := string(jsonData)

	notification4 := &Notification[SubscriptionTopic]{
		Data: map[string]interface{}{
			"relevant_events": jsonStr,
		},
		Target:   SubscriptionTopic(testTopic),
		Title:    testTitle,
		Body:     testBody,
		ImageURL: "https://example.com/image.jpg",
	}

	message4 := client.createTopicMessage(notification4)
	require.NotNil(t, message4)
	require.Contains(t, message4.Data, "relevant_events")

	require.Equal(t, jsonStr, message4.Data["relevant_events"])

	var parsedEvents []string
	err = json.Unmarshal([]byte(message4.Data["relevant_events"]), &parsedEvents)
	require.NoError(t, err, "relevant_events should be a valid JSON array")
	require.Len(t, parsedEvents, 2)
	require.Equal(t, `{"id":"event1","content":"Sample content 1"}`, parsedEvents[0])
	require.Equal(t, `{"id":"event2","content":"Sample content 2"}`, parsedEvents[1])

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

func TestDecryptToken(t *testing.T) {
	t.Parallel()

	privKeyBE, pubKeyBE := model.GenerateKeyPair()
	privKeyFE, _ := model.GenerateKeyPair()

	privKeyX25519FE, err := nip44.ConvertEd25519PrivateKeyToX25519(privKeyFE)
	require.NoError(t, err)

	privKeyX25519BE, err := nip44.ConvertEd25519PrivateKeyToX25519(privKeyBE)
	require.NoError(t, err)

	pubKeyX25519BE, err := nip44.ConvertEd25519PublicKeyToX25519(pubKeyBE)
	require.NoError(t, err)

	t.Run("successful decryption of token", func(t *testing.T) {
		t.Parallel()
		ev := &model.Event{}
		ev.Kind = nostr.KindTextNote
		require.NoError(t, ev.SignWithAlg(privKeyFE, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		conversationKey, err := nip44.GenerateConversationKeyX25519(privKeyX25519FE, pubKeyX25519BE)
		require.NoError(t, err)

		originalToken := "test-device-token-123456"
		encryptedToken, err := nip44.EncryptX25519(originalToken, conversationKey, nil)
		require.NoError(t, err)

		ev.Tags = nostr.Tags{{"token", encryptedToken}}

		decryptedToken, err := DecryptToken(ev, privKeyX25519BE)
		require.NoError(t, err)
		require.Equal(t, originalToken, decryptedToken)
	})

	t.Run("token is missing", func(t *testing.T) {
		t.Parallel()
		ev := &model.Event{}
		ev.Kind = nostr.KindTextNote
		require.NoError(t, ev.SignWithAlg(privKeyFE, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		decryptedToken, err := DecryptToken(ev, privKeyX25519FE)
		require.NoError(t, err)
		require.Empty(t, decryptedToken)
	})

	t.Run("wrong private key", func(t *testing.T) {
		t.Parallel()
		ev := &model.Event{}
		ev.Kind = nostr.KindTextNote
		require.NoError(t, ev.SignWithAlg(privKeyFE, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		conversationKey, err := nip44.GenerateConversationKeyX25519(privKeyX25519FE, pubKeyX25519BE)
		require.NoError(t, err)

		originalToken := "test-device-token-123456"
		encryptedToken, err := nip44.EncryptX25519(originalToken, conversationKey, nil)
		require.NoError(t, err)

		ev.Tags = nostr.Tags{{"token", encryptedToken}}
		wrongPrivKey, _ := model.GenerateKeyPair()

		_, err = DecryptToken(ev, wrongPrivKey)
		require.Error(t, err)
	})

	t.Run("wrong token format", func(t *testing.T) {
		t.Parallel()
		ev := &model.Event{}
		ev.Kind = nostr.KindTextNote
		require.NoError(t, ev.SignWithAlg(privKeyFE, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		ev.Tags = nostr.Tags{{"token", "not-a-valid-encrypted-token"}}

		_, err := DecryptToken(ev, privKeyX25519FE)
		require.Error(t, err)
	})
}
