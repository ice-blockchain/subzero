// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"fmt"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateSettingsTag(t *testing.T) {
	t.Parallel()
	ev := &model.Event{
		Event: nostr.Event{
			Tags: model.Tags{},
		},
	}

	tests := []struct {
		name    string
		event   *model.Event
		tag     model.Tag
		wantNil bool
	}{
		{
			name:    "no timestamp at the settings",
			event:   ev,
			tag:     nostr.Tag{"settings", "foo", "0"},
			wantNil: false,
		},
		{
			name:    "invalid settings length",
			event:   ev,
			tag:     nostr.Tag{"settings", "foo"},
			wantNil: false,
		},
		{
			name: "invalid settings value for comments_enabled",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "comments_enabled", "bar", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "valid settings value for comments_enabled",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "comments_enabled", "true", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for comments_enabled",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "wrong kind for comments_enabled settings",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "invalid settings value for role_required_for_posting",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "role_required_for_posting", "admin", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "invalid settings value for role_required_for_posting",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "role_required_for_posting", "moderator", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "wrong kind for role_required_for_posting settings",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "role_required_for_posting", "admin", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "invalid settings value for role_required_for_posting",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "role_required_for_posting", "dummy", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "following,mentioned,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindArticle,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "following", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "mentioned", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "following,mentioned", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "mentioned,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			wantNil: true,
		},
		{
			name: "valid settings value for who_can_reply",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "dummy,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "wrong kind for who_can_reply settings",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindRepost,
					Tags: model.Tags{},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "following,mentioned,badge|30009:alice:bravery", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "reply event with e tag cannot set settings",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					Tags: model.Tags{
						{"e", "some_event_id", "", model.TagMarkerReply},
					},
				},
			},
			tag:     nostr.Tag{"settings", "who_can_reply", "following", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
		{
			name: "reply event with a tag cannot set settings",
			event: &model.Event{
				Event: nostr.Event{
					Kind: model.CustomIONKindCommunityDefinition,
					Tags: model.Tags{
						{"a", "30023:pubkey:dtag", "", model.TagMarkerReply},
					},
				},
			},
			tag:     nostr.Tag{"settings", "comments_enabled", "true", fmt.Sprint(time.Now().Unix())},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateSettingsTag(tt.event, tt.tag)
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
		wantTag     model.Tag
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
			wantTag: model.Tag{"settings", "foo", "1", fmt.Sprint(now.Add(-1 * time.Minute).Unix())},
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
