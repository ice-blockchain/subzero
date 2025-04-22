// SPDX-License-Identifier: ice License 1.0


package internal

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
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

func (m *mockClient) SendSingle(ctx context.Context, notification *Notification[*model.Event]) error {
	tokenTag := notification.Target.GetTag("token")
	if tokenTag == nil || tokenTag.Value() == "" || !isValidToken(tokenTag.Value()) {
		return ErrInvalidDeviceToken
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

	event1 := &model.Event{}
	event1.Tags = append(event1.Tags, model.Tag{"token", testToken})
	event1.Tags = append(event1.Tags, model.Tag{"deviceId", uuid.NewString()})

	n1 := &Notification[*model.Event]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   event1,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err := client.SendSingle(t.Context(), n1)
	require.ErrorIs(t, err, ErrInvalidDeviceToken)

	event2 := &model.Event{}
	event2.Tags = append(event2.Tags, model.Tag{"token", testToken + "_valid"})
	event2.Tags = append(event2.Tags, model.Tag{"deviceId", uuid.NewString()})

	n2 := &Notification[*model.Event]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   event2,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	err = client.SendSingle(t.Context(), n2)
	require.NoError(t, err)
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

	event := &model.Event{}
	event.Tags = append(event.Tags, model.Tag{"token", testToken + "_valid"})
	event.Tags = append(event.Tags, model.Tag{"deviceId", uuid.NewString()})

	n1 := &Notification[*model.Event]{
		Data:     map[string]interface{}{"deeplink": fmt.Sprintf("ion.app/something/%v", uuid.NewString())},
		Target:   event,
		Title:    testTitle,
		Body:     testBody + uuid.NewString(),
		ImageURL: "https://example.com/image.jpg",
	}

	wg := new(sync.WaitGroup)
	const concurrency = 1000
	wg.Add(concurrency)

	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(wg *sync.WaitGroup, n *Notification[*model.Event]) {
			defer wg.Done()

			err := client.SendSingle(context.Background(), n)
			if err != nil {
				results <- err
			}
		}(wg, n1)
	}

	wg.Wait()
	close(results)

	for err := range results {
		require.NoError(t, err)
	}
}
