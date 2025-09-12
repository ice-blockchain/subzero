// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateReactionsAndTagsOneOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   *model.Event
		wantErr bool
	}{
		{
			name: "valid reaction with e tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindReaction,
					Content: "+",
					Tags: model.Tags{
						{"e", "event_id"},
						{"p", "pubkey"},
						{"k", "1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid reaction with a tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindReaction,
					Content: "+",
					Tags: model.Tags{
						{"a", "30023:alice:article"},
						{"p", "pubkey"},
						{"k", "1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid reaction with both e and a tags",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindReaction,
					Content: "+",
					Tags: model.Tags{
						{"e", "event_id"},
						{"a", "30023:alice:article"},
						{"p", "pubkey"},
						{"k", "1"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid reaction with no e or a tags",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindReaction,
					Content: "+",
					Tags: model.Tags{
						{"p", "pubkey"},
						{"k", "1"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid reaction missing required p tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindReaction,
					Content: "+",
					Tags: model.Tags{
						{"e", "event_id"},
						{"k", "1"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid reaction missing required k tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindReaction,
					Tags: model.Tags{
						{"e", "event_id"},
						{"p", "pubkey"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := Validate(t.Context(), model.Events{tt.event})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
