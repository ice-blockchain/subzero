// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateDeviceRegistration(t *testing.T) {
	t.Parallel()

	const (
		relayURL        = "wss://example.com"
		validKindFilter = `[{"kinds":[1]}]`
	)

	key := model.GeneratePrivateKey()
	validator := newEventValidator(
		t.Context(),
		&Config{
			RelayURL: relayURL,
		},
		WithIONIdentityPublicKeys(emptyIONIdentityKeys),
	)

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
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("invalid relay tag value", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"relay", "\x00"},
			{"token", "device-token"},
		}
		ev.Content = validKindFilter
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})

	t.Run("missing token tag", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", model.DeviceTokenOSAndroid},
			{"relay", relayURL},
		}
		ev.Content = validKindFilter
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
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
		require.Error(t, validator.Validate(t.Context(), model.Events{&ev}))
	})
}
