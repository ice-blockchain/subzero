// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTranslation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	testData := `{
		"en": {
			"title": "Test title",
			"body": "Test body {{name}}"
		},
		"fr": {
			"title": "Titre de test",
			"body": "Corps de test {{name}}"
		}
	}`

	err := os.WriteFile(filepath.Join(tempDir, "post.json"), []byte(testData), 0644)
	require.NoError(t, err, "No error should occur when creating translation file")

	tm := NewTranslationManager(tempDir)

	title := tm.GetTranslation(NotificationTypePost, "fr", "title", nil)
	require.Equal(t, "Titre de test", title, "Incorrect title translation in French")

	body := tm.GetTranslation(NotificationTypePost, "fr", "body", map[string]interface{}{
		"name": "John",
	})
	require.Equal(t, "Corps de test John", body, "Incorrect body translation with placeholder in French")

	titleEn := tm.GetTranslation(NotificationTypePost, "en", "title", nil)
	require.Equal(t, "Test title", titleEn, "Incorrect title translation in English")

	nonExistentTitle := tm.GetTranslation(NotificationType("non_existent_type"), "fr", "title", nil)
	require.Equal(t, "fr_title", nonExistentTitle, "For non-existent type, format language_key should be returned")
}

func TestLoadTranslations(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	postData := `{
		"fr": {
			"title": "Titre de test du post",
			"body": "Corps de test du post {{name}}"
		},
		"en": {
			"title": "Test post title",
			"body": "Test post body {{name}}"
		}
	}`

	reactionData := `{
		"fr": {
			"title": "Nouvelle réaction",
			"body": "{{user}} a réagi à votre post"
		},
		"en": {
			"title": "New reaction",
			"body": "{{user}} reacted to your post"
		}
	}`

	err := os.WriteFile(filepath.Join(tempDir, "post.json"), []byte(postData), 0644)
	require.NoError(t, err, "No error should occur when creating post translation file")

	err = os.WriteFile(filepath.Join(tempDir, "reaction.json"), []byte(reactionData), 0644)
	require.NoError(t, err, "No error should occur when creating reaction translation file")

	tm := &TranslationManager{
		translations: make(map[NotificationType]map[Language]map[string]string),
	}

	err = tm.loadTranslations(tempDir)
	require.NoError(t, err, "No error should occur when loading translations")

	require.Equal(t, "Titre de test du post", tm.translations[NotificationTypePost]["fr"]["title"], "Post title translation in French should be loaded correctly")
	require.Equal(t, "Corps de test du post {{name}}", tm.translations[NotificationTypePost]["fr"]["body"], "Post body translation in French should be loaded correctly")
	require.Equal(t, "Test post title", tm.translations[NotificationTypePost]["en"]["title"], "Post title translation in English should be loaded correctly")
	require.Equal(t, "Test post body {{name}}", tm.translations[NotificationTypePost]["en"]["body"], "Post body translation in English should be loaded correctly")

	require.Equal(t, "Nouvelle réaction", tm.translations[NotificationTypeReaction]["fr"]["title"], "Reaction title translation in French should be loaded correctly")
	require.Equal(t, "{{user}} a réagi à votre post", tm.translations[NotificationTypeReaction]["fr"]["body"], "Reaction body translation in French should be loaded correctly")
	require.Equal(t, "New reaction", tm.translations[NotificationTypeReaction]["en"]["title"], "Reaction title translation in English should be loaded correctly")
	require.Equal(t, "{{user}} reacted to your post", tm.translations[NotificationTypeReaction]["en"]["body"], "Reaction body translation in English should be loaded correctly")
}

func TestNewTranslationManager(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	testData := `{
		"fr": {
			"title": "Titre de test",
			"body": "Corps de test {{name}}"
		},
		"en": {
			"title": "Test title",
			"body": "Test body {{name}}"
		}
	}`

	err := os.WriteFile(filepath.Join(tempDir, "post.json"), []byte(testData), 0644)
	require.NoError(t, err, "No error should occur when creating translation file")

	tm := NewTranslationManager(tempDir)

	require.NotNil(t, tm, "Translation manager should be created")

	title := tm.GetTranslation(NotificationTypePost, "fr", "title", nil)
	require.Equal(t, "Titre de test", title, "Title translation in French should be loaded correctly")

	titleEn := tm.GetTranslation(NotificationTypePost, "en", "title", nil)
	require.Equal(t, "Test title", titleEn, "Title translation in English should be loaded correctly")
}

func TestGetAvailableLanguages(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	postData := `{
		"fr": {
			"title": "Titre de test du post",
			"body": "Corps de test du post {{name}}"
		},
		"en": {
			"title": "Test post title",
			"body": "Test post body {{name}}"
		}
	}`

	reactionData := `{
		"fr": {
			"title": "Nouvelle réaction",
			"body": "{{user}} a réagi à votre message"
		},
		"de": {
			"title": "Neue Reaktion",
			"body": "{{user}} hat auf deinen Beitrag reagiert"
		}
	}`

	err := os.WriteFile(filepath.Join(tempDir, "post.json"), []byte(postData), 0644)
	require.NoError(t, err, "No error should occur when creating post translation file")

	err = os.WriteFile(filepath.Join(tempDir, "reaction.json"), []byte(reactionData), 0644)
	require.NoError(t, err, "No error should occur when creating reaction translation file")

	tm := NewTranslationManager(tempDir)

	languages := tm.GetAvailableLanguages()

	require.Len(t, languages, 3, "There should be 3 languages")
	require.Contains(t, languages, Language("fr"), "Should contain French language")
	require.Contains(t, languages, Language("en"), "Should contain English language")
	require.Contains(t, languages, Language("de"), "Should contain German language")
}

func TestFallbackToEnglish(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	testData := `{
		"en": {
			"title": "English Title",
			"body": "English Body"
		},
		"fr": {
			"title": "Titre Français",
			"body": "Corps Français"
		}
	}`

	err := os.WriteFile(filepath.Join(tempDir, "post.json"), []byte(testData), 0644)
	require.NoError(t, err, "No error should occur when creating translation file")

	tm := NewTranslationManager(tempDir)

	title := tm.GetTranslation(NotificationTypePost, "de", "title", nil)
	require.Equal(t, "English Title", title, "If language is not found, English should be used")

	nonExistentKey := tm.GetTranslation(NotificationTypePost, "fr", "non_existent", nil)
	require.Equal(t, "fr_non_existent", nonExistentKey, "If key is not found, language_key should be returned")
}

func TestPlaceholderReplacement(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	testData := `{
		"en": {
			"title": "Hello {{name}}",
			"body": "Welcome to {{service}}, {{name}}! Your ID: {{id}}"
		}
	}`

	err := os.WriteFile(filepath.Join(tempDir, "post.json"), []byte(testData), 0644)
	require.NoError(t, err, "No error should occur when creating translation file")

	tm := NewTranslationManager(tempDir)

	title := tm.GetTranslation(NotificationTypePost, "en", "title", map[string]interface{}{
		"name": "John",
	})
	require.Equal(t, "Hello John", title, "Simple placeholder should be replaced")

	body := tm.GetTranslation(NotificationTypePost, "en", "body", map[string]interface{}{
		"name":    "Alice",
		"service": "Nostr",
		"id":      12345,
	})
	require.Equal(t, "Welcome to Nostr, Alice! Your ID: 12345", body, "Multiple placeholders should be replaced")

	body = tm.GetTranslation(NotificationTypePost, "en", "body", map[string]interface{}{
		"name":    "Alice",
		"service": nil,
		"id":      12345,
	})
	require.Equal(t, "Welcome to {{service}}, Alice! Your ID: 12345", body, "Nil placeholders should not be replaced")
}
