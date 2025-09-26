// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func helperNewEventIterator(t *testing.T, filters model.Filters, data model.Events) func(func(*model.Event, error) bool) {
	t.Helper()

	return func(yield func(*model.Event, error) bool) {
		for i := range data {
			if !filters.Match(&data[i].Event) {
				continue
			}
			if !yield(data[i], nil) {
				return
			}
		}
	}
}

func TestValidateKindProfileBadgesEventProofOfOwnership(t *testing.T) {
	t.Parallel()

	userPrivate, userPublic := model.GenerateKeyPair()
	serviceKey := model.GeneratePrivateKey()

	var badgeDef model.Event
	badgeDef.Kind = nostr.KindBadgeDefinition
	badgeDef.CreatedAt = nostr.Now()
	badgeDef.Tags = model.Tags{
		{"d", "username_proof_of_ownership~testuser"},
		{"p", userPublic},
	}
	require.NoError(t, badgeDef.SignWithAlg(serviceKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var badgeAward model.Event
	badgeAward.Kind = nostr.KindBadgeAward
	badgeAward.CreatedAt = nostr.Now()
	badgeAward.Tags = model.Tags{
		{"a", badgeDef.Address()},
		{"p", userPublic},
	}
	require.NoError(t, badgeAward.SignWithAlg(serviceKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	validator := newEventValidator(t.Context(), global.Validator.Config,
		WithIONIdentityPublicKeys(emptyIONIdentityKeys),
		WithQueryFunc(func(ctx context.Context, f ...model.Filter) query.EventIterator {
			return helperNewEventIterator(t, model.Filters(f), model.Events{&badgeDef, &badgeAward})
		}),
	)
	rules := new(ruleSet).Configure(
		RuleWithSkipProfileMetadataProofEventsVerify(),
	)

	var cases = []struct {
		Name    string
		Event   *model.Event
		WantErr bool
	}{
		{
			Name: "profile with single proof badge",
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindProfileBadges,
					Tags: model.Tags{
						{"a", badgeDef.Address()},
						{"e", badgeAward.ID},
						{"d", model.ProfileBadgesIdentifier},
					},
				},
			},
		},
		{
			Name:    "profile with single two proof badges",
			WantErr: true,
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindProfileBadges,
					Tags: model.Tags{
						{"a", "30009:ABCDEF:username_proof_of_ownership~testuser"},
						{"a", "30009:ABCDEF:username_proof_of_ownership~testuser2"},
						{"e", "some_event_id"},
						{"e", "some_event_id2"},
						{"d", model.ProfileBadgesIdentifier},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			tc.Event.CreatedAt = nostr.Now()
			require.NoError(t, tc.Event.SignWithAlg(userPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := validator.validate(t.Context(), rules, model.Events{}, tc.Event)
			if tc.WantErr {
				t.Logf("got expected error: %v", err)
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
