// SPDX-License-Identifier: ice License 1.0

package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestValidateGiftWrap(t *testing.T) {
	t.Parallel()

	key := GeneratePrivateKey()

	var ev Event
	ev.Kind = nostr.KindGiftWrap
	ev.CreatedAt = 1
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())

	ev.Tags = append(ev.Tags, Tag{"p", "foop"}, Tag{"k", "123"})
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())

	ev.Tags = append(ev.Tags, Tag{"expiration", "foo"})
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())

	ev.Tags = append(ev.Tags[:len(ev.Tags)-2], Tag{"expiration", "123"})
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())
}

func TestValidatePollTag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Tag Tag
		Err error
	}{
		{Tag{CustomIONTagPoll, "type single", "ttl 3600", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{Tag{CustomIONTagPoll, "type multi", "ttl 3600", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{Tag{CustomIONTagPoll, "type invalid", "ttl 3600", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type single", "ttl -1", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type single", "ttl 3600", "title", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type single", "ttl 3600", "title Test Poll", "options []"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type single", "ttl 3600", "title Test Poll"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type single", "ttl 3600", "title ", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type multi", "ttl 3600", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type multi", "ttl 3600", "title Test Poll", "options [\"Option 1\", \"Option 2\"]", "somekey2 someval"}, ErrWrongEventParams},
		{Tag{CustomIONTagPoll, "type multi", "ttl 3600", "title Test Poll", "options [foo]"}, ErrWrongEventParams},
	}
	for _, c := range cases {
		err := validatePollTag(c.Tag)
		if c.Err != nil {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}

	t.Run("OptionsInTheEvent", func(t *testing.T) {
		var ev Event

		ev.Kind = nostr.KindTextNote
		ev.Content = "Test Content"
		ev.Tags = Tags{
			{CustomIONTagPoll, "type single", "ttl 3600", "title Test Poll", `options ["Option 1", "Option 2"]`},
			{"e", "123"},
		}
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))
		require.NoError(t, ev.Validate())
	})
}

func TestPollVoteExpiration(t *testing.T) {
	t.Parallel()

	var ev Event

	ev.Kind = CustomIONKindPollVote
	ev.CreatedAt = 1
	ev.Tags = Tags{
		{"e", "123"},
	}
	require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))
	require.NoError(t, ev.Validate())

	ev.Tags = append(ev.Tags, Tag{"expiration", "123"})
	require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())
}

func TestValidateSettingsTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     Tag
		kind    int
		wantNil bool
	}{
		{
			name:    "no timestamp at the settings",
			tag:     nostr.Tag{"settings", "foo", "0"},
			kind:    KindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "invalid settings length",
			tag:     nostr.Tag{"settings", "foo"},
			kind:    KindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "invalid settings value for comments_enabled",
			tag:     nostr.Tag{"settings", "comments_enabled", "bar", fmt.Sprint(time.Now().Unix())},
			kind:    KindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "valid settings value for comments_enabled",
			tag:     nostr.Tag{"settings", "comments_enabled", "true", fmt.Sprint(time.Now().Unix())},
			kind:    KindCommunityDefinition,
			wantNil: true,
		},
		{
			name:    "valid settings value for comments_enabled",
			tag:     nostr.Tag{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
			kind:    KindCommunityDefinition,
			wantNil: true,
		},
		{
			name:    "wrong kind for comments_enabled settings",
			tag:     nostr.Tag{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: false,
		},
		{
			name:    "invalid settings value for role_required_for_posting",
			tag:     nostr.Tag{"settings", "role_required_for_posting", "admin", fmt.Sprint(time.Now().Unix())},
			kind:    KindCommunityDefinition,
			wantNil: true,
		},
		{
			name:    "invalid settings value for role_required_for_posting",
			tag:     nostr.Tag{"settings", "role_required_for_posting", "moderator", fmt.Sprint(time.Now().Unix())},
			kind:    KindCommunityDefinition,
			wantNil: true,
		},
		{
			name:    "wrong kind for role_required_for_posting settings",
			tag:     nostr.Tag{"settings", "role_required_for_posting", "admin", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: false,
		},
		{
			name:    "invalid settings value for role_required_for_posting",
			tag:     nostr.Tag{"settings", "role_required_for_posting", "dummy", fmt.Sprint(time.Now().Unix())},
			kind:    KindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "following,mentioned,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: true,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "following", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindArticle,
			wantNil: true,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "mentioned", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: true,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: true,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "following,mentioned", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: true,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "mentioned,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: true,
		},
		{
			name:    "valid settings value for who_can_reply",
			tag:     nostr.Tag{"settings", "who_can_reply", "dummy,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			kind:    nostr.KindTextNote,
			wantNil: false,
		},
		{
			name:    "wrong kind for who_can_reply settings",
			tag:     nostr.Tag{"settings", "who_can_reply", "following,mentioned,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			kind:    KindComment,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateSettingsTag(tt.kind, tt.tag)
			if got == nil && !tt.wantNil || got != nil && tt.wantNil {
				t.Fatalf("validateSettingsTag(%v) = nil, wantNil %v", tt.tag, tt.wantNil)
			}
		})
	}
}
