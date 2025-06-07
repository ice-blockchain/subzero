// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"strconv"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestValidateKindRepostEvent(t *testing.T) {
	t.Parallel()

	// Create original event to be reposted.
	originalEvent := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindTextNote,
			Content: "original content",
		},
	}
	require.NoError(t, originalEvent.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	// Create original addressable event.
	addressableEvent := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindArticle,
			Content: "addressable content",
			Tags:    model.Tags{{"d", "test"}},
		},
	}
	require.NoError(t, addressableEvent.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	tests := []struct {
		name    string
		event   *model.Event
		wantErr bool
	}{
		{
			name: "valid repost of text note",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: originalEvent.String(),
					Tags:    model.Tags{{"e", originalEvent.ID}, {"p", originalEvent.PubKey}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid generic repost",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindGenericRepost,
					Content: addressableEvent.String(),
					Tags: model.Tags{
						{"a", addressableEvent.Address()},
						{"p", addressableEvent.PubKey},
						{"k", strconv.Itoa(addressableEvent.Kind)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid content - not JSON",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: "not json",
					Tags:    model.Tags{{"e", originalEvent.ID}, {"p", originalEvent.PubKey}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid repost - wrong kind",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: addressableEvent.String(),
					Tags:    model.Tags{{"e", addressableEvent.ID}, {"p", addressableEvent.PubKey}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid generic repost - wrong k tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindGenericRepost,
					Content: addressableEvent.String(),
					Tags: model.Tags{
						{"a", addressableEvent.Address()},
						{"p", addressableEvent.PubKey},
						{"k", "999"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid repost - wrong pubkey",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: originalEvent.String(),
					Tags:    model.Tags{{"e", originalEvent.ID}, {"p", "wrongpubkey"}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid repost - missing e tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: originalEvent.String(),
					Tags:    model.Tags{{"p", originalEvent.PubKey}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid repost - wrong e tag value",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: originalEvent.String(),
					Tags:    model.Tags{{"e", "wrongid"}, {"p", originalEvent.PubKey}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid generic repost - missing a tag for addressable",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindGenericRepost,
					Content: addressableEvent.String(),
					Tags: model.Tags{
						{"p", addressableEvent.PubKey},
						{"k", strconv.Itoa(addressableEvent.Kind)},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid generic repost - using e tag instead of a tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindGenericRepost,
					Content: addressableEvent.String(),
					Tags: model.Tags{
						{"e", addressableEvent.ID},
						{"p", addressableEvent.PubKey},
						{"k", strconv.Itoa(addressableEvent.Kind)},
					},
				},
			},
			wantErr: true,
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
