// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateFollowListEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   *model.Event
		wantErr bool
	}{
		{
			name: "valid follow list with single pubkey",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindFollowList,
					Tags: model.Tags{{"p", "valid_pubkey_1"}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid follow list with multiple pubkeys",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindFollowList,
					Tags: model.Tags{
						{"p", "valid_pubkey_1"},
						{"p", "valid_pubkey_2"},
						{"p", "valid_pubkey_3"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "follow list with empty pubkey",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindFollowList,
					Tags: model.Tags{{"p", ""}},
				},
			},
			wantErr: true,
		},
		{
			name: "follow list with duplicate pubkeys",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindFollowList,
					Tags: model.Tags{
						{"p", "valid_pubkey_1"},
						{"p", "valid_pubkey_1"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "follow list with mixed valid and invalid pubkeys",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindFollowList,
					Tags: model.Tags{
						{"p", "valid_pubkey_1"},
						{"p", ""},
						{"p", "valid_pubkey_2"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty follow list",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindFollowList,
					Tags: model.Tags{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := Validate(t.Context(), tt.event)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
