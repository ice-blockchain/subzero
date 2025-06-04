// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query/internal/postgres/fixture"
	"github.com/ice-blockchain/subzero/model"
)

var (
	mainTestContainer *fixture.Container
)

func helperNewDatabase(t *testing.T) *dbClient {
	t.Helper()

	connString, _ := mainTestContainer.MustTempDB(t.Context())

	dbClient := openDatabase(t.Context(), connString, true).
		WithPrivateKey(model.GeneratePrivateKey()).
		WithRelayURL("wss://localhost")

	return dbClient
}

func TestMain(m *testing.M) {
	mainTestContainer = fixture.New(context.Background())
	code := m.Run()
	mainTestContainer.Close(context.Background())
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Printf("goleak found issues: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestReplaceableEvents(t *testing.T) {
	t.Parallel()

	t.Run("normal, non-replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		expectedEvents := []*model.Event{}
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()) + 1,
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[1], stored[0])
		require.EqualValues(t, expectedEvents[0], stored[1])
	})
	t.Run("ephemeral event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:        "normal, 1st event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},

				Content: `{"name":"username","about":"bogus","picture":"https://localhost:9999/bogus.jpg"}`,
			},
		}))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Empty(t, stored)
	})
	t.Run("normal, replaceable event with user metadata", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		expectedEvents := []*model.Event{}
		expectedEvents = append(expectedEvents, &model.Event{Event: nostr.Event{Tags: model.Tags{}}})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},

				Content: `{"name":"username","about":"bogus","picture":"https://localhost:9999/bogus.jpg"}`,
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindProfileMetadata},
		})
		require.Len(t, stored, 2)
		require.EqualValues(t, stored[0], expectedEvents[1])
		require.EqualValues(t, stored[1], expectedEvents[0])
	})

	t.Run("replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		ev1 := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable event 1 that must be replaced",
				PubKey:    "bogus",
				CreatedAt: 1,
				Kind:      nostr.KindFollowList,
				Tags:      model.Tags{{"p", "event1", "wss://localhost:9999/"}},
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), ev1))

		// Overwrite.
		ev2 := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable event 2",
				PubKey:    "bogus",
				CreatedAt: 2,
				Kind:      nostr.KindFollowList,
				Tags:      nostr.Tags{{"p", "event2", "wss://localhost:9999/"}},
			},
		}

		// Add another event.
		ev3 := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable event 3",
				PubKey:    "another bogus",
				CreatedAt: 3,
				Kind:      nostr.KindFollowList,
				Tags:      nostr.Tags{{"p", "event3", "wss://localhost:9999/"}},
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), ev2, ev3))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindFollowList},
		})
		require.Len(t, stored, 2)
		require.Equal(t, ev3, stored[0], "event 3")
		require.Equal(t, ev2, stored[1], "event 2")

		// Rollback
		require.NoError(t, db.RollbackEvents(t.Context(), ev2, ev3))
		stored = helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindFollowList},
		})
		require.Len(t, stored, 1)
		require.Equal(t, ev1, stored[0], "event 1")

		// Replaceable event is not rollbackable if called from consensus replay (as already committed).
		// Overwrite once again
		replayCtx := context.WithValue(t.Context(), model.ConsensusReplayCtxKey, true)
		require.NoError(t, db.AcceptEvents(replayCtx, ev2))

		stored = helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindFollowList},
		})
		require.Len(t, stored, 1)
		require.Equal(t, ev2, stored[0], "event 2")
		require.NoError(t, db.RollbackEvents(t.Context(), ev2)) // No-op.
		stored = helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindFollowList},
		})
		require.Len(t, stored, 1)
		require.Equal(t, ev2, stored[0], "event 2")
	})
}

func TestParametrizedReplaceableEvents(t *testing.T) {
	t.Parallel()

	t.Run("param replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		expectedEvents := []*model.Event{}
		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:        "item to be replaced" + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: model.Tags{
					[]string{"d", "bogus"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		}))
		// Overwrite
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable 1 " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: model.Tags{
					[]string{"d", "bogus"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))
		// Another D value
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable 2 " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: model.Tags{
					[]string{"d", "another bogus" + uuid.NewString()},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))
		// Another pubkey
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable 3 " + uuid.NewString(),
				PubKey:    "another bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: model.Tags{
					[]string{"d", "bogus" + uuid.NewString()},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[2]))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindRepositoryAnnouncement},
		})
		require.Len(t, stored, 3)
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
		require.Contains(t, stored, expectedEvents[2])
	})
}

func TestEphemeralEvents(t *testing.T) {
	t.Parallel()

	t.Run("ephemeral event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:        "ephemeral" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindClientAuthentication,
			},
		}))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Empty(t, stored)
	})
}

func TestNIP09DeleteEvents(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("normal, non-replaceable event", func(t *testing.T) {
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal1",
				PubKey:    "pk1",
				CreatedAt: 1,
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), publishedEvent))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)

		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:     "deletion event",
				PubKey: publishedEvent.PubKey,
				Kind:   nostr.KindDeletion,
				Tags: model.Tags{
					{"e", publishedEvent.ID},
				},
			},
		}))
		require.Empty(t, helperSelectEvents(t, db))
	})
	t.Run("replaceable event without d-tag", func(t *testing.T) {
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable1",
				PubKey:    "pk2",
				CreatedAt: 2,
				Kind:      nostr.KindProfileMetadata,
				Content:   "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
				Tags:      model.Tags{},
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), publishedEvent))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindProfileMetadata},
		})
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)

		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:     "deletion event2",
				PubKey: publishedEvent.PubKey,
				Kind:   nostr.KindDeletion,
				Tags: model.Tags{
					{"a", fmt.Sprintf("%v:%v:", nostr.KindProfileMetadata, publishedEvent.PubKey)},
				},
			},
		}))
		require.Empty(t, helperSelectEvents(t, db))
	})
	t.Run("replaceable event with d tag", func(t *testing.T) {
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable1",
				PubKey:    "pk3",
				CreatedAt: 3,
				Kind:      nostr.KindArticle,
				Tags: model.Tags{
					{"d", "bogus"},
				},
				Content: "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), publishedEvent))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindArticle},
		})
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)

		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:     "deletion event3",
				PubKey: publishedEvent.PubKey,
				Kind:   nostr.KindDeletion,
				Tags: model.Tags{
					{"a", fmt.Sprintf("%v:%v:bogus", nostr.KindArticle, publishedEvent.PubKey)},
				},
			},
		}))
		require.Empty(t, helperSelectEvents(t, db))
	})
	t.Run("event that doesn't exist", func(t *testing.T) {
		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				PubKey: "bogus",
				Kind:   nostr.KindDeletion,
				Tags: model.Tags{
					{"e", "bogus"},
				},
			},
		}))
	})
	t.Run("account delete", func(t *testing.T) {
		require.NoError(t, db.AcceptEvents(t.Context(),
			&model.Event{
				Event: nostr.Event{
					ID:        "replaceable1",
					PubKey:    "pk4",
					CreatedAt: 4,
					Kind:      nostr.KindProfileMetadata,
					Content:   "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
				}},
			&model.Event{
				Event: nostr.Event{
					ID:        "note2",
					PubKey:    "pk4",
					CreatedAt: 5,
					Kind:      nostr.KindTextNote,
					Content:   "bogus",
				}},
		))
		require.Equal(t, 2, len(helperSelectEvents(t, db)))
		require.NoError(t, db.AcceptEvents(t.Context(),
			&model.Event{
				Event: nostr.Event{
					ID:      "deletion event4",
					PubKey:  "pk4",
					Kind:    nostr.KindDeletion,
					Content: "Remember me",
				}},
		))
		require.Empty(t, helperSelectEvents(t, db))
	})
}

func TestQueryEventWithTagsReorderAndSignature(t *testing.T) {
	t.Parallel()

	pk := model.GeneratePrivateKey()
	require.NotEmpty(t, pk)

	var ev model.Event
	ev.Tags = model.Tags{{"a", "b"}, {"imeta", "foo", "bar", "m image/png"}, {"c", "d"}}
	ev.Content = "some tags and content here"
	ev.CreatedAt = 1
	ev.Kind = nostr.KindTextNote

	require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	t.Logf("event id: %s (sign %v)", ev.ID, ev.Sig)

	ok, err := ev.CheckSignature()
	require.NoError(t, err)
	require.True(t, ok)

	t.Run("SingleEvent", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		t.Run("Save", func(t *testing.T) {
			err := db.AcceptEvents(t.Context(), &ev)
			require.NoError(t, err)
		})
		t.Run("ByID", func(t *testing.T) {
			events := helperSelectEvents(t, db, model.Filter{
				IDs: []string{ev.ID},
			})
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
		t.Run("ByMimeType", func(t *testing.T) {
			events := helperSelectEvents(t, db, model.Filter{
				Search: "images:true",
			})
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
	})
	t.Run("RepostEvent", func(t *testing.T) {
		var repostEvent model.Event

		pk2 := model.GeneratePrivateKey()
		require.NotEmpty(t, pk2)

		data, err := ev.MarshalJSON()
		require.NoError(t, err)

		repostEvent.Content = string(data)
		repostEvent.CreatedAt = 2
		repostEvent.Kind = nostr.KindRepost

		require.NoError(t, repostEvent.SignWithAlg(pk2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		t.Logf("event id: %s (sign %v)", repostEvent.ID, repostEvent.Sig)

		db := helperNewDatabase(t)
		defer db.Close()

		t.Run("Save", func(t *testing.T) {
			err := db.AcceptEvents(t.Context(), &repostEvent)
			require.NoError(t, err)
		})
		t.Run("ByID", func(t *testing.T) {
			events := helperSelectEvents(t, db, model.Filter{
				IDs: []string{repostEvent.ID},
			})
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
		t.Run("ByMimeType", func(t *testing.T) {
			events := helperSelectEvents(t, db, model.Filter{
				Search: "images:true",
				Kinds:  []int{nostr.KindRepost},
			})
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			require.Equal(t, repostEvent.ID, events[0].ID) // Should be the reposted event.
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
		t.Run("Count", func(t *testing.T) {
			count, err := db.CountEvents(t.Context())
			require.NoError(t, err)
			require.Equal(t, int64(1), count) // Only the reposted event should be counted.
		})
	})
}

func TestQueryEventAttestation(t *testing.T) {
	t.Parallel()

	master, masterPk := model.GenerateKeyPair()
	active, activePk := model.GenerateKeyPair()

	t.Logf("master   public key: %s", masterPk)
	t.Logf("onbehalf public key: %s", activePk)

	db := helperNewDatabase(t)
	defer db.Close()

	now := time.Now().Unix()

	t.Run("AddAttestation", func(t *testing.T) {
		var ev model.Event

		t.Log("add first attestation")
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = model.Tags{{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now, 10)}}
		require.NoError(t, ev.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		t.Logf("event %+v", ev)
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))

		count, err := db.CountEvents(t.Context(), model.Filter{
			Kinds:   []int{model.CustomIONKindAttestation},
			Authors: []string{masterPk},
			Search:  "nostr",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
		t.Run("TryOverride", func(t *testing.T) {
			ev.Kind = model.CustomIONKindAttestation
			ev.CreatedAt = 2
			ev.Tags = model.Tags{
				{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now-1, 10)},
			}
			require.NoError(t, ev.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			t.Logf("event %+v", ev)
			require.ErrorIs(t, db.AcceptEvents(t.Context(), &ev), ErrAttestationUpdateRejected)
		})

		t.Log("add second attestation")
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 3
		ev.Tags = model.Tags{
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now, 10)},
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now-20, 10)},
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindInactive + ":" + strconv.FormatInt(now-10, 10)},
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now-5, 10)},
		}
		require.NoError(t, ev.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		t.Logf("event %+v", ev)
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))

		count, err = db.CountEvents(t.Context(), model.Filter{
			Kinds:   []int{model.CustomIONKindAttestation},
			Authors: []string{masterPk},
			Search:  "nostr",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("Publish", func(t *testing.T) {
		t.Run("AsMaster", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = 1
			ev.Content = "hello world"
			require.NoError(t, ev.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		})
		var originalPost model.Event
		t.Run("OnBehalf", func(t *testing.T) {
			originalPost.Kind = nostr.KindTextNote
			originalPost.CreatedAt = 2
			originalPost.Content = "hello world from active"
			originalPost.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPk}}
			require.NoError(t, originalPost.SignWithAlg(active, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			t.Logf("event %+v", originalPost)
			require.NoError(t, db.AcceptEvents(t.Context(), &originalPost))
		})
		t.Run("OnBehalfOfUnknownUser", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = 3
			ev.Content = "hello world from non-existing user"
			ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, model.GeneratePrivateKey()}}
			require.NoError(t, ev.SignWithAlg(active, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			t.Logf("event %+v", ev)
			require.ErrorIs(t, db.AcceptEvents(t.Context(), &ev), model.ErrOnBehalfAccessDenied)
		})
		otherUserMasterPrivKey, otherUserMasterPubkey := model.GenerateKeyPair()
		t.Run("OnBehalfOfUnknownUserWithEphemeralEmbedding", func(t *testing.T) {
			var repost model.Event
			repost.Kind = nostr.KindRepost
			repost.CreatedAt = 4
			repost.Content = originalPost.String()
			privKey, pubKey := model.GenerateKeyPair()
			repost.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, otherUserMasterPubkey}}
			require.NoError(t, repost.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			var ephemeralAttestation model.Event
			ephemeralAttestation.Kind = model.CustomIONKindAttestation
			ephemeralAttestation.CreatedAt = 1
			ephemeralAttestation.Tags = model.Tags{{model.TagAttestationName, pubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(1, 10)}}
			require.NoError(t, ephemeralAttestation.SignWithAlg(otherUserMasterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			wrappedEphemeralAttestation := &model.Event{Event: nostr.Event{
				CreatedAt: 4,
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					[]string{model.CustomIONTagOnBehalfOf, otherUserMasterPubkey},
					[]string{"e", repost.ID}},
				Content: ephemeralAttestation.String(),
			}}

			t.Logf("event %+v %+v", repost, wrappedEphemeralAttestation)
			require.NoError(t, db.AcceptEvents(t.Context(), &repost, wrappedEphemeralAttestation))
		})
		t.Run("Count", func(t *testing.T) {
			count, err := db.CountEvents(t.Context(), model.Filter{
				Kinds:   []int{nostr.KindTextNote},
				Authors: []string{masterPk},
				Search:  "nostr",
			})
			require.NoError(t, err)
			require.Equal(t, int64(2), count) // Both events should be counted, master + on behalf.
			count, err = db.CountEvents(t.Context(), model.Filter{
				Kinds:   []int{nostr.KindRepost},
				Authors: []string{otherUserMasterPubkey},
				Search:  "nostr",
			})
			require.NoError(t, err)
			require.Equal(t, int64(1), count) // Repost saved althrough it did not have attestation saved
		})
	})
}

func TestEventDeleteWithAttestation(t *testing.T) {
	t.Parallel()

	masterPrivate, masterPublic := model.GenerateKeyPair()
	user1Private, user1Public := model.GenerateKeyPair()
	user2Private, user2Public := model.GenerateKeyPair()
	hackerPrivate, hackerPublic := model.GenerateKeyPair()

	db := helperNewDatabase(t)
	defer db.Close()
	now := time.Now().Unix()

	baseAttestation := model.Tags{
		{model.TagAttestationName, user1Public, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-10))},
		{model.TagAttestationName, user2Public, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-5))},
	}
	masterMessageIds := []string{}
	user1MessageIds := []string{}
	user2MessageIds := []string{}
	user3MessageIds := []string{}

	counter := func(t *testing.T, kinds []int, ids, authors []string) int64 {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Authors: authors,
			Kinds:   kinds,
			IDs:     ids,
			Search:  "nostr",
		})
		require.NoError(t, err)
		return count
	}
	mustBeZero := func(t *testing.T, id string) {
		require.Zero(t, counter(t, nil, []string{id}, nil))
	}
	mustBeOne := func(t *testing.T, id string) {
		require.Equal(t, int64(1), counter(t, nil, []string{id}, nil))
	}

	t.Run("AddAttestation", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = baseAttestation
		require.NoError(t, ev.SignWithAlg(masterPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))
	})
	t.Run("AddEvents", func(t *testing.T) {
		t.Run("Master", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(3 + n)
				ev.Content = "hello world" + strconv.Itoa(n)
				require.NoError(t, ev.SignWithAlg(masterPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				masterMessageIds = append(masterMessageIds, ev.ID)
				require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			}
		})
		t.Run("Master of behalf of user1", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(5 + n)
				ev.Content = "hello world from user1 number" + strconv.Itoa(n)
				ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPublic}}
				require.NoError(t, ev.SignWithAlg(user1Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				user1MessageIds = append(user1MessageIds, ev.ID)
				require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			}
			t.Logf("user 1 messages = %v", user1MessageIds)
		})
		t.Run("Master of behalf of user2", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(7 + n)
				ev.Content = "hello world from user2 number" + strconv.Itoa(n)
				ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPublic}}
				require.NoError(t, ev.SignWithAlg(user2Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				user2MessageIds = append(user2MessageIds, ev.ID)
				require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			}
			t.Logf("user 2 messages = %v", user2MessageIds)
		})
		t.Run("Count", func(t *testing.T) {
			require.Equal(t, int64(6), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("User2 could not add master attestation", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindAttestation
			ev.CreatedAt = 11
			ev.Tags = append(ev.Tags, baseAttestation...)
			ev.Tags = append(ev.Tags, model.Tag{model.TagAttestationName, hackerPublic, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-1))})
			ev.Tags = append(ev.Tags, model.Tag{model.CustomIONTagOnBehalfOf, masterPublic})
			require.NoError(t, ev.SignWithAlg(user2Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.ErrorIs(t, db.AcceptEvents(t.Context(), &ev), model.ErrOnBehalfAccessDenied)
		})
	})
	t.Run("DeleteEvents", func(t *testing.T) {
		t.Run("Master could remove events of user1", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 10
			ev.Tags = model.Tags{{"e", user1MessageIds[0]}}
			require.NoError(t, ev.SignWithAlg(masterPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))

			mustBeZero(t, user1MessageIds[0])
			require.Equal(t, int64(5), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("User2 could remove events of user1", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{
				{"e", user1MessageIds[1]},
				{model.CustomIONTagOnBehalfOf, masterPublic},
			}
			require.NoError(t, ev.SignWithAlg(user2Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))

			mustBeZero(t, user1MessageIds[1])
			require.Equal(t, int64(4), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("User2 tries to remove events of user1 again and they don't exist", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", user1MessageIds[1]}}
			require.NoError(t, ev.SignWithAlg(user2Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))

			mustBeZero(t, user1MessageIds[1])
			require.Equal(t, int64(4), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("Hacker could not remove events of user2 nor master", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", user2MessageIds[0]}}
			require.NoError(t, ev.SignWithAlg(hackerPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))

			ev.Tags = model.Tags{{"e", masterMessageIds[0]}}
			require.NoError(t, ev.SignWithAlg(hackerPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))

			require.EqualValues(t, 2, counter(t, nil, []string{user2MessageIds[0], masterMessageIds[0]}, nil))
		})
		t.Run("User1 could not remove master events", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", masterMessageIds[1]}}
			require.NoError(t, ev.SignWithAlg(user2Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			mustBeOne(t, masterMessageIds[1])
			require.Equal(t, int64(4), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
	})
	t.Run("Rewoke", func(t *testing.T) {
		t.Run("Revoke attestation of user1", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindAttestation
			ev.CreatedAt = 12
			ev.Tags = append(ev.Tags, baseAttestation...)
			ev.Tags = append(ev.Tags, model.Tag{model.TagAttestationName, user1Public, "", model.CustomIONAttestationKindRevoked + ":" + strconv.Itoa(int(now-3))})
			require.NoError(t, ev.SignWithAlg(masterPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		})
		t.Run("User1 could not remove events of user2", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", user2MessageIds[0]}}
			require.NoError(t, ev.SignWithAlg(user1Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			mustBeOne(t, user2MessageIds[0])
		})
	})
	t.Run("ephemeral", func(t *testing.T) {
		user3MasterPrivate, user3MasterPublic := model.GenerateKeyPair()
		user3Private, user3Public := model.GenerateKeyPair()

		var ephemeralAttestation model.Event
		ephemeralAttestation.Kind = model.CustomIONKindAttestation
		ephemeralAttestation.CreatedAt = 1
		ephemeralAttestation.Tags = model.Tags{{model.TagAttestationName, user3Public, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(1, 10)}}
		require.NoError(t, ephemeralAttestation.SignWithAlg(user3MasterPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		t.Run("user 3 publishes reply to master event", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(8 + n)
				ev.Content = "hello world from user3 number" + strconv.Itoa(n)
				ev.Tags = model.Tags{
					{model.CustomIONTagOnBehalfOf, user3MasterPublic},
					{"e", masterMessageIds[0], "wss://relay.com", model.TagMarkerReply},
				}
				require.NoError(t, ev.SignWithAlg(user3Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				user3Attestation := &model.Event{Event: nostr.Event{
					CreatedAt: 4,
					Kind:      model.CustomIONKindEphemeralEmbeddding,
					Tags: nostr.Tags{
						[]string{model.CustomIONTagOnBehalfOf, user3MasterPublic},
						[]string{"e", ev.ID}},
					Content: ephemeralAttestation.String(),
				}}
				require.NoError(t, ephemeralAttestation.SignWithAlg(user3Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				require.NoError(t, db.AcceptEvents(t.Context(), &ev, user3Attestation))
				user3MessageIds = append(user3MessageIds, ev.ID)
			}
			t.Logf("user 3 messages = %v", user3MessageIds)
		})
		t.Run("user 3 deletes his replies to master event", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(9 + n)
				ev.Tags = model.Tags{
					{model.CustomIONTagOnBehalfOf, user3MasterPublic},
					{"k", fmt.Sprintf("%v", nostr.KindTextNote)},
					{"e", user3MessageIds[n]},
				}
				require.NoError(t, ev.SignWithAlg(user3Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				user3Attestation := &model.Event{Event: nostr.Event{
					CreatedAt: 4,
					Kind:      model.CustomIONKindEphemeralEmbeddding,
					Tags: nostr.Tags{
						[]string{model.CustomIONTagOnBehalfOf, user3MasterPublic},
						[]string{"e", ev.ID}},
					Content: ephemeralAttestation.String(),
				}}
				require.NoError(t, ephemeralAttestation.SignWithAlg(user3Private, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				require.NoError(t, db.AcceptEvents(t.Context(), &ev, user3Attestation))
			}
		})
	})
}

func TestQueryReply(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("AddEvents", func(t *testing.T) {
		var ev model.Event

		ev.ID = "1"
		ev.PubKey = "1pub"
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "hello world"
		ev.Tags = model.Tags{{"e", "event1", "", "reply"}}

		var ev2 model.Event
		ev2.ID = "2"
		ev2.PubKey = "2pub"
		ev2.Kind = nostr.KindTextNote
		ev2.CreatedAt = 2
		ev2.Content = "hello world 2"
		ev2.Tags = model.Tags{{"e", "event2", "", "root"}}
		require.NoError(t, db.AcceptEvents(t.Context(), &ev, &ev2))
	})
	t.Run("Filter", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Tags: model.TagMap{}.
				Set("e", nil, nil, model.PointerOf("reply")).
				Append("e", nil, nil, model.PointerOf("root")),
		})
		require.Len(t, events, 2)
	})
}

func TestQueryDiscoverContentCreatorsToFollow(t *testing.T) {
	t.Parallel()

	db, _ := helperEnsureDatabaseWithData(t, 1000)
	defer db.Close()

	t.Run("Filter", func(t *testing.T) {
		const eventCount = 10

		eventsRandom := helperSelectEvents(t, db, model.Filter{
			Search: "discover content creators to follow",
			Limit:  eventCount,
		})
		require.Len(t, eventsRandom, eventCount)

		eventsNotRandom := helperSelectEvents(t, db, model.Filter{
			Limit: eventCount,
		})
		require.Len(t, eventsNotRandom, eventCount)

		require.NotEqual(t, eventsRandom, eventsNotRandom)
	})
}

func TestSelectFilterATagWithAttestation(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	priv1, pub1 := model.GenerateKeyPair()
	_, pub2 := model.GenerateKeyPair()

	t.Run("Add attestation", func(t *testing.T) {
		var attestation model.Event
		attestation.Kind = model.CustomIONKindAttestation
		attestation.CreatedAt = 1
		attestation.Tags = model.Tags{
			{model.TagAttestationName, pub2, "", model.CustomIONAttestationKindActive + ":1"},
		}
		require.NoError(t, attestation.SignWithAlg(priv1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &attestation))
	})
	t.Run("Create event", func(t *testing.T) {
		var evMain model.Event
		evMain.CreatedAt = 2
		evMain.Kind = nostr.KindTextNote
		evMain.Content = "hello world"
		evMain.Tags = model.Tags{
			{"a", "1:" + pub1 + ":"},
		}
		require.NoError(t, evMain.SignWithAlg(priv1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &evMain))
	})
	t.Run("Lookup", func(t *testing.T) {
		eventsByMaster := helperSelectEvents(t, db, model.Filter{
			Limit: 2,
			Tags: model.TagMap{}.
				SetLiterals("a", "1:"+pub1+":"),
		})
		require.Len(t, eventsByMaster, 1)

		eventsByDelegated := helperSelectEvents(t, db, model.Filter{
			Limit: 2,
			Tags: model.TagMap{}.
				SetLiterals("a", "1:"+pub2+":"),
		})
		require.Len(t, eventsByDelegated, 1)

		require.Equal(t, eventsByMaster, eventsByDelegated)
	})
}

func TestDeleteNestedEvents(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	rootPriv := model.GeneratePrivateKey()
	var root model.Event

	t.Run("Add events", func(t *testing.T) {
		// Root event.
		root.CreatedAt = 1
		root.Kind = nostr.KindTextNote
		root.Content = "root event"
		require.NoError(t, root.SignWithAlg(rootPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &root))

		// First level events.
		var ev1 model.Event
		ev1.CreatedAt = 2
		ev1.Kind = nostr.KindTextNote
		ev1.Content = "regular event"
		ev1.Tags = model.Tags{{"e", root.ID}}
		require.NoError(t, ev1.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev1))

		var ev2 model.Event
		ev2.CreatedAt = 3
		ev2.Kind = nostr.KindArticle
		ev2.Content = "addressable event"
		ev2.Tags = model.Tags{
			{"d", "article1"},
			{"e", root.ID},
		}
		require.NoError(t, ev2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev2))

		var ev3 model.Event
		ev3.CreatedAt = 4
		ev3.Kind = nostr.KindProfileMetadata
		ev3.Content = "replaceable event"
		ev3.Tags = model.Tags{{"e", root.ID}}
		require.NoError(t, ev3.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev3))

		// Second level events.
		var ev4 model.Event
		ev4.CreatedAt = 5
		ev4.Kind = nostr.KindTextNote
		ev4.Content = "regular child event"
		ev4.Tags = model.Tags{{"e", ev1.ID}}
		require.NoError(t, ev4.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev4))

		var ev5 model.Event
		ev5.CreatedAt = 6
		ev5.Kind = nostr.KindArticle
		ev5.Content = "addressable child event"
		ev5.Tags = model.Tags{
			{"d", "article2"},
			{"a", ev2.Address()},
		}
		require.NoError(t, ev5.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev5))

		var ev6 model.Event
		ev6.CreatedAt = 7
		ev6.Kind = nostr.KindProfileMetadata
		ev6.Content = "replaceable child event"
		ev6.Tags = model.Tags{
			{"a", ev3.Address()},
		}
		require.NoError(t, ev6.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev6))
	})

	events := helperSelectEvents(t, db)
	require.Len(t, events, 7)

	var rootDelete model.Event
	t.Run("Delete root event", func(t *testing.T) {
		rootDelete.CreatedAt = 8
		rootDelete.Kind = nostr.KindDeletion
		rootDelete.Content = "delete root event"
		rootDelete.Tags = model.Tags{{"e", root.ID}}
		require.NoError(t, rootDelete.SignWithAlg(rootPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &rootDelete))

		// Check if all events are deleted.
		require.Zero(t, len(helperSelectEvents(t, db)))
	})
	t.Run("Rollback", func(t *testing.T) {
		require.NoError(t, db.RollbackEvents(t.Context(), &rootDelete))
		events := helperSelectEvents(t, db)
		require.Len(t, events, 7)
	})
}

func TestSelectRepostWithSpecialKind(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var post1, post2 model.Event
	post1.Kind = nostr.KindTextNote
	post1.Content = "post1 content"
	post1.CreatedAt = nostr.Now()
	require.NoError(t, post1.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	post2.Kind = model.CustomIONKindEditableTextNote
	post2.Content = "post2 content"
	post2.CreatedAt = nostr.Now()
	require.NoError(t, post2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var repost1, repost2 model.Event
	repost1.Kind, repost2.Kind = nostr.KindGenericRepost, nostr.KindGenericRepost
	repost1.CreatedAt, repost2.CreatedAt = nostr.Now(), nostr.Now()
	repost1.Content, repost2.Content = post1.String(), post2.String()

	repost1.Tags = model.Tags{
		{"e", post1.ID},
		{"k", strconv.Itoa(post1.Kind)},
	}

	repost2.Tags = model.Tags{
		{"a", post2.Address()},
		{"k", strconv.Itoa(post2.Kind)},
	}

	require.NoError(t, repost1.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, repost2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	require.NoError(t, db.AcceptEvents(t.Context(), &post1, &post2))
	require.NoError(t, db.AcceptEvents(t.Context(), &repost1, &repost2))
	t.Run("CustomIONKindRepostOfEditableTextNote", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindRepostOfEditableTextNote},
		})
		require.Len(t, events, 1)
		require.Equal(t, repost2.ID, events[0].ID)
	})
	t.Run("CustomIONKindRepostOfEditableTextNote with CustomIONKindRepostOfArticle", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindRepostOfEditableTextNote, model.CustomIONKindRepostOfArticle},
		})
		require.Len(t, events, 1)
		require.Equal(t, repost2.ID, events[0].ID)
	})
	t.Run("Repost with CustomIONKindRepostOfEditableTextNote", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindGenericRepost, model.CustomIONKindRepostOfEditableTextNote},
		})
		require.Len(t, events, 2)
		require.ElementsMatch(t, []*model.Event{&repost1, &repost2}, events)
	})
}

func TestEditablePostFlow(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var post model.Event
	post.Kind = model.CustomIONKindEditableTextNote
	post.ID = "1"
	post.PubKey = "1pub"
	post.CreatedAt = 1
	post.Content = "hello world"
	require.NoError(t, db.AcceptEvents(t.Context(), &post))

	t.Run("Quote", func(t *testing.T) {
		var quote1, quote2 model.Event

		quote1.Kind = nostr.KindArticle
		quote1.ID = "ev2"
		quote1.PubKey = "2pub"
		quote1.CreatedAt = 2
		quote1.Content = "quoted content1"
		quote1.Tags = model.Tags{
			{"Q", post.Address()},
			{"d", "article1"},
		}

		quote2.Kind = nostr.KindArticle
		quote2.ID = "ev3"
		quote2.PubKey = "2pub"
		quote2.CreatedAt = 3
		quote2.Content = "quoted content2"
		quote2.Tags = model.Tags{
			{"Q", post.Address()},
			{"d", "article2"},
		}

		require.NoError(t, db.AcceptEvents(t.Context(), &quote1, &quote2))

		helperMustBePrecalculatedCount(t, db, 2, model.Filter{
			Kinds: []int{nostr.KindArticle},
			Tags:  model.TagMap{}.SetLiterals("Q", post.Address()),
		})

		t.Run("Delete quote1", func(t *testing.T) {
			var delete model.Event

			delete.Kind = nostr.KindDeletion
			delete.ID = "3"
			delete.PubKey = "2pub"
			delete.CreatedAt = 3
			delete.Tags = model.Tags{{"e", quote1.ID}}
			require.NoError(t, db.AcceptEvents(t.Context(), &delete))

			helperMustBePrecalculatedCount(t, db, 1, model.Filter{
				Kinds: []int{nostr.KindArticle},
				Tags:  model.TagMap{}.SetLiterals("Q", post.Address()),
			})
		})
	})

	t.Run("Reply", func(t *testing.T) {
		var reply1, reply2 model.Event

		reply1.Kind = nostr.KindArticle
		reply1.ID = "ev4"
		reply1.PubKey = "2pub"
		reply1.CreatedAt = 2
		reply1.Content = "reply content1"
		reply1.Tags = model.Tags{
			{"a", post.Address(), "", "reply"},
			{"d", "reply1"},
		}

		reply2.Kind = nostr.KindArticle
		reply2.ID = "ev5"
		reply2.PubKey = "2pub"
		reply2.CreatedAt = 3
		reply2.Content = "reply content2"
		reply2.Tags = model.Tags{
			{"a", post.Address(), "", "root"},
			{"a", post.Address(), "", "reply"},
			{"d", "reply2"},
		}

		require.NoError(t, db.AcceptEvents(t.Context(), &reply1, &reply2))

		postAddress := post.Address()
		helperMustBePrecalculatedCount(t, db, 3, model.Filter{ // 1 root, 1 reply, 1 quote.
			Kinds: []int{nostr.KindArticle},
			Tags:  model.TagMap{}.SetLiterals("a", postAddress),
		})
		helperMustBePrecalculatedCount(t, db, 2, model.Filter{
			Kinds: []int{nostr.KindArticle},
			Tags:  model.TagMap{}.Set("a", &postAddress, nil, model.PointerOf("reply")),
		})

		t.Run("Delete reply1", func(t *testing.T) {
			var delete model.Event

			delete.Kind = nostr.KindDeletion
			delete.ID = "3"
			delete.PubKey = "2pub"
			delete.CreatedAt = 3
			delete.Tags = model.Tags{{"e", reply1.ID}}
			require.NoError(t, db.AcceptEvents(t.Context(), &delete))

			helperMustBePrecalculatedCount(t, db, 2, model.Filter{ // 1 reply, 1 quote.
				Kinds: []int{nostr.KindArticle},
				Tags:  model.TagMap{}.SetLiterals("a", postAddress),
			})
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{
				Kinds: []int{nostr.KindArticle},
				Tags:  model.TagMap{}.Set("a", &postAddress, nil, model.PointerOf("reply")),
			})
		})
	})
	t.Run("Delete post", func(t *testing.T) {
		var delete model.Event

		delete.Kind = nostr.KindDeletion
		delete.ID = "4"
		delete.PubKey = "1pub"
		delete.CreatedAt = 4
		delete.Tags = model.Tags{{"e", post.ID}}
		require.NoError(t, db.AcceptEvents(t.Context(), &delete))

		helperMustBePrecalculatedCount(t, db, 0, model.Filter{
			Kinds: []int{nostr.KindArticle},
			Tags:  model.TagMap{}.SetLiterals("a", post.Address()),
		})
		helperMustBePrecalculatedCount(t, db, 0, model.Filter{
			Kinds: []int{nostr.KindArticle},
			Tags:  model.TagMap{}.SetLiterals("Q", post.Address()),
		})
	})
}

func TestAccountDeleteWithSubAccounts(t *testing.T) {
	t.Parallel()

	const dummyAmount = 100
	db, _ := helperEnsureDatabaseWithData(t, dummyAmount)
	defer db.Close()

	masterPriv, masterPub := model.GenerateKeyPair()
	user1Priv, user1Pub := model.GenerateKeyPair()
	user2Priv, user2Pub := model.GenerateKeyPair()

	t.Run("Add attestation", func(t *testing.T) {
		var attestation model.Event
		attestation.Kind = model.CustomIONKindAttestation
		attestation.CreatedAt = 1
		attestation.Tags = model.Tags{
			{model.TagAttestationName, user1Pub, "", model.CustomIONAttestationKindActive + ":1"},
			{model.TagAttestationName, user2Pub, "", model.CustomIONAttestationKindActive + ":1"},
		}
		require.NoError(t, attestation.SignWithAlg(masterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &attestation))
	})
	t.Run("Post events on behalf of users", func(t *testing.T) {
		for i, key := range []string{user1Priv, user2Priv} {
			var ev model.Event
			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = model.Timestamp(1 + i)
			ev.Content = "hello world"
			ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPub}}
			require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		}
	})
	require.Len(t, helperSelectEvents(t, db), dummyAmount+3) // 1 attestation, 2 events.

	t.Run("Root account delete", func(t *testing.T) {
		var delete model.Event
		delete.Kind = nostr.KindDeletion
		delete.CreatedAt = 3
		delete.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPub}}
		require.NoError(t, delete.SignWithAlg(masterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &delete))
		require.Len(t, helperSelectEvents(t, db), dummyAmount)
	})
}

func TestSelectSoftDeletedPosts(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	now := time.Now().Unix()
	var posts []*model.Event
	var keys []string

	t.Run("Create posts", func(t *testing.T) {
		posts = []*model.Event{
			{
				Event: nostr.Event{
					Kind:      nostr.KindArticle,
					Content:   "The quick brown fox jumps over the lazy dog",
					CreatedAt: model.Timestamp(now),
					Tags: model.Tags{
						{"published_at", strconv.FormatInt(now, 10)},
					},
				},
			},
			{
				Event: nostr.Event{
					Kind:      nostr.KindArticle,
					Content:   "Pack my box with five dozen liquor jugs",
					CreatedAt: nostr.Timestamp(now),
					Tags: model.Tags{
						{"published_at", strconv.FormatInt(now, 10)},
						{"d", "article1"},
					},
				},
			},
			{
				Event: nostr.Event{
					Kind:      model.CustomIONKindEditableTextNote,
					Content:   "How vexingly quick daft zebras jump",
					CreatedAt: nostr.Timestamp(now),
					Tags: model.Tags{
						{"published_at", strconv.FormatInt(now, 10)},
					},
				},
			},
		}
		for i := range posts {
			key := model.GeneratePrivateKey()
			keys = append(keys, key)
			require.NoError(t, posts[i].SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		}
		require.NoError(t, db.AcceptEvents(t.Context(), posts...))
	})

	repostEvent := posts[1]

	t.Run("Create repost of post1", func(t *testing.T) {
		var repost model.Event
		repost.Kind = nostr.KindGenericRepost
		repost.Content = repostEvent.String()
		repost.CreatedAt = nostr.Now()
		repost.Tags = model.Tags{
			{"k", strconv.Itoa(repostEvent.Kind)},
			{"a", repostEvent.Address()},
		}
		require.NoError(t, repost.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &repost))
	})

	t.Run("Fetch all", func(t *testing.T) {
		require.Len(t, helperSelectEvents(t, db), 4, "should return all posts plus repost")
	})

	t.Run("Soft delete second post", func(t *testing.T) {
		deletedPost := &model.Event{
			Event: nostr.Event{
				Kind:      posts[1].Kind,
				Content:   "",                       // Empty content for soft deletion.
				CreatedAt: nostr.Timestamp(now + 1), // Should be newer than the original post.
				Tags: model.Tags{
					{"published_at", strconv.FormatInt(now, 10)},
					{"d", "article1"},
				},
			},
		}
		require.NoError(t, deletedPost.SignWithAlg(keys[1], model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), deletedPost))
		posts[1] = deletedPost
	})

	t.Run("Repost of deleted post", func(t *testing.T) {
		var repost model.Event
		repost.Kind = nostr.KindGenericRepost
		repost.Content = repostEvent.String()
		repost.CreatedAt = nostr.Now()
		repost.Tags = model.Tags{
			{"k", strconv.Itoa(repostEvent.Kind)},
			{"a", repostEvent.Address()},
		}
		require.NoError(t, repost.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.ErrorIs(t, db.AcceptEvents(t.Context(), &repost), ErrRepostOfDeletedPost)
	})

	t.Run("Fetch without filters", func(t *testing.T) {
		events := helperSelectEvents(t, db)
		require.Len(t, events, 2, "should return only non-deleted posts and no reposts")
		require.ElementsMatch(t, events, []*model.Event{posts[0], posts[2]}, "deleted post should not be included")
	})

	t.Run("Fetch without ID filters", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{Kinds: []int{nostr.KindArticle, model.CustomIONKindEditableTextNote}})
		require.Len(t, events, 2, "should return only non-deleted posts")
		require.ElementsMatch(t, events, []*model.Event{posts[0], posts[2]}, "deleted post should not be included")
	})

	t.Run("Fetch with ID filters", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			IDs: []string{posts[0].ID, posts[1].ID, posts[2].ID},
		})
		require.Len(t, events, 3, "should return all posts including deleted")
		require.ElementsMatch(t, posts, events)
	})

	t.Run("Fetch with addressable filters", func(t *testing.T) {
		events := helperSelectEvents(t, db,
			model.Filter{
				IDs: []string{posts[0].ID, posts[2].ID},
			},
			model.Filter{
				Kinds:   []int{posts[1].Kind},
				Authors: []string{posts[1].PubKey},
				Tags:    model.TagMap{}.SetLiterals("d", posts[1].Tags.GetD()),
				Limit:   10,
			},
		)
		require.Len(t, events, 3, "should return all posts including deleted")
		require.ElementsMatch(t, posts, events)
	})
}

func TestSelectRankTopEvents(t *testing.T) {
	t.Parallel()

	const (
		numLikes  = 5
		numQuotes = 3
	)

	db := helperNewDatabase(t)
	defer db.Close()

	// Create 3 text notes.
	notes := make([]*model.Event, 3)
	for i := range notes {
		notes[i] = &model.Event{
			Event: nostr.Event{
				ID:        "note" + strconv.Itoa(i+1),
				PubKey:    "pub" + strconv.Itoa(i+1),
				Kind:      nostr.KindTextNote,
				CreatedAt: nostr.Timestamp(time.Now().Unix() + int64(i)),
				Content:   "my text note " + strconv.Itoa(i+1),
			},
		}
	}
	require.NoError(t, db.AcceptEvents(t.Context(), notes...))

	// Add likes and quotes to first two notes.
	for i := 0; i < 2; i++ {
		// Add likes.
		for j := 0; j < numLikes+i; j++ {
			like := &model.Event{
				Event: nostr.Event{
					ID:        "like" + strconv.Itoa(i+1) + "_" + strconv.Itoa(j+1),
					PubKey:    "like_pub" + strconv.Itoa(j+10),
					Kind:      nostr.KindReaction,
					CreatedAt: nostr.Now(),
					Tags:      model.Tags{{"e", notes[i].ID}},
					Content:   "+",
				},
			}
			require.NoError(t, db.AcceptEvents(t.Context(), like))
		}

		// Add quotes.
		for j := 0; j < numQuotes+i; j++ {
			quote := &model.Event{
				Event: nostr.Event{
					ID:        "quote" + strconv.Itoa(i+1) + "_" + strconv.Itoa(j+1),
					PubKey:    "quote_pub" + strconv.Itoa(j+10),
					Kind:      nostr.KindTextNote,
					CreatedAt: nostr.Now(),
					Tags:      model.Tags{{"q", notes[i].ID}},
					Content:   "quote " + strconv.Itoa(j+1),
				},
			}
			require.NoError(t, db.AcceptEvents(t.Context(), quote))
		}
	}

	// Query top ranked events.
	top := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote},
		Search: "top",
	})

	trending := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote},
		Search: "trending",
	})
	require.Equal(t, top, trending)

	// Should return top 2 events with most reactions.
	require.Len(t, top, 2)
	require.Equal(t, notes[1].ID, top[0].ID)
	require.Equal(t, notes[0].ID, top[1].ID)

	helperPointsScoreEqual(t, db, notes[1].ID, 22, 22e4)
	helperPointsScoreEqual(t, db, notes[0].ID, 17, 17e4)
}

func TestExtendWhereFilters(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	in := model.Filter{
		Tags: model.TagMap{}.Set("Q", nil, nil, model.PointerOf("foo")),
	}
	out := db.extendWhereFilters(t.Context(), in.Clone())
	require.Len(t, out, 1)
	require.Equal(t, in, out[0])
}

func TestSoftDeletedReplies(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	now := time.Now().Unix()

	mainPost := &model.Event{
		Event: nostr.Event{
			Kind:      model.CustomIONKindEditableTextNote,
			CreatedAt: nostr.Timestamp(now),
			Content:   "This is the main editable post",
			Tags: model.Tags{
				{"published_at", strconv.FormatInt(now, 10)},
			},
		},
	}
	mainKey := model.GeneratePrivateKey()
	require.NoError(t, mainPost.SignWithAlg(mainKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), mainPost))

	// Create reply of the same kind.
	replyPost := &model.Event{
		Event: nostr.Event{
			Kind:      model.CustomIONKindEditableTextNote,
			CreatedAt: nostr.Timestamp(now),
			Content:   "This is a reply to the main post",
			Tags: model.Tags{
				{"published_at", strconv.FormatInt(now, 10)},
				{"a", mainPost.Address(), "", "reply"},
			},
		},
	}
	replyKey := model.GeneratePrivateKey()
	require.NoError(t, replyPost.SignWithAlg(replyKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), replyPost))

	// Check reply counter is 1
	mainPostAddress := mainPost.Address()
	helperMustBePrecalculatedCount(t, db, 1, model.Filter{
		Kinds: []int{model.CustomIONKindEditableTextNote},
		Tags:  model.TagMap{}.Set("a", &mainPostAddress, nil, model.PointerOf("reply")),
	})

	// Soft delete the reply.
	softDeletedReply := &model.Event{
		Event: nostr.Event{
			Kind:      model.CustomIONKindEditableTextNote,
			CreatedAt: nostr.Timestamp(now + 1), // Must be newer than original.
			Content:   "",                       // Empty content for soft deletion.
			Tags: model.Tags{
				{"published_at", strconv.FormatInt(now, 10)},
				{"a", mainPost.Address(), "", "reply"},
			},
		},
	}
	require.NoError(t, softDeletedReply.SignWithAlg(replyKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), softDeletedReply))

	// Check that reply counter is now zero.
	helperMustBePrecalculatedCount(t, db, 0, model.Filter{
		Kinds: []int{model.CustomIONKindEditableTextNote},
		Tags:  model.TagMap{}.Set("a", &mainPostAddress, nil, model.PointerOf("reply")),
	})
}

func TestSelectEventsSortMultipleFilters(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	now := nostr.Now()

	notes := make([]*model.Event, 3)
	for i := range notes {
		notes[i] = &model.Event{
			Event: nostr.Event{
				ID:        "note" + strconv.Itoa(i+1),
				PubKey:    "pub" + strconv.Itoa(i+1),
				Kind:      nostr.KindTextNote,
				CreatedAt: now.Add(time.Second + (time.Duration(i) * time.Second)),
				Content:   "my text note " + strconv.Itoa(i+1),
				Tags:      model.Tags{},
			},
		}
	}
	require.NoError(t, db.AcceptEvents(t.Context(), notes...))

	articles := make([]*model.Event, 3)
	for i := range articles {
		articles[i] = &model.Event{
			Event: nostr.Event{
				ID:        "article" + strconv.Itoa(i+1),
				PubKey:    "pub" + strconv.Itoa(i+1),
				Kind:      nostr.KindArticle,
				CreatedAt: now.Add(time.Minute + (time.Duration(i) * time.Second)),
				Content:   "my article " + strconv.Itoa(i+1),
				Tags:      model.Tags{},
			},
		}
	}
	require.NoError(t, db.AcceptEvents(t.Context(), articles...))

	events := helperSelectEvents(t, db,
		model.Filter{
			Kinds: []int{nostr.KindTextNote},
		},
		model.Filter{
			Kinds: []int{nostr.KindArticle},
		},
	)
	require.Len(t, events, 6)

	// Expected order: acticle3, article2, article1, note3, note2, note1. The newest events should be first.
	for i := range articles {
		require.Equal(t, articles[2-i], events[i])
		require.Equal(t, notes[2-i], events[i+3])
	}
}

func TestReplaceEventCheckSignature(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	pk := model.GeneratePrivateKey()

	var ev model.Event
	ev.Kind = nostr.KindProfileMetadata
	ev.CreatedAt = nostr.Now()
	ev.Content = "hello world"
	ev.Tags = model.Tags{{"x", "y"}}
	require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &ev))

	events := helperSelectEvents(t, db)
	require.Len(t, events, 1)
	require.Equal(t, &ev, events[0])
	ok, err := events[0].CheckSignature()
	require.NoError(t, err)
	require.True(t, ok)

	// Do update.
	ev.CreatedAt = nostr.Now() + 1
	ev.Tags = model.Tags{{"x", "z"}}
	require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &ev))

	events = helperSelectEvents(t, db)
	require.Len(t, events, 1)
	require.Equal(t, &ev, events[0])
	ok, err = events[0].CheckSignature()
	require.NoError(t, err)
	require.True(t, ok)
}

func TestQueryDependencyWithReply(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	priv, pub := model.GenerateKeyPair()
	var rootEvent, replyEvent model.Event

	rootEvent.CreatedAt = nostr.Now()
	rootEvent.Kind = model.CustomIONKindEditableTextNote
	rootEvent.Content = "root post"
	rootEvent.Tags = model.Tags{
		{"d", "root1"},
	}
	require.NoError(t, rootEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	replyEvent.CreatedAt = nostr.Now()
	replyEvent.Kind = model.CustomIONKindEditableTextNote
	replyEvent.Content = "reply post"
	replyEvent.Tags = model.Tags{
		{"a", rootEvent.Address(), "", "root"},
		{"a", rootEvent.Address(), "", "reply"},
		{"d", "reply1"},
	}
	require.NoError(t, replyEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &rootEvent, &replyEvent))

	f := model.Filter{
		Authors: []string{pub},
		Kinds:   []int{model.CustomIONKindEditableTextNote},
		Limit:   10,
		Search:  `include:dependencies:kind30175>` + pub + `@kind30175+e+reply references:false`,
	}

	events := helperSelectEvents(t, db, f)
	require.ElementsMatch(t, events, []*model.Event{&replyEvent, &rootEvent}, "should return both events") // root post, and reply using the dependency.
}
