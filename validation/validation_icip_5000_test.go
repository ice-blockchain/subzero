// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/fixture"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidatePollTag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Tag model.Tag
		Err error
	}{
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl 3000000000", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{model.Tag{model.CustomIONTagPoll, "type multi", "ttl 3000000000", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{model.Tag{model.CustomIONTagPoll, "type multi", "ttl 0", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{model.Tag{model.CustomIONTagPoll, "type multi", "ttl 1000000000", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type invalid", "ttl 3000000000", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl -1", "title Test Poll", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl 3000000000", "title", "options [\"Option 1\", \"Option 2\"]"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl 3000000000", "title Test Poll", "options []"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl 3000000000", "title Test Poll"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl 3000000000", "title ", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{model.Tag{model.CustomIONTagPoll, "type single", "ttl 3000000000", "options [\"Option 1\", \"Option 2\"]"}, nil},
		{model.Tag{model.CustomIONTagPoll, "type multi", "ttl 3000000000", "title Test Poll", "options [\"Option 1\", \"Option 2\"]", "somekey2 someval"}, ErrWrongEventParams},
		{model.Tag{model.CustomIONTagPoll, "type multi", "ttl 3000000000", "title Test Poll", "options [foo]"}, ErrWrongEventParams},
	}
	for i, c := range cases {
		t.Run("test case "+strconv.Itoa(i), func(t *testing.T) {
			err := validatePollTag(c.Tag)
			if c.Err != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("OptionsInTheEvent", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindTextNote
		ev.Content = "Test Content"
		ev.Tags = model.Tags{
			{model.CustomIONTagPoll, "type single", "ttl 3000000000", "title Test Poll", `options ["Option 1", "Option 2"]`},
			{"e", "123"},
		}
		ev.CreatedAt = 1
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), model.Events{&ev}))
	})
}

func TestValidatePollVote(t *testing.T) {
	t.Parallel()

	var db fixture.MemDB
	v := newEventValidator(t.Context(), global.Validator.Config, WithQueryFunc(db.SelectEvents), WithIONIdentityPublicKeys(emptyIONIdentityKeys))

	var pollSingle, pollMulti, pollExpired model.Event
	t.Run("CreatePolls", func(t *testing.T) {
		deadline := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)

		pollSingle.Kind = nostr.KindTextNote
		pollSingle.Tags = model.Tags{
			{model.CustomIONTagPoll, "type single", "ttl " + deadline, "title Test single Poll", "options [\"Option 1\", \"Option 2\"]"},
		}
		require.NoError(t, pollSingle.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, v.Validate(t.Context(), model.Events{&pollSingle}))

		pollMulti.Kind = nostr.KindTextNote
		pollMulti.Tags = model.Tags{
			{model.CustomIONTagPoll, "type multi", "ttl " + deadline, "title Test multi Poll", "options [\"Option 1\", \"Option 2\"]"},
		}
		require.NoError(t, pollMulti.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, v.Validate(t.Context(), model.Events{&pollMulti}))

		pollExpired.Kind = nostr.KindTextNote
		pollExpired.Tags = model.Tags{
			{model.CustomIONTagPoll, "type single", "ttl 1", "title Test single Poll", "options [\"Option 1\", \"Option 2\"]"},
		}
		require.NoError(t, pollExpired.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, v.Validate(t.Context(), model.Events{&pollExpired})) // pollExpired is not valid.

		require.NoError(t, db.AcceptEvents(t.Context(), &pollSingle, &pollMulti, &pollExpired))
	})

	tests := []struct {
		name    string
		event   *model.Event
		wantErr bool
	}{
		{
			name: "valid single poll vote",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollSingle.ID}},
					Content: "[0]",
				},
			},
		},
		{
			name: "valid multi poll vote",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollMulti.ID}},
					Content: "[0, 1]",
				},
			},
		},
		{
			name: "invalid poll vote with expired poll",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollExpired.ID}},
					Content: "[0]",
				},
			},
			wantErr: true,
		},
		{
			name: "vote for non-existing poll",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", "non-existing-poll-id"}},
					Content: "[0]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with no e tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Content: "[0]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with multiple e tags",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollSingle.ID}, {"e", pollMulti.ID}},
					Content: "[0]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with invalid option index",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollSingle.ID}},
					Content: "[2]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with duplicate options",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollMulti.ID}},
					Content: "[0, 0]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with two options for single poll",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollSingle.ID}},
					Content: "[0, 0]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with empty options",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollSingle.ID}},
					Content: "[]",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll vote with invalid JSON content",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    model.CustomIONKindPollVote,
					Tags:    model.Tags{{"e", pollSingle.ID}},
					Content: "[invalid]",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.event.CreatedAt = nostr.Now()
			require.NoError(t, tt.event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

			err := v.Validate(t.Context(), model.Events{tt.event})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
