// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func BenchmarkCanForwardEvent(b *testing.B) {
	b.Run("device key", func(b *testing.B) {
		var ev model.Event

		ev.Kind = nostr.KindGiftWrap
		ev.Tags = model.Tags{
			{"k", "1"},
			{"expiration", "1234567890"},
			{"p", "deviceKey"},
		}
		b.ReportAllocs()
		for b.Loop() {
			canForwardEvent(&ev, nil, "master", "deviceKey")
		}
	})
	b.Run("device key with master", func(b *testing.B) {
		var ev model.Event

		ev.Kind = nostr.KindGiftWrap
		ev.Tags = model.Tags{
			{"k", "1"},
			{"expiration", "1234567890"},
			{"p", "deviceKey", "", "master"},
		}
		b.ReportAllocs()
		for b.Loop() {
			canForwardEvent(&ev, nil, "master", "deviceKey")
		}
	})
}

type dummyWriter struct{}

func (*dummyWriter) WriteMessage(context.Context, int, []byte) error { return nil }
func (*dummyWriter) Close() error                                    { return nil }

func BenchmarkAuthLoadAndBroadcast(b *testing.B) {
	const subscriptionCount = 500_000

	h := newHandler("", "")
	require.NotNil(b, h)

	writers := make([]Writer, subscriptionCount)
	spareWriters := make([]Writer, subscriptionCount)
	for idx := range subscriptionCount {
		w := new(dummyWriter)
		h.linkSubscription(w, &model.Subscription{
			ID: strconv.Itoa(idx),
			Filters: model.Filters{
				{
					Kinds: []int{nostr.KindTextNote},
					Since: model.PointerOf(nostr.Now().Add(time.Hour)),
				},
			},
		})
		h.ConnAuth.Store(w, connAuthData{Challenge: "challenge-" + strconv.Itoa(idx)})
		writers[idx] = w
		spareWriters[idx] = new(dummyWriter)
	}

	var ev model.Event
	ev.Kind = nostr.KindArticle
	ev.CreatedAt = nostr.Now()
	ev.Tags = model.Tags{
		{"k", "1"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var opCount int
		localRand := rand.New(rand.NewSource(rand.Int63()))

		for pb.Next() {
			writer := writers[localRand.Intn(len(writers))]
			_, _ = h.ConnAuth.Load(writer)

			if opCount%3 == 0 {
				nextWriter := spareWriters[localRand.Intn(len(spareWriters))]
				h.ConnAuth.Store(nextWriter, connAuthData{})
			} else {
				_ = h.BroadcastNewEvents(b.Context(), &ev)
			}
			opCount++
		}
	})
}
