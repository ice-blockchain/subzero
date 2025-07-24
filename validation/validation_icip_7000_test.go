// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestPostWithRichTextOnly(t *testing.T) {
	t.Parallel()
	privKey, _ := model.GenerateKeyPair()

	var ev model.Event
	ev.Kind = model.CustomIONKindEditableTextNote
	ev.CreatedAt = nostr.Now()
	ev.Tags = model.Tags{
		{model.CustomIONTagRichText, "foo"},
		{"d", "foo"},
		{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
	}
	require.NoError(t, ev.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	validator := newEventValidator(&Config{
		AllowedNFTCollections: []string{"Test Collection"},
	}, WithQueryFunc(func(ctx context.Context, filters ...model.Filter) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			profileContent := model.ProfileMetadataContent{
				Name:        "testuser",
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"Test Collection": {
						Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			}
			profileContentJSON, err := json.Marshal(profileContent)
			require.NoError(t, err)
			profileEvent := &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindProfileMetadata,
					Content: string(profileContentJSON),
				},
			}
			require.NoError(t, profileEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			yield(profileEvent, nil)
		}
	}))

	require.NoError(t, validator.Validate(t.Context(), &ev))
}
