// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	stdlibtime "time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	testToken = "bogusToken"
	testTitle = "Push test title"
	testBody  = "Push test body"
	testTopic = "testing"
)

type mockClient struct {
	dryRun bool
}

func (m *mockClient) SendSingle(ctx context.Context, notification *Notification[DeviceToken]) error {
	if notification.Target.Token == "" || !isValidToken(notification.Target.Token) {
		return ErrInvalidDeviceToken
	}
	return nil
}

func (m *mockClient) SendBatch(ctx context.Context, notifications []*Notification[DeviceToken]) error {
	for _, notification := range notifications {
		if notification.Target.Token == "" || !isValidToken(notification.Target.Token) {
			return fmt.Errorf("can't send 1 from %d notifications: [%w]", len(notifications), ErrInvalidDeviceToken)
		}
	}
	return nil
}

func (m *mockClient) SendMulticast(ctx context.Context, notification *Notification[DeviceTokens]) error {
	invalidTokens := make(map[DeviceToken]error)
	for _, deviceToken := range notification.Target {
		if deviceToken.Token == "" || !isValidToken(deviceToken.Token) {
			invalidTokens[deviceToken] = ErrInvalidDeviceToken
		}
	}

	if len(invalidTokens) > 0 {
		return &MulticastError{
			Err:           fmt.Errorf("can't send %d from %d notifications", len(invalidTokens), len(notification.Target)),
			InvalidTokens: invalidTokens,
		}
	}
	return nil
}

func (m *mockClient) SendTopic(ctx context.Context, notification *Notification[SubscriptionTopic]) error {
	if string(notification.Target) == "" {
		return fmt.Errorf("topic cannot be empty")
	}
	return nil
}

func isValidToken(token string) bool {
	return token != testToken
}

func newTestClient() Client {
	return &mockClient{dryRun: true}
}

func TestSendSingle(t *testing.T) {
	t.Parallel()

	client := newTestClient()

	n1 := &Notification[DeviceToken]{
		Data: map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target: DeviceToken{
			Token:    testToken,
			DeviceID: DeviceID(uuid.NewString()),
		},
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err := client.SendSingle(t.Context(), n1)
	require.ErrorIs(t, err, ErrInvalidDeviceToken)

	n2 := &Notification[DeviceToken]{
		Data: map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target: DeviceToken{
			Token:    testToken + "_valid",
			DeviceID: DeviceID(uuid.NewString()),
		},
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err = client.SendSingle(t.Context(), n2)
	require.NoError(t, err)
}

func TestSendBatch(t *testing.T) {
	t.Parallel()

	client := newTestClient()

	notifications := make([]*Notification[DeviceToken], 0, 5)

	for i := 0; i < 4; i++ {
		n := &Notification[DeviceToken]{
			Data: map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
			Target: DeviceToken{
				Token:    testToken + "_valid" + fmt.Sprintf("_%d", i),
				DeviceID: DeviceID(uuid.NewString()),
			},
			Title:    testTitle,
			Body:     testBody + uuid.NewString(),
			ImageURL: "https://example.com/image.jpg",
		}
		notifications = append(notifications, n)
	}

	err := client.SendBatch(t.Context(), notifications)
	require.NoError(t, err)

	invalidNotification := &Notification[DeviceToken]{
		Data: map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target: DeviceToken{
			Token:    testToken,
			DeviceID: DeviceID(uuid.NewString()),
		},
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}
	notifications = append(notifications, invalidNotification)

	err = client.SendBatch(t.Context(), notifications)
	require.Error(t, err)
	require.Contains(t, err.Error(), "can't send")
}

func TestSendMulticast(t *testing.T) {
	t.Parallel()

	client := newTestClient()
	tokens := make([]DeviceToken, 0, 5)

	for i := 0; i < 4; i++ {
		token := DeviceToken{
			Token:    testToken + "_valid" + fmt.Sprintf("_%d", i),
			DeviceID: DeviceID(uuid.NewString()),
		}
		tokens = append(tokens, token)
	}

	n1 := &Notification[DeviceTokens]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   tokens,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err := client.SendMulticast(t.Context(), n1)
	require.NoError(t, err)

	invalidToken := DeviceToken{
		Token:    testToken,
		DeviceID: DeviceID(uuid.NewString()),
	}
	tokens = append(tokens, invalidToken)

	n2 := &Notification[DeviceTokens]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   tokens,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err = client.SendMulticast(t.Context(), n2)
	require.Error(t, err)

	multicastErr, ok := err.(*MulticastError)
	require.True(t, ok)

	require.Len(t, multicastErr.InvalidTokens, 1)
	_, contains := multicastErr.InvalidTokens[invalidToken]
	require.True(t, contains)
}

func TestSendTopic(t *testing.T) {
	t.Parallel()

	client := newTestClient()

	n1 := &Notification[SubscriptionTopic]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   SubscriptionTopic(testTopic),
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err := client.SendTopic(t.Context(), n1)
	require.NoError(t, err)

	n2 := &Notification[SubscriptionTopic]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   SubscriptionTopic(""),
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err = client.SendTopic(t.Context(), n2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "topic cannot be empty")
}

func TestSendSingle_Stability(t *testing.T) {
	t.Parallel()

	client := newTestClient()

	n1 := &Notification[DeviceToken]{
		Data: map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target: DeviceToken{
			Token:    testToken + "_valid",
			DeviceID: DeviceID(uuid.NewString()),
		},
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	wg := new(sync.WaitGroup)
	const concurrency = 1000
	wg.Add(concurrency)

	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			stdlibtime.Sleep(stdlibtime.Duration(rand.Intn(4)) * stdlibtime.Millisecond)

			err := client.SendSingle(t.Context(), n1)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		require.NoError(t, err)
		successCount++
	}

	require.Equal(t, concurrency, successCount)
}
