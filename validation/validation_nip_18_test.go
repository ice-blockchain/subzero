// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
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

	profileMetadata := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindProfileMetadata,
			Content: fmt.Sprintf(`{"name":"testuser","display_name":"Test User","ion_content_nft_collections":{"%v":{"address":"0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290","created_by":"0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4"}}}`, "ion"),
			Tags:    model.Tags{{"b", addressableEvent.GetMasterPublicKey()}},
		},
	}
	require.NoError(t, profileMetadata.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	expiredPollEvent := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagPoll, "type single", "ttl 1", "title Test single Poll", "options [\"Option 1\", \"Option 2\"]"},
			},
		},
	}
	require.NoError(t, expiredPollEvent.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

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
			name: "valid repost of the expired poll",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindRepost,
					Content: expiredPollEvent.String(),
					Tags: model.Tags{
						{"e", expiredPollEvent.Address()},
						{"p", expiredPollEvent.PubKey},
						{"k", strconv.Itoa(expiredPollEvent.Kind)},
					},
				},
			},
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
			validator := newEventValidator(appcontext.TContext(t), cfg.MustGet[Config](), WithIONIdentityPublicKeys(emptyIONIdentityKeys), WithQueryFunc(func(ctx context.Context, filters ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					for _, filter := range filters {
						hasProfileKind := false
						for _, kind := range filter.Kinds {
							if kind == nostr.KindProfileMetadata {
								hasProfileKind = true

								break
							}
						}
						if !hasProfileKind {
							continue
						}
						for _, author := range filter.Authors {
							if author == addressableEvent.GetMasterPublicKey() {
								yield(profileMetadata, nil)

								return
							}
						}
					}
				}
			}))
			err := validator.Validate(t.Context(), model.Events{tt.event})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
