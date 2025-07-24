// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateIONContentNFTCollections(t *testing.T) {
	t.Parallel()
	privKey, _ := model.GenerateKeyPair()
	tests := []struct {
		name        string
		collections map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata
		shouldError bool
		errorText   string
	}{
		{
			name: "valid collections",
			collections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
				"collection1": {
					Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
					CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
				},
				"collection2": {
					Address:   "0:6753FD4A022F03A17C17D51B170084B9B3B3F45748761F252A113C6FC9752F85",
					CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
				},
			},
			shouldError: false,
		},
		{
			name: "empty collection name",
			collections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
				"": {
					Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
					CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
				},
			},
			shouldError: true,
			errorText:   "collection name cannot be empty",
		},
		{
			name: "empty address",
			collections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
				"collection1": {
					Address:   "",
					CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
				},
			},
			shouldError: true,
			errorText:   "collection address cannot be empty",
		},
		{
			name: "empty created_by",
			collections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
				"collection1": {
					Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
					CreatedBy: "",
				},
			},
			shouldError: true,
			errorText:   "created_by cannot be empty",
		},
		{
			name: "valid collections with any address format",
			collections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
				"collection1": {
					Address:   "some_address_format",
					CreatedBy: "some_creator_address",
				},
				"collection2": {
					Address:   "another:address:format",
					CreatedBy: "another_creator",
				},
			},
			shouldError: false,
		},
		{
			name:        "nil collections (should pass)",
			collections: nil,
			shouldError: false,
		},
		{
			name:        "empty collections map (should pass)",
			collections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindProfileMetadata,
					Content: "test content",
				},
			}
			require.NoError(t, event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := validateIONContentNFTCollections(tt.collections, event)
			if tt.shouldError {
				require.Error(t, err, "Expected error for test case: %s", tt.name)
			} else {
				require.NoError(t, err, "Expected no error for test case: %s", tt.name)
			}
		})
	}
}
