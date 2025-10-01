// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"

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
