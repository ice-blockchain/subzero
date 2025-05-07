// SPDX-License-Identifier: ice License 1.0

package hashtagssender

import (
	"fmt"
	"testing"
	"time"

	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestRealEndpointIntegration(t *testing.T) {
	t.Skip("skipping as it requires heimdall real endpoint")
	processor := &hashtagsSender{
		eventsQueue:  make([]*model.Event, 0, 1000),
		eventsToSend: make(chan []*model.Event, 10),
		config: &Config{
			BaseURL:            "https://localhost:8001",
			RequestTimeout:     30 * time.Second,
			MaxEventsQueueSize: 1000,
			SendInterval:       1 * time.Hour,
		},
		lastSent: time.Now(),
		req:      req.C().SetBaseURL("https://localhost:8001").EnableInsecureSkipVerify(),
	}

	go processor.startSender(t.Context())
	defer close(processor.eventsToSend)

	hashtags := []string{"#test", "#integration", "#subzero", "#blockchain", "#ice", "#crypto", "#decentralized", "#web3"}

	const eventsCount = 2100
	testEvents := make([]*model.Event, eventsCount)

	t.Logf("Generating %d test events (should be sent in at least 2 batches of 1000)...", eventsCount)
	for i := 0; i < eventsCount; i++ {
		testEvents[i] = helperGenerateEvent(t, i, hashtags)
	}
	t.Log("Events generation complete")

	t.Logf("Sending %d events to real endpoint: /v1/statistics/hashtags", eventsCount)
	startTime := time.Now()
	err := processor.processEvents(testEvents...)
	require.NoError(t, err)

	t.Log("Waiting for processing to complete (multiple batches)...")
	time.Sleep(30 * time.Second)

	elapsedTime := time.Since(startTime)
	t.Logf("Integration test completed in %v. Processed %d events in multiple batches.", elapsedTime, eventsCount)
}

func TestTimeoutTriggeredIntegration(t *testing.T) {
	t.Skip("skipping as it requires heimdall real endpoint")
	sentEventsCount := 0
	sentEventsCh := make(chan int, 10)

	processor := &hashtagsSender{
		eventsQueue:  make([]*model.Event, 0, 100),
		eventsToSend: make(chan []*model.Event, 10),
		config: &Config{
			BaseURL:            "https://localhost:8001",
			RequestTimeout:     30 * time.Second,
			MaxEventsQueueSize: 100,
			SendInterval:       1 * time.Minute,
		},
		lastSent: time.Now(),
		req:      req.C().SetBaseURL("https://localhost:8001").EnableInsecureSkipVerify(),
	}

	go func() {
		for {
			select {
			case count, ok := <-sentEventsCh:
				if !ok {
					return
				}
				sentEventsCount += count
				t.Logf("SENDING EVENTS TO ENDPOINT: %d events (total: %d)", count, sentEventsCount)
			case <-t.Context().Done():
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case events, ok := <-processor.eventsToSend:
				if !ok {
					return
				}
				t.Logf("Processing events in startSender: %d", len(events))
				err := processor.sendEvents(t.Context(), events)
				if err != nil {
					t.Logf("Error sending events: %v", err)
				} else {
					sentEventsCh <- len(events)
				}
			case <-t.Context().Done():
				return
			}
		}
	}()

	defer close(sentEventsCh)
	defer close(processor.eventsToSend)

	const eventsCount = 100
	testEvents := make([]*model.Event, eventsCount)

	t.Logf("Generating %d test events for processEvents test...", eventsCount)
	hashtags := []string{"#test", "#integration", "#subzero", "#blockchain", "#ice", "#crypto", "#decentralized", "#web3"}
	for i := 0; i < eventsCount; i++ {
		testEvents[i] = helperGenerateEvent(t, i, hashtags)
	}
	t.Log("Events generation complete")

	t.Logf("Calling processEvents with %d events (should be sent by max events queue size)", eventsCount)
	startTime := time.Now()

	err := processor.processEvents(testEvents...)
	require.NoError(t, err)

	t.Log("Waiting for events to be processed...")
	time.Sleep(10 * time.Second)

	elapsedTime := time.Since(startTime)
	t.Logf("ProcessEvents test completed in %v. Processed %d events, sent to endpoint: %d",
		elapsedTime, eventsCount, sentEventsCount)

	require.Equal(t, eventsCount, sentEventsCount, "Not all events were sent to the endpoint")
}

func helperGenerateEvent(t *testing.T, index int, hashtags []string) *model.Event {
	t.Helper()

	var kind int
	switch index % 3 {
	case 0:
		kind = nostr.KindTextNote
	case 1:
		kind = model.CustomIONKindEditableTextNote
	case 2:
		kind = nostr.KindArticle
	}

	contentHashtags := []string{}

	for i := 0; i < 3 && i < len(hashtags); i++ {
		tagIndex := (index + i) % len(hashtags)
		contentHashtags = append(contentHashtags, hashtags[tagIndex])
	}

	content := fmt.Sprintf("Test message %d with %s, %s and %s for hashtags processor integration testing.",
		index, contentHashtags[0], contentHashtags[1], contentHashtags[2])

	tags := []nostr.Tag{
		{"b", fmt.Sprintf("key%d", index)},
	}
	if kind == nostr.KindArticle || kind == model.CustomIONKindEditableTextNote {
		tags = append(tags, nostr.Tag{"published_at", fmt.Sprintf("1296962229")})
		tags = append(tags, nostr.Tag{"d", fmt.Sprintf("dtag-%d", index)})
	}

	for _, tag := range contentHashtags {
		tags = append(tags, nostr.Tag{"t", tag[1:]})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:        fmt.Sprintf("integration-test-%d", index),
			Kind:      kind,
			Content:   content,
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(index%100)),
			PubKey:    fmt.Sprintf("key%d", index),
			Tags:      tags,
		},
	}
}
