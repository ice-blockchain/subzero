// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSqlFts5CleanupText(t *testing.T) {
	t.Parallel()

	t.Run("Usual", func(t *testing.T) {
		require.Equal(t, "", sqlFts5CleanupText(""))
		require.Equal(t, "a", sqlFts5CleanupText("a"))
		require.Equal(t, "a b", sqlFts5CleanupText("a b"))
	})
	t.Run("Multiple spacs", func(t *testing.T) {
		require.Equal(t, "a b", sqlFts5CleanupText("a        b   "))
	})
	t.Run("With punctuation", func(t *testing.T) {
		require.Equal(t, "a b", sqlFts5CleanupText("a, b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a; b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a. b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a* b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a& b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a% b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a# b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a? b"))
		require.Equal(t, "a b", sqlFts5CleanupText("a? b $"))
	})
	t.Run("nostr", func(t *testing.T) {
		require.Equal(t, "a b", sqlFts5CleanupText("npub112412951925 a npub112412951925 b npub112412951925"))
		require.Equal(t, "a b", sqlFts5CleanupText("nsec35235236622 a nsec35235236622 b nsec35235236622"))
		require.Equal(t, "a b", sqlFts5CleanupText("nprofile35235236622 a nprofile35235236622 b nprofile35235236622"))
		require.Equal(t, "a b", sqlFts5CleanupText("nostr:nprofile35235236622 a nostr:nprofile35235236622 b nostr:nprofile35235236622"))
	})
	t.Run("hashes", func(t *testing.T) {
		require.Equal(t, "a b", sqlFts5CleanupText("a b #some"))
		require.Equal(t, "a b", sqlFts5CleanupText("#hash a #testhash b #some #some2 #some_hash"))
	})
}

func TestSqlExtractIMeta(t *testing.T) {
	t.Parallel()

	jsonIMeta := `["imeta","url https://alicerelay.example.com","m image/jpg","dim 3024x4032","i foobar","alt A scenic photo overlooking the coast of Costa Rica","summary dummy summary content","x 68747470733a2f2f616c69636572656c61792e6578616d706c652e636f6d","ox 68747470733a2f2f616c69636572656c61792e6578616d706c652e636f6d"]`

	val, err := sqlExtractIMeta(jsonIMeta, "alt")
	require.NoError(t, err)
	require.Equal(t, "A scenic photo overlooking the coast of Costa Rica", val)
}
