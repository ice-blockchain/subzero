// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateKindProfileMetadataEvent(t *testing.T) {
	t.Parallel()
	privKey, _ := model.GenerateKeyPair()

	tests := []struct {
		name        string
		profileData model.ProfileMetadataContent
		shouldError bool
		errorText   string
	}{
		{
			name: "valid profile metadata with collections",
			profileData: model.ProfileMetadataContent{
				Name:        "testuser",
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"collection1": {
						Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
					"collection2": {
						Address:   "0:6753FD4A022F03A17C17D51B170084B9B3B3F45748761F252A113C6FC9752F85",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "empty collection name",
			profileData: model.ProfileMetadataContent{
				Name:        "testuser",
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"": {
						Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			},
			shouldError: true,
			errorText:   "collection name cannot be empty",
		},
		{
			name: "empty address",
			profileData: model.ProfileMetadataContent{
				Name:        "testuser",
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"collection1": {
						Address:   "",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			},
			shouldError: true,
			errorText:   "collection address cannot be empty",
		},
		{
			name: "empty created_by",
			profileData: model.ProfileMetadataContent{
				Name:        "testuser",
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"collection1": {
						Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
						CreatedBy: "",
					},
				},
			},
			shouldError: true,
			errorText:   "created_by cannot be empty",
		},
		{
			name: "valid collections with any address format",
			profileData: model.ProfileMetadataContent{
				Name:        "testuser",
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"collection1": {
						Address:   "some_address_format",
						CreatedBy: "some_creator_address",
					},
					"collection2": {
						Address:   "another:address:format",
						CreatedBy: "another_creator",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "nil collections (should pass)",
			profileData: model.ProfileMetadataContent{
				Name:                     "testuser",
				DisplayName:              "Test User",
				IONContentNFTCollections: nil,
			},
			shouldError: false,
		},
		{
			name: "empty collections map (should pass)",
			profileData: model.ProfileMetadataContent{
				Name:                     "testuser",
				DisplayName:              "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{},
			},
			shouldError: false,
		},
		{
			name: "missing name field",
			profileData: model.ProfileMetadataContent{
				DisplayName: "Test User",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"collection1": {
						Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			},
			shouldError: true,
			errorText:   "required content fields",
		},
		{
			name: "missing display_name field",
			profileData: model.ProfileMetadataContent{
				Name: "testuser",
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"collection1": {
						Address:   "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
						CreatedBy: "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			},
			shouldError: true,
			errorText:   "required content fields",
		},
		{
			name: "valid profile with all optional fields",
			profileData: model.ProfileMetadataContent{
				Name:        "fulluser",
				DisplayName: "Full User Profile",
				About:       "This is a test user profile",
				Picture:     "https://example.com/avatar.jpg",
				Website:     "https://example.com",
				Banner:      "https://example.com/banner.jpg",
				Location:    "Test City, Test Country",
				Category:    "Technology",
				Bot:         false,
				IONContentNFTCollections: map[model.IONContentNFTCollectionName]model.IONContentNFTCollectionMetadata{
					"test_collection": {
						Address:   "0:TEST123456789ABCDEF",
						CreatedBy: "0:CREATOR123456789ABCDEF",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "profile with wallets",
			profileData: model.ProfileMetadataContent{
				Name:        "walletuser",
				DisplayName: "Wallet User",
				Wallets: map[string]string{
					"bitcoin":  "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
					"ethereum": "0x1234567890123456789012345678901234567890",
				},
			},
			shouldError: false,
		},
		{
			name: "empty name and display_name",
			profileData: model.ProfileMetadataContent{
				About:   "Profile without name",
				Picture: "https://example.com/pic.jpg",
			},
			shouldError: true,
			errorText:   "required content fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentBytes, err := json.Marshal(tt.profileData)
			require.NoError(t, err)
			contentStr := string(contentBytes)

			event := &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindProfileMetadata,
					Content: contentStr,
				},
			}
			require.NoError(t, event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			validator := &eventValidator{
				SkipKindProfileProofEventsVerify: true,
			}
			err = validator.validateKindProfileMetadataEvent(t.Context(), event, nil)
			if tt.shouldError {
				require.Error(t, err, "Expected error for test case: %s", tt.name)
			} else {
				require.NoError(t, err, "Expected no error for test case: %s", tt.name)
			}
		})
	}
}
