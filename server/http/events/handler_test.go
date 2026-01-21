// SPDX-License-Identifier: ice License 1.0

package events

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestUpdatePreviewWithEvent(t *testing.T) {
	preview := &PostPreview{}
	authorPubKey := "pubkey_author_123"
	masterKey := "master_key_456"
	events := []*model.Event{
		{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				PubKey:  authorPubKey,
				Content: `{"name":"username", "display_name":"DisplayName", "picture":"https://example.com/avatar.png"}`,
				Tags:    nostr.Tags{{"b", masterKey}},
			},
		},
		{
			Event: nostr.Event{
				Kind:      model.CustomIONKindEditableTextNote,
				PubKey:    authorPubKey,
				Content:   "Check this out! #ice #subzero",
				CreatedAt: nostr.Timestamp(1705750000),
				Tags: nostr.Tags{
					{"imeta", "url https://example.com/video.mp4", "m video/mp4", "thumb https://ice.io/thumb.jpg"},
					{"b", masterKey},
				},
			},
		},
		{
			Event: nostr.Event{
				Kind:   nostr.KindBadgeAward,
				PubKey: "123",
				Tags: nostr.Tags{
					{"a", "30009:123:verified"},
					{"p", masterKey},
				},
			},
		},
		{
			Event: nostr.Event{
				Kind:    model.KindJobNostrEventCount + 1000,
				Content: `{"+":5000}`,
				Tags: nostr.Tags{
					{"request", `{"kind":5400, "content":"[{\"kinds\":[7]}]"}`},
				},
			},
		},
		{
			Event: nostr.Event{
				Kind:    model.KindJobNostrEventCount + 1000,
				Content: "25",
				Tags: nostr.Tags{
					{"request", `{"kind":5400, "content":"[{\"kinds\":[30175]}]"}`},
				},
			},
		},
	}

	for _, ev := range events {
		err := updatePreviewWithEvent(preview, ev)
		require.NoError(t, err)
	}
	require.Equal(t, "username", preview.Author.Name)
	require.Equal(t, "DisplayName", preview.Author.DisplayName)
	require.Equal(t, "https://example.com/avatar.png", preview.Author.Avatar)
	require.True(t, preview.Author.Verified, "Author should be verified")

	require.Equal(t, "video", preview.Type)
	require.Contains(t, preview.Content, "Check this out!")
	require.Equal(t, 1705750000, int(preview.CreatedAt.Unix()))

	require.Len(t, preview.Media, 1)
	require.Equal(t, "https://example.com/video.mp4", preview.Media[0].URL)
	require.Equal(t, "video", preview.Media[0].Type)
	require.NotNil(t, preview.Media[0].Thumbnail)

	require.Equal(t, 5000, preview.Likes)
	require.Equal(t, 25, preview.Comments)
}
