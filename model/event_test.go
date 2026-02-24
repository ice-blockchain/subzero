// SPDX-License-Identifier: ice License 1.0

package model

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func helperTestSignVerify(t *testing.T, ev *Event, pk string, signAlg EventSignAlg, keyAlg EventKeyAlg) {
	t.Helper()

	t.Run("Sign", func(t *testing.T) {
		err := ev.SignWithAlg(pk, signAlg, keyAlg)
		require.NoError(t, err)
		t.Logf("%v: signature: %s", signAlg, ev.Sig)
	})
	t.Run("Verify", func(t *testing.T) {
		ok, err := ev.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
		t.Logf("%v: signature: %s: OK", signAlg, ev.Sig)
	})
}

func TestEventSignVerify(t *testing.T) {
	t.Parallel()

	t.Run(strings.Join([]string{string(SignAlgSchnorr), string(KeyAlgSecp256k1)}, "_"), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgSchnorr)

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, SignAlgSchnorr, KeyAlgSecp256k1)
	})
	t.Run(strings.Join([]string{string(SignAlgEDDSA), string(KeyAlgCurve25519)}, "_"), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgEDDSA)

		_, pk, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		helperTestSignVerify(t, &ev, hex.EncodeToString(pk), SignAlgEDDSA, KeyAlgCurve25519)
	})
	t.Run("ArgError", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			err := ev.SignWithAlg("", SignAlgEDDSA, "")
			require.ErrorIs(t, err, ErrUnsupportedAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			testData := []string{
				"foo",
				"/foo",
				"/",
				"foo/",
				"",
				"/",
				"//",
			}
			for i := range testData {
				var ev Event
				ev.Sig = testData[i] + ":" + hex.EncodeToString([]byte("signature"))
				ok, err := ev.CheckSignature()
				require.ErrorIsf(t, err, ErrUnsupportedAlg, "testData[%d]: %s", i, testData[i])
				require.False(t, ok)

			}
		})
	})
	t.Run("Unknown", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			pk := hex.EncodeToString([]byte("private key"))
			require.ErrorIs(t, ev.SignWithAlg(pk, EventSignAlg("unknown"), EventKeyAlg("unknown")), ErrUnsupportedAlg)
			require.ErrorIs(t, ev.SignWithAlg(pk, SignAlgSchnorr, KeyAlgCurve25519), ErrUnsupportedAlg)
			require.ErrorIs(t, ev.SignWithAlg(pk, SignAlgEDDSA, KeyAlgSecp256k1), ErrUnsupportedAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			var ev Event
			ev.Sig = "unknown/foo:" + hex.EncodeToString([]byte("signature"))
			ok, err := ev.CheckSignature()
			require.ErrorIs(t, err, ErrUnsupportedAlg)
			require.False(t, ok)
		})
	})
	t.Run("Default", func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "default"

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, "", "")
	})
}

func TestDeduplicateSlice(t *testing.T) {
	t.Parallel()

	t.Run("Events", func(t *testing.T) {
		events := []*Event{
			{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					ID:   "id1",
				},
			},
			{
				Event: nostr.Event{
					ID:   "id2",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id1",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id3",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id2",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id4",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id3",
					Kind: nostr.KindTextNote,
				},
			},
		}
		deduplicated := DeduplicateSlice(events, func(e *Event) string { return e.ID })
		require.Len(t, deduplicated, 4)
		require.Equal(t, "id1", deduplicated[0].ID)
		require.Equal(t, "id2", deduplicated[1].ID)
		require.Equal(t, "id3", deduplicated[2].ID)
		require.Equal(t, "id4", deduplicated[3].ID)
	})

	t.Run("String", func(t *testing.T) {
		ids := []string{"id1", "id2", "id1", "id3", "id2", "id4", "id3"}
		deduplicated := DeduplicateSlice(ids, func(e string) string { return e })
		require.Len(t, deduplicated, 4)
		require.Equal(t, "id1", deduplicated[0])
		require.Equal(t, "id2", deduplicated[1])
		require.Equal(t, "id3", deduplicated[2])
		require.Equal(t, "id4", deduplicated[3])
	})
}
func TestSplitBatch(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		result := SplitBatch([]int{}, 2)
		require.Len(t, result, 1)
		require.Empty(t, result[0])
	})

	t.Run("Smaller than batch", func(t *testing.T) {
		input := []int{1, 2}
		result := SplitBatch(input, 3)
		require.Len(t, result, 1)
		require.Equal(t, input, result[0])
	})

	t.Run("Equal to batch", func(t *testing.T) {
		input := []int{1, 2, 3}
		result := SplitBatch(input, 3)
		require.Len(t, result, 1)
		require.Equal(t, input, result[0])
	})

	t.Run("Multiple batches", func(t *testing.T) {
		input := []int{1, 2, 3, 4, 5, 6, 7}
		result := SplitBatch(input, 3)
		require.Len(t, result, 3)
		require.Equal(t, []int{1, 2, 3}, result[0])
		require.Equal(t, []int{4, 5, 6}, result[1])
		require.Equal(t, []int{7}, result[2])
	})
}

func TestHasVideoImeta(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		event    *Event
		expected bool
	}{
		{
			name: "should return true for video/mp4",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "url https://example.com/video.mp4", "m video/mp4"},
					},
				},
			},
			expected: true,
		},
		{
			name: "should return true for video/webm",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "url https://example.com/video.webm", "m video/webm"},
					},
				},
			},
			expected: true,
		},
		{
			name: "should return true for generic video type",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "url https://example.com/video", "m video"},
					},
				},
			},
			expected: true,
		},
		{
			name: "should return false for image/jpeg",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "url https://example.com/image.jpg", "m image/jpeg"},
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false for no imeta tags",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{},
				},
			},
			expected: false,
		},
		{
			name: "should return false for malformed imeta tag",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "invalid-format"},
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false for imeta without mime type",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "url https://example.com/file", "size 1024"},
					},
				},
			},
			expected: false,
		},
		{
			name: "should return true for multiple imeta tags with video",
			event: &Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"imeta", "url https://example.com/image.jpg", "m image/jpeg"},
						{"imeta", "url https://example.com/video.mp4", "m video/mp4"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := tc.event.HasVideoIMeta()
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestParseEphemeralEmbeddingEvents(t *testing.T) {
	t.Parallel()

	var profileEvent Event
	profileEvent.Kind = nostr.KindProfileMetadata
	profileEvent.CreatedAt = nostr.Now()
	profileEvent.Content = ProfileMetadataContent{
		Name:  "test",
		About: "test about",
	}.String()
	require.NoError(t, profileEvent.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

	t.Run("Multiple references", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = CustomIONKindEphemeralEmbedding
		ev.CreatedAt = nostr.Now()
		ev.Content = profileEvent.String()
		ev.Tags = nostr.Tags{
			{"e", "ref1"},
			{"a", "ref2"},
			{"e", "ref3"},
		}
		require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

		refs, err := ParseEphemeralEmbeddingEvents(&ev)
		require.NoError(t, err)

		for _, tag := range ev.Tags {
			if tag.Key() == "e" || tag.Key() == "a" {
				require.Contains(t, refs, tag.Value())
			}
		}

		for _, events := range refs {
			for _, event := range events {
				require.Equal(t, &profileEvent, event.ContentEvent)
			}
		}
	})
	t.Run("Malformed content", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = CustomIONKindEphemeralEmbedding
		ev.CreatedAt = nostr.Now()
		ev.Content = "invalid json"
		ev.Tags = nostr.Tags{
			{"e", "ref1"},
		}
		require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

		_, err := ParseEphemeralEmbeddingEvents(&ev)
		require.Error(t, err)
	})
	t.Run("No references", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = CustomIONKindEphemeralEmbedding
		ev.CreatedAt = nostr.Now()
		ev.Content = profileEvent.String()
		require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

		_, err := ParseEphemeralEmbeddingEvents(&ev)
		require.Error(t, err)
	})
	t.Run("Empty reference values", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = CustomIONKindEphemeralEmbedding
		ev.CreatedAt = nostr.Now()
		ev.Content = profileEvent.String()
		ev.Tags = nostr.Tags{
			{"e", ""},
			{"a", ""},
			{"e", "ref1"},
			{"a", "ref2"},
		}
		require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

		refs, err := ParseEphemeralEmbeddingEvents(&ev)
		require.NoError(t, err)

		// Empty tag values should be ignored as references.
		require.NotContains(t, refs, "")

		// Non-empty references should be parsed as in the multiple references case.
		require.Contains(t, refs, "ref1")
		require.Contains(t, refs, "ref2")

		for _, events := range refs {
			for _, event := range events {
				require.Equal(t, &profileEvent, event.ContentEvent)
			}
		}
	})
}
