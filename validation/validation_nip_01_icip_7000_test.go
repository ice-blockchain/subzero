// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidateRootContentNFTCollections(t *testing.T) {
	t.Parallel()

	privKey, _ := model.GenerateKeyPair()
	masterPrivKey, masterPubkey := model.GenerateKeyPair()

	tests := []struct {
		name               string
		event              *model.Event
		profileMetadata    *model.Event
		profileMetadataErr error
		config             *Config
		shouldError        bool
		expectedErrorType  error
	}{
		{
			name:  "valid article with NFT collections",
			event: helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":                        "testuser",
				"display_name":                "Test User",
				"ion_content_nft_collections": helperCreateTestNFTCollections(t),
			}),
			config:      nil,
			shouldError: false,
		},
		{
			name:  "valid article with allowed NFT collection",
			event: helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":                        "testuser",
				"display_name":                "Test User",
				"ion_content_nft_collections": helperCreateTestNFTCollections(t),
			}),
			config: &Config{
				AllowedNFTCollections: []string{"My NFT Collection", "Other Collection"},
			},
			shouldError: false,
		},
		{
			name:  "article with non-allowed NFT collection should fail",
			event: helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":                        "testuser",
				"display_name":                "Test User",
				"ion_content_nft_collections": helperCreateTestNFTCollections(t),
			}),
			config: &Config{
				AllowedNFTCollections: []string{"Allowed Collection", "Other Allowed Collection"},
			},
			shouldError: true,
		},
		{
			name:  "valid editable text note with allowed NFT collection",
			event: helperCreateEditableTextNoteEventForTest(t, privKey, masterPubkey, "Editable content", "my-note"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":         "testuser",
				"display_name": "Test User",
				"ion_content_nft_collections": map[string]interface{}{
					"Another Collection": map[string]interface{}{
						"address":    "0:6753FD4A022F03A17C17D51B170084B9B3B3F45748761F252A113C6FC9752F85",
						"created_by": "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			}),
			config: &Config{
				AllowedNFTCollections: []string{"My NFT Collection", "Another Collection"},
			},
			shouldError: false,
		},
		{
			name:  "article without NFT collections should fail",
			event: helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":         "testuser",
				"display_name": "Test User",
			}),
			config:      nil,
			shouldError: true,
		},
		{
			name:  "editable text note without NFT collections should fail",
			event: helperCreateEditableTextNoteEventForTest(t, privKey, masterPubkey, "Editable content", "my-note"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":         "testuser",
				"display_name": "Test User",
			}),
			config:      nil,
			shouldError: true,
		},
		{
			name:  "article with empty NFT collections should fail",
			event: helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":                        "testuser",
				"display_name":                "Test User",
				"ion_content_nft_collections": map[string]interface{}{},
			}),
			config:      nil,
			shouldError: true,
		},
		{
			name:            "text note should pass without NFT collections check",
			event:           helperCreateTextNoteEventForTest(t, privKey, masterPubkey, "Just a text note", nil),
			profileMetadata: nil,
			config:          nil,
			shouldError:     false,
		},
		{
			name:            "no profile metadata should fail",
			event:           helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: nil,
			config:          nil,
			shouldError:     true,
		},
		{
			name:  "invalid NFT collections structure should fail",
			event: helperCreateArticleEventForTest(t, privKey, masterPubkey, "Test Article", "Article content"),
			profileMetadata: helperCreateProfileMetadataWithNFTCollections(t, masterPrivKey, masterPubkey, map[string]interface{}{
				"name":         "testuser",
				"display_name": "Test User",
				"ion_content_nft_collections": map[string]interface{}{
					"": map[string]interface{}{
						"address":    "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
						"created_by": "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
					},
				},
			}),
			config:      nil,
			shouldError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &eventValidator{
				Config:    tt.config,
				QueryFunc: createMockQueryFunc(tt.profileMetadata, tt.profileMetadataErr),
			}

			err := validator.validateRootContentNFTCollections(t.Context(), tt.event)

			if tt.shouldError {
				require.Error(t, err, "Expected error for test case: %s", tt.name)
				if tt.expectedErrorType != nil {
					require.ErrorIs(t, err, tt.expectedErrorType, "Expected specific error type for test case: %s", tt.name)
				}
			} else {
				require.NoError(t, err, "Expected no error for test case: %s", tt.name)
			}
		})
	}
}

func helperCreateArticleEventForTest(t *testing.T, privKey, masterPubkey, title, content string) *model.Event {
	t.Helper()
	event := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindArticle,
			Content: content,
			Tags: model.Tags{
				{"b", masterPubkey},
				{"d", "article-" + title},
				{"title", title},
			},
		},
	}
	require.NoError(t, event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return event
}

func helperCreateEditableTextNoteEventForTest(t *testing.T, privKey, masterPubkey, content, dTag string) *model.Event {
	t.Helper()
	event := &model.Event{
		Event: nostr.Event{
			Kind:    model.CustomIONKindEditableTextNote,
			Content: content,
			Tags: model.Tags{
				{"b", masterPubkey},
				{"d", dTag},
				{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
			},
		},
	}
	require.NoError(t, event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return event
}

func helperCreateTextNoteEventForTest(t *testing.T, privKey, masterPubkey, content string, additionalTags []string) *model.Event {
	t.Helper()
	tags := model.Tags{{"b", masterPubkey}}
	for _, tag := range additionalTags {
		parts := strings.Split(tag, " ")
		tags = append(tags, parts)
	}

	event := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindTextNote,
			Content: content,
			Tags:    tags,
		},
	}
	require.NoError(t, event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	return event
}

func helperCreateProfileMetadataWithNFTCollections(t *testing.T, privKey, masterPubkey string, content map[string]interface{}) *model.Event {
	t.Helper()
	contentJSON, err := json.Marshal(content)
	require.NoError(t, err)

	event := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindProfileMetadata,
			Content: string(contentJSON),
			Tags:    model.Tags{{"b", masterPubkey}},
		},
	}
	require.NoError(t, event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return event
}

func helperCreateTestNFTCollections(t *testing.T) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		"My NFT Collection": map[string]interface{}{
			"address":    "0:3091ABF860DBB033A1EBCDD12AB689C6FF3F9752C151563FEFFF8B508A888290",
			"created_by": "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
		},
		"Another Collection": map[string]interface{}{
			"address":    "0:6753FD4A022F03A17C17D51B170084B9B3B3F45748761F252A113C6FC9752F85",
			"created_by": "0:1825C553BC67ED4DAFFE789C921FFEC7E3005EF88CE3B58F4E5A73AF6DCD08D4",
		},
	}
}

func createMockQueryFunc(profileMetadata *model.Event, queryError error) func(context.Context, ...model.Filter) query.EventIterator {
	return func(ctx context.Context, filters ...model.Filter) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			if queryError != nil {
				yield(nil, queryError)
				return
			}
			for _, filter := range filters {
				for _, kind := range filter.Kinds {
					if kind == nostr.KindProfileMetadata && profileMetadata != nil {
						yield(profileMetadata, nil)
						return
					}
				}
			}
		}
	}
}
