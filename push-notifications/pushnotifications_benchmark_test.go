// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

var (
	benchmarkKinds = []int{
		nostr.KindTextNote,
		nostr.KindReaction,
		nostr.KindRepost,
		nostr.KindGenericRepost,
		model.CustomIONKindEditableTextNote,
		model.CustomIONKindTokenizedCommunityDefinition,
		model.CustomIONKindTokenizedCommunityAction,
	}
)

func benchmarkSetupUsers(b *testing.B, pm *PushNotificationManager, userCount int, devicesPerUser int) []string {
	b.Helper()

	pubKeys := make([]string, userCount)

	for i := range userCount {
		privKey, pubKey := model.GenerateKeyPair()
		pubKeys[i] = pubKey

		for deviceNum := range devicesPerUser {
			deviceID := "device-" + pubKey[:8] + "-" + strconv.Itoa(deviceNum)

			randomAuthor := pubKeys[rand.IntN(max(1, i+1))]
			randomKind := benchmarkKinds[rand.IntN(len(benchmarkKinds))]

			filters := model.Filters{
				{
					Authors: []string{randomAuthor},
					Kinds:   []int{randomKind},
				},
			}

			var deviceEvent model.Event
			deviceEvent.Kind = model.CustomIONKindDeviceRegistration
			deviceEvent.CreatedAt = nostr.Now()
			deviceEvent.Content = filters.String()
			deviceEvent.Tags = model.Tags{
				{"d", deviceID},
				{"t", []string{"ios", "android"}[deviceNum%2]},
				{"relay", pm.relayURL},
				{"token", "token-" + deviceID},
			}
			require.NoError(b, deviceEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(b, pm.processDeviceRegistrationEvent(b.Context(), &deviceEvent))
		}
	}

	return pubKeys
}

func benchmarkCreateTestEvent(pubKeys []string) *model.Event {
	randomAuthor := pubKeys[rand.IntN(len(pubKeys))]
	randomKind := benchmarkKinds[rand.IntN(len(benchmarkKinds))]

	var ev model.Event
	ev.Kind = randomKind
	ev.CreatedAt = 1
	ev.PubKey = randomAuthor
	ev.Content = "benchmark test content"
	ev.Tags = model.Tags{
		{"d", "some-random-device-d-tag"},
	}

	return &ev
}

func BenchmarkCollectTargetDevices(b *testing.B) {
	if os.Getenv("CI") != "" {
		b.Skip("Skipping benchmark tests in CI environment as they require significant resources")
	}

	const devicesPerUser = 2

	benchmarks := []struct {
		name      string
		userCount int
	}{
		{"10k_users", 10_000},
		{"50k_users", 50_000},
		{"100k_users", 100_000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			pm := helperNewManager(b)
			pubKeys := benchmarkSetupUsers(b, pm, bm.userCount, devicesPerUser)

			testEvent := benchmarkCreateTestEvent(pubKeys)

			b.ResetTimer()
			for b.Loop() {
				_ = pm.collectLocalDevices("", testEvent)
			}
		})
	}
}
