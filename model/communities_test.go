// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestGetCommunityRoleByPubkey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pubkey   string
		defEvent Event
		wantRole Role
	}{
		{
			name:     "no author",
			pubkey:   "foo",
			wantRole: RegularRole,
		},
		{
			name:   "author",
			pubkey: "foo",
			defEvent: Event{
				Event: nostr.Event{
					PubKey: "foo",
				},
			},
			wantRole: OwnerRole,
		},
		{
			name:   string(ModeratorRole),
			pubkey: "foo",
			defEvent: Event{
				Event: nostr.Event{
					PubKey: "bar",
					Tags:   Tags{{"p", "foo", "", string(ModeratorRole)}},
				},
			},
			wantRole: ModeratorRole,
		},
		{
			name:   "admin",
			pubkey: "foo",
			defEvent: Event{
				Event: nostr.Event{
					PubKey: "bar",
					Tags:   Tags{{"p", "foo", "", string(AdminRole)}},
				},
			},
			wantRole: AdminRole,
		},
		{
			name:   "author with b tag",
			pubkey: "foo1",
			defEvent: Event{
				Event: nostr.Event{
					PubKey: "foo",
					Tags:   Tags{{"b", "foo1"}},
				},
			},
			wantRole: OwnerRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := GetCommunityRoleByPubkey(tt.pubkey, &tt.defEvent)

			if role != Role(tt.wantRole) {
				t.Errorf("getCommunityRoleByPubkey() = %q, want %q", role, tt.wantRole)
			}
		})
	}
}
