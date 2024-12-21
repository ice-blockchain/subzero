// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"

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
