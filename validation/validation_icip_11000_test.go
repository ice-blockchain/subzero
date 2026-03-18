// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidateTokenizedCommunityFirstBuy(t *testing.T) {
	t.Parallel()

	pk := model.GeneratePrivateKey()

	var userPost model.Event
	userPost.Kind = nostr.KindTextNote
	userPost.CreatedAt = nostr.Now()
	userPost.Content = "Hello, world!"
	require.NoError(t, userPost.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var postTokenizedEvent model.Event
	postTokenizedEvent.Kind = model.CustomIONKindTokenizedCommunityDefinition
	postTokenizedEvent.CreatedAt = nostr.Now()
	postTokenizedEvent.Tags = model.Tags{
		{"e", userPost.Address()},
		{"k", strconv.Itoa(userPost.Kind)},
	}
	require.NoError(t, postTokenizedEvent.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var userProfileWithoutWallet model.Event
	userProfileWithoutWallet.Kind = nostr.KindProfileMetadata
	userProfileWithoutWallet.CreatedAt = nostr.Now()
	userProfileWithoutWallet.Content = model.ProfileMetadataContent{
		Name:  "Alice",
		About: "Just a test user",
	}.String()
	require.NoError(t, userProfileWithoutWallet.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var userProfileWithWallet model.Event
	userProfileWithWallet.Kind = nostr.KindProfileMetadata
	userProfileWithWallet.CreatedAt = nostr.Now()
	userProfileWithWallet.Content = model.ProfileMetadataContent{
		Name:  "Alice",
		About: "Just a test user",
		Wallets: map[string]string{
			"bsc": "0x1234567890abcdef1234567890abcdef12345678",
		},
	}.String()
	require.NoError(t, userProfileWithWallet.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	t.Run("Defination event without 'p' tag", func(t *testing.T) {
		var ev model.Event

		ev.Kind = model.CustomIONKindTokenizedCommunityDefinition
		ev.Tags = model.Tags{
			{"a", "tc_event_address"},
			{"k", "0"},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), &eventValidator{}, &ev, nil))
	})
	t.Run("Defination event with 'p' tag for xcom", func(t *testing.T) {
		var ev model.Event

		ev.Kind = model.CustomIONKindTokenizedCommunityDefinition
		ev.Tags = model.Tags{
			{"h", "tc_event_address"},
			{"k", "1"},
			{"p", "creator_pubkey"},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), &eventValidator{}, &ev, nil))
	})
	t.Run("First buy without tokenized event", func(t *testing.T) {
		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
				}
			},
		}
		var ev model.Event

		ev.Kind = model.CustomIONKindTokenizedCommunityDefinition
		ev.Tags = model.Tags{
			{"a", "some_event_address"},
			{"p", "creator_pubkey"},
			{"k", "1"},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.ErrorIs(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &ev, nil), ErrNotFound)
	})
	t.Run("First buy without tokenized event in broadcast mode", func(t *testing.T) {
		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
				}
			},
		}
		var ev model.Event

		ev.Kind = model.CustomIONKindTokenizedCommunityDefinition
		ev.Tags = model.Tags{
			{"a", "some_event_address"},
			{"p", "creator_pubkey"},
			{"k", "1"},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &ev, &ruleSet{BroadcastMode: true}))
	})
	t.Run("Missing profile metadata event", func(t *testing.T) {
		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					yield(&postTokenizedEvent, nil)
				}
			},
		}
		ev := postTokenizedEvent
		ev.Tags = slices.Clone(postTokenizedEvent.Tags)
		ev.Tags = append(ev.Tags, model.Tag{"p", postTokenizedEvent.GetMasterPublicKey()})

		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.ErrorIs(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &ev, nil), ErrNotFound)
	})
	t.Run("Missing profile wallet", func(t *testing.T) {
		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					if yield(&postTokenizedEvent, nil) {
						yield(&userProfileWithoutWallet, nil)
					}
				}
			},
		}
		ev := postTokenizedEvent
		ev.Tags = slices.Clone(postTokenizedEvent.Tags)
		ev.Tags = append(ev.Tags, model.Tag{"p", postTokenizedEvent.GetMasterPublicKey()})

		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.ErrorIs(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &ev, nil), ErrWalletRequired)
	})
	t.Run("Valid first buy", func(t *testing.T) {
		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					if yield(&postTokenizedEvent, nil) {
						yield(&userProfileWithWallet, nil)
					}
				}
			},
		}
		ev := postTokenizedEvent
		ev.Tags = slices.Clone(postTokenizedEvent.Tags)
		ev.Tags = append(ev.Tags, model.Tag{"p", postTokenizedEvent.GetMasterPublicKey()})

		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &ev, nil))
	})
	t.Run("Tokenized event is profile metadata", func(t *testing.T) {
		var tcDef, tcBuy model.Event

		tcDef.Kind = model.CustomIONKindTokenizedCommunityDefinition
		tcDef.CreatedAt = nostr.Now()
		tcDef.Tags = model.Tags{
			{"a", userProfileWithWallet.Address()},
			{"k", strconv.Itoa(userProfileWithWallet.Kind)},
		}
		require.NoError(t, tcDef.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		tcBuy = tcDef
		tcBuy.Tags = slices.Clone(tcDef.Tags)
		tcBuy.Tags = append(tcBuy.Tags, model.Tag{"p", userProfileWithWallet.GetMasterPublicKey()})
		require.NoError(t, tcBuy.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					yield(&userProfileWithWallet, nil)
				}
			},
		}

		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &tcBuy, nil))
	})
	t.Run("Tokenize kind 1 comment with e root tag", func(t *testing.T) {
		var rootPost model.Event
		rootPost.Kind = nostr.KindTextNote
		rootPost.CreatedAt = nostr.Now()
		rootPost.Content = "Root post content"
		require.NoError(t, rootPost.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		var comment model.Event
		comment.Kind = nostr.KindTextNote
		comment.CreatedAt = nostr.Now()
		comment.Content = "This is a comment"
		comment.Tags = model.Tags{
			{"e", rootPost.ID, "", "root"},
			{"e", rootPost.ID, "", "reply"},
		}
		require.NoError(t, comment.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.True(t, comment.IsComment(), "event should be identified as a comment")

		var tcBuy model.Event
		tcBuy.Kind = model.CustomIONKindTokenizedCommunityDefinition
		tcBuy.CreatedAt = nostr.Now()
		tcBuy.Tags = model.Tags{
			{"e", comment.ID},
			{"k", strconv.Itoa(comment.Kind)},
			{"t", "community_token"},
			{"t", "community_token_action"},
			{"p", comment.GetMasterPublicKey()},
		}
		require.NoError(t, tcBuy.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					if yield(&comment, nil) {
						yield(&userProfileWithWallet, nil)
					}
				}
			},
		}
		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &tcBuy, nil))
	})

	t.Run("Tokenize kind 30175 comment with a root tag", func(t *testing.T) {
		var rootPost model.Event
		rootPost.Kind = model.CustomIONKindEditableTextNote
		rootPost.CreatedAt = nostr.Now()
		rootPost.Content = "Editable root post"
		rootPost.Tags = model.Tags{
			{"d", "editable-post-1"},
		}
		require.NoError(t, rootPost.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		var comment model.Event
		comment.Kind = model.CustomIONKindEditableTextNote
		comment.CreatedAt = nostr.Now()
		comment.Content = "Comment on editable post"
		comment.Tags = model.Tags{
			{"d", "comment-1"},
			{"a", rootPost.Address(), "", "root"},
			{"a", rootPost.Address(), "", "reply"},
		}
		require.NoError(t, comment.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.True(t, comment.IsComment(), "event should be identified as a comment")

		var tcBuy model.Event
		tcBuy.Kind = model.CustomIONKindTokenizedCommunityDefinition
		tcBuy.CreatedAt = nostr.Now()
		tcBuy.Tags = model.Tags{
			{"a", comment.Address()},
			{"k", strconv.Itoa(comment.Kind)},
			{"t", "community_token"},
			{"t", "community_token_action"},
			{"p", comment.GetMasterPublicKey()},
		}
		require.NoError(t, tcBuy.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					if yield(&comment, nil) {
						yield(&userProfileWithWallet, nil)
					}
				}
			},
		}
		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &tcBuy, nil))
	})

	t.Run("Tokenize nested comment (reply to comment)", func(t *testing.T) {
		var rootPost model.Event
		rootPost.Kind = nostr.KindTextNote
		rootPost.CreatedAt = nostr.Now()
		rootPost.Content = "Root post"
		require.NoError(t, rootPost.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		var firstComment model.Event
		firstComment.Kind = nostr.KindTextNote
		firstComment.CreatedAt = nostr.Now()
		firstComment.Content = "First comment"
		firstComment.Tags = model.Tags{
			{"e", rootPost.ID, "", "root"},
			{"e", rootPost.ID, "", "reply"},
		}
		require.NoError(t, firstComment.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		var nestedComment model.Event
		nestedComment.Kind = nostr.KindTextNote
		nestedComment.CreatedAt = nostr.Now()
		nestedComment.Content = "Reply to comment"
		nestedComment.Tags = model.Tags{
			{"e", rootPost.ID, "", "root"},
			{"e", firstComment.ID, "", "reply"},
		}
		require.NoError(t, nestedComment.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.True(t, nestedComment.IsComment(), "nested reply should be identified as a comment")

		var tcBuy model.Event
		tcBuy.Kind = model.CustomIONKindTokenizedCommunityDefinition
		tcBuy.CreatedAt = nostr.Now()
		tcBuy.Tags = model.Tags{
			{"e", nestedComment.ID},
			{"k", strconv.Itoa(nestedComment.Kind)},
			{"t", "community_token"},
			{"t", "community_token_action"},
			{"p", nestedComment.GetMasterPublicKey()},
		}
		require.NoError(t, tcBuy.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		v := &eventValidator{
			QueryFunc: func(ctx context.Context, f ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					if yield(&nestedComment, nil) {
						yield(&userProfileWithWallet, nil)
					}
				}
			},
		}

		require.NoError(t, validateTokenizedCommunityFirstBuy(t.Context(), v, &tcBuy, nil))
	})
}
