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
