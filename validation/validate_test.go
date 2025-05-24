// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	addr, release := query.NewTestDatabase(ctx)
	query.MustInit(ctx, query.WithConfig(&query.Config{
		URL: addr,
	}))
	MustInit()

	code := m.Run()
	cancel()
	release()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Printf("goleak found issues: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestValidateGiftWrap(t *testing.T) {
	t.Parallel()

	key := model.GeneratePrivateKey()

	var ev model.Event
	ev.Kind = nostr.KindGiftWrap
	ev.CreatedAt = 1
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	ev.Tags = append(ev.Tags, model.Tag{"p", "foop"}, model.Tag{"k", "123"})
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	ev.Tags = append(ev.Tags, model.Tag{"expiration", "foo"})
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	ev.Tags = append(ev.Tags[:len(ev.Tags)-2], model.Tag{"expiration", "123"})
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))
}

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
		require.NoError(t, Validate(t.Context(), &ev))
	})
}

func TestValidateSettingsTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     model.Tag
		kind    int
		wantNil bool
	}{
		{
			name:    "no timestamp at the settings",
			tag:     nostr.Tag{"settings", "foo", "0"},
			kind:    model.CustomIONKindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "invalid settings length",
			tag:     nostr.Tag{"settings", "foo"},
			kind:    model.CustomIONKindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "invalid settings value for comments_enabled",
			tag:     nostr.Tag{"settings", "comments_enabled", "bar", fmt.Sprint(time.Now().Unix())},
			kind:    model.CustomIONKindCommunityDefinition,
			wantNil: false,
		},
		{
			name:    "valid settings value for comments_enabled",
			tag:     nostr.Tag{"settings", "comments_enabled", "true", fmt.Sprint(time.Now().Unix())},
			kind:    model.CustomIONKindCommunityDefinition,
			wantNil: true,
		},
		{
			name:    "valid settings value for comments_enabled",
			tag:     nostr.Tag{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
			kind:    model.CustomIONKindCommunityDefinition,
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
			kind:    model.CustomIONKindCommunityDefinition,
			wantNil: true,
		},
		{
			name:    "invalid settings value for role_required_for_posting",
			tag:     nostr.Tag{"settings", "role_required_for_posting", "moderator", fmt.Sprint(time.Now().Unix())},
			kind:    model.CustomIONKindCommunityDefinition,
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
			kind:    model.CustomIONKindCommunityDefinition,
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
			kind:    nostr.KindRepost,
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

func TestGetLatestSettingsTag(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name        string
		settingsTag string
		community   model.Event
		wantTag     *model.Tag
	}{
		{
			name:        "no settings tag",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{},
				},
			},
			wantTag: nil,
		},
		{
			name:        "settings tag",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "foo", "0"},
					},
				},
			},
			wantTag: nil,
		},
		{
			name:        "multiple settings tags",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "foo", "0", fmt.Sprint(now.Add(-5 * time.Minute).Unix())},
						{"settings", "foo", "1", fmt.Sprint(now.Add(-1 * time.Minute).Unix())},
						{"settings", "foo", "3", fmt.Sprint(now.Add(-3 * time.Minute).Unix())},
						{"settings", "foo", "2", fmt.Sprint(now.Add(-4 * time.Minute).Unix())},
					},
				},
			},
			wantTag: &model.Tag{"settings", "foo", "1", fmt.Sprint(now.Add(-1 * time.Minute).Unix())},
		},
		{
			name:        "wrong unix timestamps",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "foo", "0", "x"},
						{"settings", "foo", "1", "y"},
						{"settings", "foo", "3", "z"},
						{"settings", "foo", "2", "w"},
					},
				},
			},
			wantTag: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := getLatestSettingsTag(&tt.community, tt.settingsTag)

			require.EqualValues(t, tag, tt.wantTag)
		})
	}
}

func TestRoleRequiredForPosting(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name string
		def  *model.Event
		want model.Role
	}{
		{
			name: "role_required_for_posting settings is admin",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", model.RoleRequiredForPostingSettings, string(model.AdminRole), fmt.Sprint(now.Unix())}}}},
			want: model.AdminRole,
		},
		{
			name: "role_required_for_posting settings is moderator",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(now.Unix())}}}},
			want: model.ModeratorRole,
		},
		{
			name: "role_required_for_posting settings is wrong",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", model.RoleRequiredForPostingSettings, "wrong", fmt.Sprint(now.Unix())}}}},
			want: "",
		},
		{
			name: "several role_required_for_posting settings is moderator",
			def: &model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(now.Add(-1 * time.Hour).Unix())},
						{"settings", model.RoleRequiredForPostingSettings, string(model.AdminRole), fmt.Sprint(now.Unix())},
					},
				},
			},
			want: model.AdminRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roleRequiredForPosting(tt.def)

			if got != tt.want {
				t.Errorf("roleRequiredForPosting() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCommunityCommentsEnabled(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name string
		def  *model.Event
		want bool
	}{
		{
			name: "comments_enabled settings is true",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(now.Unix())}}}},
			want: true,
		},
		{
			name: "comments_enabled settings is false",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(now.Unix())}}}},
			want: false,
		},
		{
			name: "several comments_enabled settings",
			def: &model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(now.Add(-1 * time.Hour).Unix())},
						{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(now.Unix())},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCommunityCommentsEnabled(tt.def)

			if got != tt.want {
				t.Errorf("isCommunityCommentsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyChangeCommunityPatch(t *testing.T) {
	tests := []struct {
		name    string
		def     *model.Event
		patches []*model.Event
		want    model.Tags
	}{
		{
			name: "apply 1 patch",
			def: &model.Event{
				Event: nostr.Event{
					CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Hour).Unix()),
					Tags: model.Tags{
						{"name", "old name"},
						{"description", "old description"},
						{"open"},
						{"private"},
						{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
					},
				},
			},
			patches: []*model.Event{
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Minute).Unix()),
						Tags:      model.Tags{{"name", "new name"}, {"description", "new description"}, {"closed"}, {"public"}},
					},
				},
			},
			want: model.Tags{
				{"name", "new name"},
				{"description", "new description"},
				{"closed"},
				{"public"},
				{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
			},
		},
		{
			name: "apply several patches",
			def: &model.Event{
				Event: nostr.Event{
					CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Hour).Unix()),
					Tags: model.Tags{
						{"name", "old name"},
						{"description", "old description"},
						{"open"},
						{"private"},
						{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-3 * time.Minute).Unix())},
						{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Add(-2 * time.Minute).Unix())},
						{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
						{"p", "moderator1", "", string(model.ModeratorRole)},
						{"p", "moderator2", "", string(model.ModeratorRole)},
						{"p", "admin1", "", string(model.AdminRole)},
						{"p", "admin2", "", string(model.AdminRole)},
					},
				},
			},
			patches: []*model.Event{
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-4 * time.Minute).Unix()),
						Tags:      model.Tags{{"name", "new name"}},
					},
				},
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-3 * time.Minute).Unix()),
						Tags:      model.Tags{{"description", "new description"}, {"p", "moderator3", "", string(model.ModeratorRole)}},
					},
				},
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-2 * time.Minute).Unix()),
						Tags:      model.Tags{{"closed"}, {"p", "admin3", "", string(model.AdminRole)}},
					},
				},
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Minute).Unix()),
						Tags:      model.Tags{{"public"}, {"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())}},
					},
				},
			},
			want: model.Tags{
				{"name", "new name"},
				{"description", "new description"},
				{"closed"},
				{"public"},
				{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-3 * time.Minute).Unix())},
				{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Add(-2 * time.Minute).Unix())},
				{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
				{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
				{"p", "moderator1", "", string(model.ModeratorRole)},
				{"p", "moderator2", "", string(model.ModeratorRole)},
				{"p", "moderator3", "", string(model.ModeratorRole)},
				{"p", "admin1", "", string(model.AdminRole)},
				{"p", "admin2", "", string(model.AdminRole)},
				{"p", "admin3", "", string(model.AdminRole)},
			},
		},
		{
			name: "apply patches with diferent time order",
			def: &model.Event{
				Event: nostr.Event{
					CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Hour).Unix()),
					Tags: model.Tags{
						{"name", "old name"},
						{"description", "old description"},
						{"open"},
						{"private"},
						{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
					},
				},
			},
			patches: []*model.Event{
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Minute).Unix()),
						Tags:      model.Tags{{"name", "new name 1"}, {"description", "new description 1"}, {"closed"}, {"public"}},
					},
				},
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-2 * time.Minute).Unix()),
						Tags:      model.Tags{{"name", "new name 2"}, {"description", "new description 2"}, {"open"}, {"public"}},
					},
				},
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-3 * time.Minute).Unix()),
						Tags:      model.Tags{{"name", "new name 3"}, {"description", "new description 3"}, {"closed"}, {"private"}},
					},
				},
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Unix()),
						Tags:      model.Tags{{"name", "new name 4"}, {"description", "new description 4"}, {"open"}, {"public"}},
					},
				},
			},
			want: model.Tags{
				{"name", "new name 4"},
				{"description", "new description 4"},
				{"open"},
				{"public"},
				{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
			},
		},
		{
			name: "apply 1 patch with demote admin/moderator",
			def: &model.Event{
				Event: nostr.Event{
					CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Hour).Unix()),
					Tags: model.Tags{
						{"name", "old name"},
						{"description", "old description"},
						{"open"},
						{"private"},
						{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
						{"p", "moderator1", "", string(model.ModeratorRole)},
						{"p", "moderator2", "", string(model.ModeratorRole)},
						{"p", "moderator3", "", string(model.ModeratorRole)},
						{"p", "admin1", "", string(model.AdminRole)},
						{"p", "admin2", "", string(model.AdminRole)},
						{"p", "admin3", "", string(model.AdminRole)},
					},
				},
			},
			patches: []*model.Event{
				{
					Event: nostr.Event{
						CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Minute).Unix()),
						Tags:      model.Tags{{"name", "new name"}, {"description", "new description"}, {"closed"}, {"public"}, {"p", "moderator1", "", string(model.RegularRole)}, {"p", "admin1", "", string(model.ModeratorRole)}},
					},
				},
			},
			want: model.Tags{
				{"name", "new name"},
				{"description", "new description"},
				{"closed"},
				{"public"},
				{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
				{"p", "moderator1", "", string(model.RegularRole)},
				{"p", "moderator2", "", string(model.ModeratorRole)},
				{"p", "moderator3", "", string(model.ModeratorRole)},
				{"p", "admin1", "", string(model.ModeratorRole)},
				{"p", "admin2", "", string(model.AdminRole)},
				{"p", "admin3", "", string(model.AdminRole)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ElementsMatch(t, applyChangeCommunityPatch(tt.patches, tt.def), tt.want)
		})
	}
}
func TestValidatePollVote(t *testing.T) {
	t.Parallel()

	var pollSingle, pollMulti, pollExpired model.Event
	t.Run("CreatePolls", func(t *testing.T) {
		deadline := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)

		pollSingle.Kind = nostr.KindTextNote
		pollSingle.Tags = model.Tags{
			{model.CustomIONTagPoll, "type single", "ttl " + deadline, "title Test single Poll", "options [\"Option 1\", \"Option 2\"]"},
		}
		require.NoError(t, pollSingle.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &pollSingle))

		pollMulti.Kind = nostr.KindTextNote
		pollMulti.Tags = model.Tags{
			{model.CustomIONTagPoll, "type multi", "ttl " + deadline, "title Test multi Poll", "options [\"Option 1\", \"Option 2\"]"},
		}
		require.NoError(t, pollMulti.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &pollMulti))

		pollExpired.Kind = nostr.KindTextNote
		pollExpired.Tags = model.Tags{
			{model.CustomIONTagPoll, "type single", "ttl 1", "title Test single Poll", "options [\"Option 1\", \"Option 2\"]"},
		}
		require.NoError(t, pollExpired.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &pollExpired)) // pollExpired is not valid.

		require.NoError(t, query.AcceptEvents(t.Context(), &pollSingle, &pollMulti, &pollExpired))
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
			tt.event.CreatedAt = nostr.Timestamp(time.Now().Unix())
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

func TestValidateDtag(t *testing.T) {
	t.Parallel()

	// Known event kind.
	var ev model.Event
	ev.Kind = nostr.KindArticle
	require.Error(t, Validate(t.Context(), &ev))

	// Unknown event kind, but addressable.
	ev.Kind = nostr.KindLiveEvent
	require.Error(t, Validate(t.Context(), &ev))
	ev.Tags = append(ev.Tags, model.Tag{"d", "foo"})
	require.NoError(t, Validate(t.Context(), &ev))
}
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
			err := Validate(t.Context(), tt.event)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

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
					Kind: nostr.KindReaction,
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
					Kind: nostr.KindReaction,
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
					Kind: nostr.KindReaction,
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
					Kind: nostr.KindReaction,
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
					Kind: nostr.KindReaction,
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
			err := Validate(t.Context(), tt.event)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateOneOfSingle(t *testing.T) {
	t.Parallel()

	validator := newKindValidatorBuilderEmpty().OneOfSingle("e", "a").Build()
	require.NotNil(t, validator)

	rules := map[model.Kind]kindValidator{
		nostr.KindTextNote: validator,
	}

	var cases = []struct {
		Err   bool
		Event *model.Event
	}{
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"p", "test"}}}}},
		{false, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}}}}},
		{false, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"a", "1:aa:aa"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}, {"a", "1:aa:aa"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}, {"e", "test2"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"a", "1:aa:aa2"}, {"a", "1:aa:aa2"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}, {"e", "test2"}, {"a", "1:aa:aa"}}}}},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			err := validateEventTags(c.Event, rules)
			if c.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

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
func TestValidateKindGiftWrapEvent(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Event *model.Event
		Err   bool
	}{
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
					},
				},
			},
			Err: true, // Missing expiration tag.
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
						{"expiration", "foo"}, // Invalid expiration value.
					},
				},
			},
			Err: true,
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
						{"expiration", "9223372036"}, // Too far in the future.
					},
				},
			},
			Err: true,
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", strconv.Itoa(model.CustomIONKindFundReceive)},
						{"expiration", strconv.Itoa(int(time.Now().Add(24 * time.Hour).Unix()))},
					},
				},
			},
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
						{"expiration", strconv.Itoa(int(time.Now().Add(24 * time.Hour).Unix()))},
					},
				},
			},
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", strconv.Itoa(model.CustomIONKindFundReceive)}, // Expiration tag is not required for FundReceive.
					},
				},
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			require.NoError(t, c.Event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := Validate(t.Context(), c.Event)
			if c.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateArticleSoftDelete(t *testing.T) {
	t.Parallel()

	nowUnix := time.Now().Unix()

	tests := []struct {
		name    string
		event   *model.Event
		wantErr bool
	}{
		{
			name: "valid article soft delete",
			event: &model.Event{
				Event: nostr.Event{
					Kind:      nostr.KindArticle,
					CreatedAt: nostr.Timestamp(nowUnix),
					Content:   "",
					Tags: model.Tags{
						{"d", "test"},
						{"published_at", strconv.FormatInt(nowUnix+1, 10)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid editable text note soft delete",
			event: &model.Event{
				Event: nostr.Event{
					Kind:      model.CustomIONKindEditableTextNote,
					CreatedAt: nostr.Timestamp(nowUnix),
					Content:   "",
					Tags: model.Tags{
						{"d", "test"},
						{"published_at", strconv.FormatInt(nowUnix+2, 10)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid editable text note",
			event: &model.Event{
				Event: nostr.Event{
					Kind:      model.CustomIONKindEditableTextNote,
					CreatedAt: nostr.Timestamp(nowUnix),
					Content:   "",
					Tags: model.Tags{
						{"d", "test"},
						{"published_at", strconv.FormatInt(nowUnix, 10)},
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

func TestValidateTagsBAndP(t *testing.T) {
	t.Parallel()

	require.Error(t, validateFollowListEvent(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"p", "foo"},
	}}}))

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"b", "foo"},
	}}}, KindSupportedTags))

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"p", "foo"},
		{"p", "foo"},
	}}}, KindSupportedTags))

	require.Error(t, validateFollowListEvent(&model.Event{Event: nostr.Event{PubKey: "foo", Tags: model.Tags{
		{"p", "foo"},
	}}}))

	require.NoError(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"p", "bar"},
	}}}, KindSupportedTags))
}

func TestValidateFundSend(t *testing.T) {
	t.Parallel()

	t.Run("With p", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindFundSendNotify
		ev.Tags = model.Tags{
			{"b", "bar"},
			{"network", "ion"},
			{"asset_class", "native"},
			{"asset_address", "localhost"},
		}
		require.Error(t, Validate(t.Context(), &ev))

		ev.Tags = append(ev.Tags, model.Tag{"p", "foo"})
		require.Error(t, Validate(t.Context(), &ev))

		ev.Content = "foo"
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("With l+L", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindFundSendNotify
		ev.Tags = model.Tags{
			{"b", "bar"},
			{"l", "1234", "wallet.address"},
			{"network", "ion"},
			{"asset_class", "native"},
			{"asset_address", "localhost"},
		}
		ev.Content = `{"to":"1234"}`
		require.Error(t, Validate(t.Context(), &ev))

		ev.Tags = append(ev.Tags, model.Tag{"L", "wallet.address"})
		require.NoError(t, Validate(t.Context(), &ev))

		ev.Tags = append(ev.Tags, model.Tag{"p", "bar"})
		require.Error(t, Validate(t.Context(), &ev))
	})
}

func TestMultipleTagsP(t *testing.T) {
	t.Parallel()

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{
		Kind: nostr.KindTextNote,
		Tags: model.Tags{
			{"p", "foo"},
			{"p", "foo"},
		}}}, KindSupportedTags))

	require.NoError(t, validateEventTags(&model.Event{Event: nostr.Event{
		Kind: func() int {
			for k := range kindAllowMultipleTagsP {
				return k
			}
			panic("unreachable")
		}(),
		Tags: model.Tags{
			{"p", "foo"},
			{"p", "foo"},
		}}}, KindSupportedTags))
}

func TestPostWithRichTextOnly(t *testing.T) {
	t.Parallel()

	var ev model.Event
	ev.Kind = model.CustomIONKindEditableTextNote
	ev.CreatedAt = nostr.Now()
	ev.Tags = model.Tags{
		{model.CustomIONTagRichText, "foo"},
		{"d", "foo"},
		{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
	}

	ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519)
	require.NoError(t, Validate(t.Context(), &ev))
}
