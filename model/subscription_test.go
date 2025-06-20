// SPDX-License-Identifier: ice License 1.0

package model

import (
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func helperCreateMockEvent(t *testing.T, kinds []int, pk []string) *Event {
	t.Helper()

	var ev Event

	ev.CreatedAt = nostr.Now()
	ev.Kind = kinds[rand.IntN(len(kinds))]
	if !ev.IsRegular() {
		ev.Tags = append(ev.Tags, Tag{"d", ev.CreatedAt.String()})
	}

	require.NoError(t, ev.SignWithAlg(pk[rand.IntN(len(pk))], SignAlgEDDSA, KeyAlgCurve25519))

	return &ev
}

func TestSubscription_Push_Concurrent(t *testing.T) {
	sub := NewSubscription("test-sub", Filters{})

	const numGoroutines = 5
	const numEventTotal = 10000

	kinds := []Kind{nostr.KindProfileMetadata, nostr.KindTextNote, CustomIONKindEditableTextNote}
	pk := []string{GeneratePrivateKey(), GeneratePrivateKey()}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()

			for range numEventTotal {
				sub.Push(helperCreateMockEvent(t, kinds, pk))
			}
		}()
	}
	wg.Wait()

	addrlen := len(sub.addresses)
	pending := sub.GetPending()
	require.Lenf(t, pending, addrlen, "pending events [%d] should match the number of unique addresses [%d]", len(pending), addrlen)
}
