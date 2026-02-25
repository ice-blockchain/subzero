// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/fixture"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidateDeviceRegistration(t *testing.T) {
	t.Parallel()

	const (
		relayURL        = "wss://example.com"
		validKindFilter = `[{"kinds":[1]}]`
	)
	var filterEvent model.Event
	var memdb fixture.MemDB

	key, pubkey := model.GenerateKeyPair()
	validator := newEventValidator(
		t.Context(),
		&Config{
			RelayURL: relayURL,
		},
		WithIONIdentityPublicKeys(emptyIONIdentityKeys),
		WithQueryFunc(memdb.SelectEvents),
	)

	filtersWithEvents := model.FiltersWithEvents{
		Filters: []model.Filter{
			{
				Kinds: []int{1},
			},
		},
		Data: model.Events{&filterEvent},
	}

	filterEvent.Kind = 5175
	filterEvent.CreatedAt = nostr.Now()
	require.NoError(t, filterEvent.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	t.Run("Authoritative", func(t *testing.T) {
		ctx := model.SetUserDataInContext(t.Context(), model.UserDataContext{
			Authenticated:   true,
			Authoritative:   true,
			MasterPublicKey: pubkey,
			PublicKey:       pubkey,
		})
		t.Run("valid event with minimal filter", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("relay url matches configuration with different port", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", "wss://example.com:1234"},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("relay url hostname matches configuration", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", "wss://example.com"},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("relay url doesn't match configuration", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", "wss://different-relay.example.com"},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := validator.Validate(ctx, model.Events{&ev})
			require.Error(t, err)
			require.Contains(t, err.Error(), "relay tag value")
			require.Contains(t, err.Error(), "does not match configured relay URL")
		})

		t.Run("valid event with iOS platform", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSIOS},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("valid event with web platform", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSWeb},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("missing d tag", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"t", model.DeviceTokenOSAndroid},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("missing t tag", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("invalid t tag value", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", "windows"},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("missing relay tag", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("invalid relay tag value", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", "wss://wrong-relay.example.com"},
				{"token", "device-token"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("empty content", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})

		t.Run("invalid JSON content", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSAndroid},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = `{invalid json`
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(ctx, model.Events{&ev}))
		})
		t.Run("valid event with event fitlers", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", model.DeviceTokenOSWeb},
				{"relay", relayURL},
				{"token", "device-token"},
			}
			ev.Content = filtersWithEvents.String()
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(ctx, model.Events{&ev}))
		})
	})
	t.Run("Non-Authoritative", func(t *testing.T) {
		var evAttestation, evRelayListing model.Event

		evAttestation.Kind = model.CustomIONKindAttestation
		require.NoError(t, evAttestation.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		evRelayListing.Kind = nostr.KindRelayListMetadata
		evRelayListing.Tags = model.Tags{
			{"r", relayURL},
		}
		require.NoError(t, evRelayListing.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), &evAttestation, &evRelayListing))

		t.Run("OK", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", pubkey + "_device-id"},
				{model.CustomIONTagOnBehalfOf, "foo"},
				{"relay", relayURL},
				{"relay", "wss://another-relay.example.com"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
		})
		t.Run("no relay tags", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", pubkey + "_device-id"},
				{model.CustomIONTagOnBehalfOf, "foo"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
		})
		t.Run("invalid relay url", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", pubkey + "_device-id"},
				{model.CustomIONTagOnBehalfOf, "foo"},
				{"relay", "invalid-url"},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
		})
		t.Run("same master key", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", pubkey + "_device-id"},
				{model.CustomIONTagOnBehalfOf, pubkey},
				{"relay", relayURL},
			}
			ev.Content = validKindFilter
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
		})
	})
}
