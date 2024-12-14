// SPDX-License-Identifier: ice License 1.0

package broadcast

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type nativeDatabaseAdapter struct{}

func (*nativeDatabaseAdapter) ReadRelays(ctx context.Context, userPubkey string) (relays []string, err error) {
	it := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				Kinds:   []int{nostr.KindRelayListMetadata, nostr.KindDMRelayList},
				Authors: []string{userPubkey},
			},
		},
	})

	for event, err := range it {
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read relays for user %s", userPubkey)
		}
		for _, tag := range event.Tags {
			if tag.Key() != "r" {
				continue
			}
			if len(tag) > 2 && strings.EqualFold(tag[2], "read") {
				// Skip `read` relays.
				continue
			}
			relays = append(relays, tag.Value())
		}
	}
	return relays, nil
}
