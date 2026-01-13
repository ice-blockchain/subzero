// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/fixture"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidateDeviceRegistration(t *testing.T) {
	t.Parallel()

	const relayURL = "wss://example.com"

	key := model.GeneratePrivateKey()
	validator := newEventValidator(
		t.Context(),
		&Config{
			RelayURL: relayURL,
		},
		WithQueryFunc(new(fixture.MemDB).SelectEvents),
		WithIONIdentityPublicKeys(emptyIONIdentityKeys),
	)

	t.Run("valid event with minimal filter", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("relay url matches configuration with different port", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", "wss://example.com:1234"},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("relay url hostname matches configuration", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", "wss://example.com"},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("relay url doesn't match configuration", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", "wss://different-relay.example.com"},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		err := validator.Validate(t.Context(), model.Events{&ev})
		require.Error(t, err)
		require.Contains(t, err.Error(), "relay tag value")
		require.Contains(t, err.Error(), "does not match configured relay URL")
	})

	t.Run("valid event with iOS platform", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSIOS},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("valid event with web platform", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSWeb},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("missing d tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("missing t tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("invalid t tag value", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", "windows"},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("missing relay tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("invalid relay tag value", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", "wss://wrong-relay.example.com"},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("missing token tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("empty content", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = ""
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("invalid JSON content", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `{invalid json`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("skip giftwrap test", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
	})
}
