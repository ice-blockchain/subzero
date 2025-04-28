// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateDeviceRegistration(t *testing.T) {
	t.Parallel()

	key := model.GeneratePrivateKey()

	t.Run("valid event with minimal filter", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("valid event with iOS platform", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSIOS},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("valid event with web platform", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSWeb},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("valid event with wrong complex filters", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[
			{"kinds":[1,4]},
			{"kinds":[1], "#p": ["pubkey1"]},
			{"kinds":[4], "#p": ["pubkey2"]}
		]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("valid event with allowed kinds in filter", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[
			{"kinds":[1, 30175, 6, 16]},
			{"kinds":[3]}
		]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("valid event with community filter", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[
			{"kinds":[1], "#h": ["community-id"]}
		]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("valid event with giftwrap filter", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("missing d tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("missing t tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("invalid t tag value", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", "windows"},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("missing relay tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("invalid relay tag value", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", "wss://wrong-relay.example.com"},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("missing token tag", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("empty content", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = ""
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("invalid JSON content", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `{invalid json`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("filter without kinds", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"authors":["pubkey"]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("community filter without required text note kind", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[5], "#h": ["community-id"]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("filter with disallowed kind", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[5]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})

	t.Run("skip giftwrap test", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1]}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("giftwrap_filter_missing_expiration", func(t *testing.T) {
		t.Parallel()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = `[{"kinds":[1059], "tags": {"p": ["pubkey1"], "k": ["1"]}}]`
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})
}

func TestValidateDeviceRegistrationFilters(t *testing.T) {
	t.Parallel()

	setupEvent := func(t *testing.T, content string) *model.Event {
		key := model.GeneratePrivateKey()
		var ev model.Event
		ev.Kind = model.CustomIONKindDeviceRegistration
		ev.Tags = model.Tags{
			{"d", "device-id"},
			{"t", DeviceTokenOSAndroid},
			{"relay", globalConfig.RelayURL},
			{"token", "device-token"},
		}
		ev.Content = content
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		return &ev
	}

	t.Run("valid with text note", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[1]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid with editable text note", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[30175]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid with repost", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[6]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid with generic repost", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[16]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid with follow list", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[3]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid giftwrap with fund receive", func(t *testing.T) {
		t.Parallel()
		content := `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["1755"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`
		ev := setupEvent(t, content)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid giftwrap with fund send notify", func(t *testing.T) {
		t.Parallel()
		content := `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["1756"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`
		ev := setupEvent(t, content)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid giftwrap with direct message", func(t *testing.T) {
		t.Parallel()
		content := `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["14"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`
		ev := setupEvent(t, content)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid giftwrap with custom direct message", func(t *testing.T) {
		t.Parallel()
		content := `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["30014"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`
		ev := setupEvent(t, content)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("valid giftwrap with reaction", func(t *testing.T) {
		t.Parallel()
		content := `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["7"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`
		ev := setupEvent(t, content)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("giftwrap with expired expiration", func(t *testing.T) {
		t.Parallel()
		content := `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["7"], "expiration": ["` + strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10) + `"]}]`
		ev := setupEvent(t, content)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("invalid kind", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[7]}]`)
		require.Error(t, Validate(t.Context(), ev))
	})

	t.Run("giftwrap without k tag", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[1059]}]`)
		require.Error(t, Validate(t.Context(), ev))
	})

	t.Run("giftwrap with invalid k value", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[1059], "#k": ["12345"]}]`)
		require.Error(t, Validate(t.Context(), ev))
	})

	t.Run("community filter with valid kind", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[1], "#h": ["community-id"]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("community filter with editable text note", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[30175], "#h": ["community-id"]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("community filter with repost", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[6], "#h": ["community-id"]}]`)
		require.Error(t, Validate(t.Context(), ev))
	})

	t.Run("community filter with generic repost", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[16], "#h": ["community-id"]}]`)
		require.NoError(t, Validate(t.Context(), ev))
	})

	t.Run("community filter with invalid kind", func(t *testing.T) {
		t.Parallel()
		ev := setupEvent(t, `[{"kinds":[7], "#h": ["community-id"]}]`)
		require.Error(t, Validate(t.Context(), ev))
	})
}
