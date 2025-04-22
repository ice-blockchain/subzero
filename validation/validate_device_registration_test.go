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

	tests := []struct {
		name    string
		setup   func(*model.Event)
		wantErr bool
	}{
		{
			name: "valid event with minimal filter",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "valid event with iOS platform",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSIOS},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "valid event with web platform",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSWeb},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "valid event with wrong complex filters",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[
					{"kinds":[1,4]},
					{"kinds":[1], "#p": ["pubkey1"]},
					{"kinds":[4], "#p": ["pubkey2"]}
				]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "valid event with allowed kinds in filter",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[
					{"kinds":[1, 30175, 6, 16]},
					{"kinds":[3]}
				]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "valid event with community filter",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[
					{"kinds":[1], "#h": ["community-id"]}
				]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "valid event with giftwrap filter",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "missing d tag",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "missing t tag",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "invalid t tag value",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", "windows"},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "missing relay tag",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "missing token tag",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "empty content",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = ""
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "invalid JSON content",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `{invalid json`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "filter without kinds",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"authors":["pubkey"]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "community filter without required text note kind",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[5], "#h": ["community-id"]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "filter with disallowed kind",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[5]}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
		{
			name: "skip giftwrap test",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1]}]`
				e.CreatedAt = 1
			},
			wantErr: false,
		},
		{
			name: "giftwrap_filter_missing_expiration",
			setup: func(e *model.Event) {
				e.Kind = model.CustomIONKindDeviceRegistration
				e.Tags = model.Tags{
					{"d", "device-id"},
					{"t", DeviceTokenOSAndroid},
					{"relay", "wss://relay.example.com"},
					{"token", "device-token"},
				}
				e.Content = `[{"kinds":[1059], "tags": {"p": ["pubkey1"], "k": ["1"]}}]`
				e.CreatedAt = 1
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ev model.Event
			tt.setup(&ev)
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			err := Validate(t.Context(), &ev)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateDeviceRegistrationFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid with text note",
			content: `[{"kinds":[1]}]`,
			wantErr: false,
		},
		{
			name:    "valid with editable text note",
			content: `[{"kinds":[30175]}]`,
			wantErr: false,
		},
		{
			name:    "valid with repost",
			content: `[{"kinds":[6]}]`,
			wantErr: false,
		},
		{
			name:    "valid with generic repost",
			content: `[{"kinds":[16]}]`,
			wantErr: false,
		},
		{
			name:    "valid with follow list",
			content: `[{"kinds":[3]}]`,
			wantErr: false,
		},
		{
			name:    "valid giftwrap with fund receive",
			content: `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["1755"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`,
			wantErr: false,
		},
		{
			name:    "valid giftwrap with fund send notify",
			content: `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["1756"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`,
			wantErr: false,
		},
		{
			name:    "valid giftwrap with direct message",
			content: `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["14"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`,
			wantErr: false,
		},
		{
			name:    "valid giftwrap with custom direct message",
			content: `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["30014"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`,
			wantErr: false,
		},
		{
			name:    "valid giftwrap with reaction",
			content: `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["7"], "expiration": ["` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `"]}]`,
			wantErr: false,
		},
		{
			name:    "giftwrap with expired expiration",
			content: `[{"kinds":[1059], "#p": ["some-pubkey"], "#k": ["7"], "expiration": ["` + strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10) + `"]}]`,
			wantErr: false,
		},
		{
			name:    "invalid kind",
			content: `[{"kinds":[7]}]`, // Reaction not directly allowed, only in giftwrap
			wantErr: true,
		},
		{
			name:    "giftwrap without k tag",
			content: `[{"kinds":[1059]}]`,
			wantErr: true,
		},
		{
			name:    "giftwrap with invalid k value",
			content: `[{"kinds":[1059], "#k": ["12345"]}]`,
			wantErr: true,
		},
		{
			name:    "community filter with valid kind",
			content: `[{"kinds":[1], "#h": ["community-id"]}]`,
			wantErr: false,
		},
		{
			name:    "community filter with editable text note",
			content: `[{"kinds":[30175], "#h": ["community-id"]}]`,
			wantErr: false,
		},
		{
			name:    "community filter with repost",
			content: `[{"kinds":[6], "#h": ["community-id"]}]`,
			wantErr: false,
		},
		{
			name:    "community filter with generic repost",
			content: `[{"kinds":[16], "#h": ["community-id"]}]`,
			wantErr: false,
		},
		{
			name:    "community filter with invalid kind",
			content: `[{"kinds":[7], "#h": ["community-id"]}]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key := model.GeneratePrivateKey()
			var ev model.Event
			ev.Kind = model.CustomIONKindDeviceRegistration
			ev.Tags = model.Tags{
				{"d", "device-id"},
				{"t", DeviceTokenOSAndroid},
				{"relay", "wss://relay.example.com"},
				{"token", "device-token"},
			}
			ev.Content = tt.content
			ev.CreatedAt = 1
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			err := Validate(t.Context(), &ev)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
