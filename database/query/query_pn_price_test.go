// SPDX-License-Identifier: ice License 1.0

package query

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

func helperNewPriceActionEvent(t *testing.T, defAddress, amount string, ts nostr.Timestamp) *model.Event {
	t.Helper()

	var action model.Event
	action.Kind = model.CustomIONKindTokenizedCommunityAction
	action.CreatedAt = ts
	action.Content = "test action"
	action.Tags = model.Tags{
		{"a", defAddress},
		{"tx_amount", amount, "USD"},
		{"tx_type", "buy"},
	}
	require.NoError(t, action.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	return &action
}

func helperNewPriceChangeRequestEvent(t *testing.T, tokenAddress string, deltaPercentage int, ts nostr.Timestamp) model.Event {
	t.Helper()

	return helperNewPriceChangeRequestEventWithSigner(t, model.GeneratePrivateKey(), tokenAddress, 60, deltaPercentage, ts)
}

func helperNewPriceChangeRequestEventWithSigner(t *testing.T, signer, tokenAddress string, timeWindow, deltaPercentage int, ts nostr.Timestamp) model.Event {
	t.Helper()

	var req model.Event
	req.Kind = model.CustomIONKindDVMJobRequestPriceChange
	req.CreatedAt = ts
	req.Tags = model.Tags{
		{"param", "timeWindow", strconv.Itoa(timeWindow)},
		{"param", "deltaPercentage", strconv.Itoa(deltaPercentage)},
	}
	if tokenAddress != "" {
		req.Tags = append(req.Tags, model.Tag{"param", "token", tokenAddress})
	}

	require.NoError(t, req.SignWithAlg(signer, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	return req
}

func TestPriceChangeRegisterSubscriber(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var post, def model.Event
	post.Kind = nostr.KindArticle
	post.Content = "test post"
	post.CreatedAt = nostr.Now()
	require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	def.Kind = model.CustomIONKindTokenizedCommunityDefinition
	def.CreatedAt = nostr.Now()
	def.Content = "test definition"
	def.Tags = model.Tags{
		{"a", post.Address()},
	}
	require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	action := helperNewPriceActionEvent(t, def.Address(), "0.42", nostr.Now())

	require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, action))

	t.Run("Wildcard", func(t *testing.T) {
		req := helperNewPriceChangeRequestEvent(t, "", 5, nostr.Now())
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))
	})
	t.Run("OK", func(t *testing.T) {
		req := helperNewPriceChangeRequestEvent(t, def.Address(), 50, nostr.Now())
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))
	})
	t.Run("Invalid parameter", func(t *testing.T) {
		var req model.Event
		req.Kind = model.CustomIONKindDVMJobRequestPriceChange
		req.CreatedAt = nostr.Now()
		req.Tags = model.Tags{
			{"param", "timeWindow", strconv.Itoa(math.MaxUint32)},
			{"param", "deltaPercentage", "5"},
			{"param", "token", def.Address()},
		}
		require.NoError(t, req.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))
	})
	t.Run("Unknown token", func(t *testing.T) {
		var req model.Event
		req.Kind = model.CustomIONKindDVMJobRequestPriceChange
		req.CreatedAt = nostr.Now()
		req.Tags = model.Tags{
			{"param", "timeWindow", "60"},
			{"param", "deltaPercentage", "5"},
			{"param", "token", "unknown:token:address"},
		}
		require.NoError(t, req.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		err := db.RegisterPriceChangeSubscriber(t.Context(), "", &req)
		require.ErrorIs(t, err, connector.ErrNotFound)
	})
	t.Run("No price data", func(t *testing.T) {
		var postNote, defNote model.Event
		postNote.Kind = model.CustomIONKindEditableTextNote
		postNote.Content = "test post without price data"
		postNote.CreatedAt = nostr.Now()
		require.NoError(t, postNote.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		defNote.Kind = model.CustomIONKindTokenizedCommunityDefinition
		defNote.CreatedAt = nostr.Now()
		defNote.Content = "test definition without price data"
		defNote.Tags = model.Tags{
			{"a", postNote.Address()},
		}
		require.NoError(t, defNote.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &postNote, &defNote))

		req := helperNewPriceChangeRequestEvent(t, defNote.Address(), 5, nostr.Now())
		err := db.RegisterPriceChangeSubscriber(t.Context(), "", &req)
		require.NoError(t, err)
	})
}

func TestPriceChangeCreateNotification(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Trigger when previous price exists", func(t *testing.T) {
		ts := nostr.Now()

		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post"
		post.CreatedAt = ts
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = ts + 1
		def.Content = "test definition"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		firstAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+2)
		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, firstAction))

		const testDeviceUUID = "test-device-uuid-1234"
		req := helperNewPriceChangeRequestEvent(t, def.Address(), 5, ts+3)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), testDeviceUUID, &req))

		secondAction := helperNewPriceActionEvent(t, def.Address(), "120.00", ts+4)
		require.NoError(t, db.AcceptEvents(t.Context(), secondAction))

		candidates, lastID, err := db.CollectPriceChangeSubscribersCandidates(t.Context(), secondAction, 0, 10)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Contains(t, candidates, req.PubKey)
		require.NotZero(t, lastID)

		candidates, lastID2, err := db.CollectPriceChangeSubscribersCandidates(t.Context(), secondAction, lastID, 10)
		require.NoError(t, err)
		require.Empty(t, candidates)
		require.Equal(t, lastID, lastID2)

		notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), secondAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Len(t, notifications, 1)
		require.Equal(t, notifications[0].DevicePubKey, req.PubKey)
		require.Equal(t, firstAction.ID, notifications[0].PreviousEvent.ID)
		require.Equal(t, testDeviceUUID, notifications[0].DeviceUUID)
		ok, err := notifications[0].PreviousEvent.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)

		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req.PubKey, "", def.Address(), ""))
	})
	t.Run("First action updates baseline without trigger then second triggers", func(t *testing.T) {
		ts := nostr.Now()

		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post without initial price"
		post.CreatedAt = ts
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = ts + 1
		def.Content = "test definition without initial price"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def))

		req := helperNewPriceChangeRequestEvent(t, def.Address(), 5, ts+2)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))

		firstAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+3)
		require.NoError(t, db.AcceptEvents(t.Context(), firstAction))

		notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), firstAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Empty(t, notifications)

		secondAction := helperNewPriceActionEvent(t, def.Address(), "120.00", ts+4)
		require.NoError(t, db.AcceptEvents(t.Context(), secondAction))

		candidates, _, err := db.CollectPriceChangeSubscribersCandidates(t.Context(), secondAction, 0, 10)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Contains(t, candidates, req.PubKey)

		notifications, err = db.FetchAndUpdatePriceChangeNotification(t.Context(), secondAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Len(t, notifications, 1)
		require.Equal(t, notifications[0].DevicePubKey, req.PubKey)
		require.Equal(t, firstAction.ID, notifications[0].PreviousEvent.ID)
		require.JSONEq(t, req.String(), notifications[0].Request)
		ok, err := notifications[0].PreviousEvent.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)

		t.Run("Check internal state", func(t *testing.T) {
			type priceDataRow struct {
				LastTCActionEventID   string
				LastTCActionTimestamp int64
			}

			data, err := connector.Get[priceDataRow](t.Context(), db.db, `
				SELECT last_tc_action_event_id, last_tc_action_timestamp
				FROM pn_price_changes
				WHERE user_device_pubkey = $1 AND request_event_id = $2 and tc_definition_address != ''
			`, req.PubKey, req.ID,
			)
			require.NoError(t, err)
			require.Equal(t, secondAction.ID, data.LastTCActionEventID)
			require.EqualValues(t, secondAction.CreatedAt, data.LastTCActionTimestamp)
		})

		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req.PubKey, "", "", req.ID))
	})
	t.Run("Negative delta does not trigger on rise", func(t *testing.T) {
		ts := nostr.Now()

		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post for drop-only alert"
		post.CreatedAt = ts
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = ts + 1
		def.Content = "test definition for drop-only alert"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		firstAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+2)
		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, firstAction))

		req := helperNewPriceChangeRequestEvent(t, def.Address(), -5, ts+3)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))

		riseAction := helperNewPriceActionEvent(t, def.Address(), "120.00", ts+4)
		require.NoError(t, db.AcceptEvents(t.Context(), riseAction))

		notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), riseAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Empty(t, notifications)

		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req.PubKey, "", def.Address(), ""))
	})
	t.Run("Positive delta does not trigger on drop", func(t *testing.T) {
		ts := nostr.Now()

		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post for rise-only alert"
		post.CreatedAt = ts
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = ts + 1
		def.Content = "test definition for rise-only alert"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		firstAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+2)
		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, firstAction))

		req := helperNewPriceChangeRequestEvent(t, def.Address(), 5, ts+3)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))

		dropAction := helperNewPriceActionEvent(t, def.Address(), "80.00", ts+4)
		require.NoError(t, db.AcceptEvents(t.Context(), dropAction))

		notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), dropAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Empty(t, notifications)

		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req.PubKey, "", "", ""))
	})
	t.Run("Only one notification per window", func(t *testing.T) {
		ts := nostr.Now()

		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post for one-notification-per-window"
		post.CreatedAt = ts
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = ts + 1
		def.Content = "test definition for one-notification-per-window"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		baseAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+2)
		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, baseAction))

		req := helperNewPriceChangeRequestEvent(t, def.Address(), 5, ts+3)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))

		prices := []string{"106.00", "112.36", "119.10", "126.25", "133.83", "141.86"}
		notificationCount := 0

		for i, price := range prices {
			action := helperNewPriceActionEvent(t, def.Address(), price, ts+nostr.Timestamp(4+i))
			require.NoError(t, db.AcceptEvents(t.Context(), action))

			notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), action, []string{req.PubKey})
			require.NoError(t, err)

			if len(notifications) > 0 {
				notificationCount++
				require.Len(t, notifications, 1)
				require.Equal(t, notifications[0].DevicePubKey, req.PubKey)
				require.Equal(t, baseAction.ID, notifications[0].PreviousEvent.ID)
			}
		}

		require.Equal(t, 1, notificationCount)

		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req.PubKey, "", "", req.ID))
	})
	t.Run("Duplicate event id is ignored", func(t *testing.T) {
		ts := nostr.Now()

		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post duplicate event id"
		post.CreatedAt = ts
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = ts + 1
		def.Content = "test definition duplicate event id"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		baseAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+2)
		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, baseAction))

		req := helperNewPriceChangeRequestEvent(t, def.Address(), 5, ts+3)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "", &req))

		nextAction := helperNewPriceActionEvent(t, def.Address(), "120.00", ts+4)
		require.NoError(t, db.AcceptEvents(t.Context(), nextAction))

		notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), nextAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Len(t, notifications, 1)
		require.Equal(t, notifications[0].DevicePubKey, req.PubKey)
		require.Equal(t, baseAction.ID, notifications[0].PreviousEvent.ID)

		notifications, err = db.FetchAndUpdatePriceChangeNotification(t.Context(), nextAction, []string{req.PubKey})
		require.NoError(t, err)
		require.Empty(t, notifications)

		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req.PubKey, "", "", req.ID))
	})
	t.Run("Wildcard token subscriber receives notification for all of its tokens", func(t *testing.T) {
		tokenOwner := model.GeneratePrivateKey()

		var post, article, profile, defPost, defArticle, defProfile model.Event
		post.Kind = model.CustomIONKindEditableTextNote
		post.Content = "test post for wildcard token subscriber"
		post.CreatedAt = nostr.Now()
		require.NoError(t, post.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		article.Kind = nostr.KindArticle
		article.Content = "test article for wildcard token subscriber"
		article.CreatedAt = nostr.Now()
		require.NoError(t, article.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		profile.Kind = nostr.KindProfileMetadata
		profile.Content = model.ProfileMetadataContent{
			Name:  "Test User",
			About: "Testing wildcard token subscriber",
		}.String()
		profile.CreatedAt = nostr.Now()
		require.NoError(t, profile.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &post, &article, &profile))

		defPost.Kind = model.CustomIONKindTokenizedCommunityDefinition
		defPost.CreatedAt = post.CreatedAt.Add(time.Hour)
		defPost.Content = "test definition for wildcard token subscriber - post"
		defPost.Tags = model.Tags{
			{"a", post.Address()},
			{"d", "def_post"},
		}
		require.NoError(t, defPost.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		defArticle.Kind = model.CustomIONKindTokenizedCommunityDefinition
		defArticle.CreatedAt = article.CreatedAt.Add(time.Hour)
		defArticle.Content = "test definition for wildcard token subscriber - article"
		defArticle.Tags = model.Tags{
			{"a", article.Address()},
			{"d", "def_article"},
		}
		require.NoError(t, defArticle.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		defProfile.Kind = model.CustomIONKindTokenizedCommunityDefinition
		defProfile.CreatedAt = profile.CreatedAt.Add(time.Hour)
		defProfile.Content = "test definition for wildcard token subscriber - profile"
		defProfile.Tags = model.Tags{
			{"a", profile.Address()},
			{"d", "def_profile"},
		}
		require.NoError(t, defProfile.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &defPost, &defArticle, &defProfile))

		var req model.Event
		req.Kind = model.CustomIONKindDVMJobRequestPriceChange
		req.CreatedAt = nostr.Now()
		req.Tags = model.Tags{
			{"param", "timeWindow", "60"},
			{"param", "deltaPercentage", "10"},
		}
		require.NoError(t, req.SignWithAlg(tokenOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		const testDeviceUUID = "test-device-uuid-wildcard"
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), testDeviceUUID, &req))

		otherOwner := model.GeneratePrivateKey()

		var otherPost, otherDef model.Event
		otherPost.Kind = nostr.KindArticle
		otherPost.Content = "test article owned by someone else"
		otherPost.CreatedAt = req.CreatedAt + 1
		require.NoError(t, otherPost.SignWithAlg(otherOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &otherPost))

		otherDef.Kind = model.CustomIONKindTokenizedCommunityDefinition
		otherDef.CreatedAt = otherPost.CreatedAt.Add(time.Hour)
		otherDef.Content = "test definition owned by someone else"
		otherDef.Tags = model.Tags{
			{"a", otherPost.Address()},
			{"d", "def_other_owner"},
		}
		require.NoError(t, otherDef.SignWithAlg(otherOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &otherDef))

		var cases = []struct {
			Events           model.Events
			Name             string
			WantNotification bool
		}{
			{
				Name: "post baseline",
				Events: model.Events{
					helperNewPriceActionEvent(t, defPost.Address(), "100.00", req.CreatedAt+1),
				},
			},
			{
				Name: "post baseline notify",
				Events: model.Events{
					helperNewPriceActionEvent(t, defPost.Address(), "150.00", req.CreatedAt+2),
				},
				WantNotification: true,
			},
			{
				Name: "article baseline",
				Events: model.Events{
					helperNewPriceActionEvent(t, defArticle.Address(), "200.00", req.CreatedAt+62),
				},
			},
			{
				Name: "article baseline notify",
				Events: model.Events{
					helperNewPriceActionEvent(t, defArticle.Address(), "260.00", req.CreatedAt+63),
				},
				WantNotification: true,
			},
			{
				Name: "profile baseline",
				Events: model.Events{
					helperNewPriceActionEvent(t, defProfile.Address(), "300.00", req.CreatedAt+123),
				},
			},
			{
				Name: "profile baseline notify",
				Events: model.Events{
					helperNewPriceActionEvent(t, defProfile.Address(), "360.00", req.CreatedAt+124),
				},
				WantNotification: true,
			},
			{
				Name: "foreign owner baseline",
				Events: model.Events{
					helperNewPriceActionEvent(t, otherDef.Address(), "1000.00", req.CreatedAt+184),
				},
			},
			{
				Name: "foreign owner notify must stay filtered",
				Events: model.Events{
					helperNewPriceActionEvent(t, otherDef.Address(), "1300.00", req.CreatedAt+185),
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.Name, func(t *testing.T) {
				require.NoError(t, db.AcceptEvents(t.Context(), tc.Events...))

				candidates, _, err := db.CollectPriceChangeSubscribersCandidates(t.Context(), tc.Events[len(tc.Events)-1], 0, 10)
				require.NoError(t, err)
				if tc.WantNotification {
					require.NotEmpty(t, candidates)
					data, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), tc.Events[len(tc.Events)-1], []string{req.PubKey})
					require.NoError(t, err)
					require.Len(t, data, 1)
					require.Equal(t, testDeviceUUID, data[0].DeviceUUID)
					require.NotNil(t, data[0].PreviousEvent)
					ok, err := data[0].PreviousEvent.CheckSignature()
					require.NoError(t, err)
					require.True(t, ok)
				} else {
					require.Empty(t, candidates)
				}
			})
		}
	})
}

func TestPriceChangeRegisterSubscriberOverwriteConfig(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	ts := nostr.Now()

	var post, def model.Event
	post.Kind = nostr.KindArticle
	post.Content = "test post overwrite config"
	post.CreatedAt = ts
	require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	def.Kind = model.CustomIONKindTokenizedCommunityDefinition
	def.CreatedAt = ts + 1
	def.Content = "test definition overwrite config"
	def.Tags = model.Tags{{"a", post.Address()}}
	require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	baseAction := helperNewPriceActionEvent(t, def.Address(), "100.00", ts+2)
	require.NoError(t, db.AcceptEvents(t.Context(), &post, &def, baseAction))

	deviceSigner := model.GeneratePrivateKey()
	firstReq := helperNewPriceChangeRequestEventWithSigner(t, deviceSigner, def.Address(), 300, 50, ts+3)
	require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "old-device-uuid", &firstReq))

	updatedReq := helperNewPriceChangeRequestEventWithSigner(t, deviceSigner, def.Address(), 60, 5, ts+4)
	require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "new-device-uuid", &updatedReq))

	triggerAction := helperNewPriceActionEvent(t, def.Address(), "106.00", ts+5)
	require.NoError(t, db.AcceptEvents(t.Context(), triggerAction))

	candidates, _, err := db.CollectPriceChangeSubscribersCandidates(t.Context(), triggerAction, 0, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Contains(t, candidates, updatedReq.PubKey)

	data, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), triggerAction, []string{updatedReq.PubKey})
	require.NoError(t, err)
	require.Len(t, data, 1)
	require.Equal(t, updatedReq.PubKey, data[0].DevicePubKey)
	require.Equal(t, "new-device-uuid", data[0].DeviceUUID)
	require.Equal(t, baseAction.ID, data[0].PreviousEvent.ID)
	require.JSONEq(t, updatedReq.String(), data[0].Request)

	t.Run("Reconfigure resets notification window", func(t *testing.T) {
		reconfiguredReq := helperNewPriceChangeRequestEventWithSigner(t, deviceSigner, def.Address(), 60, 3, ts+6)
		require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "reconfigured-device-uuid", &reconfiguredReq))

		firstAfterReconfigure := helperNewPriceActionEvent(t, def.Address(), "107.00", ts+7)
		require.NoError(t, db.AcceptEvents(t.Context(), firstAfterReconfigure))

		notifications, err := db.FetchAndUpdatePriceChangeNotification(t.Context(), firstAfterReconfigure, []string{reconfiguredReq.PubKey})
		require.NoError(t, err)
		require.Empty(t, notifications)

		secondAfterReconfigure := helperNewPriceActionEvent(t, def.Address(), "112.00", ts+8)
		require.NoError(t, db.AcceptEvents(t.Context(), secondAfterReconfigure))

		notifications, err = db.FetchAndUpdatePriceChangeNotification(t.Context(), secondAfterReconfigure, []string{reconfiguredReq.PubKey})
		require.NoError(t, err)
		require.Len(t, notifications, 1)
		require.Equal(t, reconfiguredReq.PubKey, notifications[0].DevicePubKey)
		require.Equal(t, "reconfigured-device-uuid", notifications[0].DeviceUUID)
		require.Equal(t, firstAfterReconfigure.ID, notifications[0].PreviousEvent.ID)
		require.JSONEq(t, reconfiguredReq.String(), notifications[0].Request)
	})

	require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), updatedReq.PubKey, "", def.Address(), ""))
}

func TestPriceChangeDeleteSubscriberFilters(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	ts := nostr.Now()

	var post1, post2, def1, def2 model.Event
	post1.Kind = nostr.KindArticle
	post1.Content = "delete filter post 1"
	post1.CreatedAt = ts
	require.NoError(t, post1.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	post2.Kind = nostr.KindArticle
	post2.Content = "delete filter post 2"
	post2.CreatedAt = ts + 1
	require.NoError(t, post2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	def1.Kind = model.CustomIONKindTokenizedCommunityDefinition
	def1.CreatedAt = ts + 2
	def1.Content = "delete filter def 1"
	def1.Tags = model.Tags{{"a", post1.Address()}}
	require.NoError(t, def1.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	def2.Kind = model.CustomIONKindTokenizedCommunityDefinition
	def2.CreatedAt = ts + 3
	def2.Content = "delete filter def 2"
	def2.Tags = model.Tags{{"a", post2.Address()}}
	require.NoError(t, def2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	// Seed latest trade event for each token so registration works with token-scoped requests.
	action1 := helperNewPriceActionEvent(t, def1.Address(), "100.00", ts+4)
	action2 := helperNewPriceActionEvent(t, def2.Address(), "100.00", ts+5)

	require.NoError(t, db.AcceptEvents(t.Context(), &post1, &post2, &def1, &def2, action1, action2))

	deviceSigner := model.GeneratePrivateKey()
	req1 := helperNewPriceChangeRequestEventWithSigner(t, deviceSigner, def1.Address(), 60, 5, ts+6)
	req2 := helperNewPriceChangeRequestEventWithSigner(t, deviceSigner, def2.Address(), 60, 5, ts+7)

	require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "device-uuid-1", &req1))
	require.NoError(t, db.RegisterPriceChangeSubscriber(t.Context(), "device-uuid-2", &req2))

	countByPubKey := func() int64 {
		t.Helper()

		data, err := connector.Get[int64](t.Context(), db.db, `SELECT COUNT(*) AS cnt FROM pn_price_changes WHERE user_device_pubkey = $1`, req1.PubKey)
		require.NoError(t, err)
		require.NotNil(t, data)

		return *data
	}

	require.EqualValues(t, 2, countByPubKey())

	t.Run("Token mismatch does not delete", func(t *testing.T) {
		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req1.PubKey, "", "invalid:token:address", ""))
		require.EqualValues(t, 2, countByPubKey())
	})

	t.Run("EventID mismatch does not delete", func(t *testing.T) {
		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req1.PubKey, "", "", "missing-event-id"))
		require.EqualValues(t, 2, countByPubKey())
	})

	t.Run("Device UUID mismatch does not delete", func(t *testing.T) {
		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req1.PubKey, "missing-device-uuid", "", ""))
		require.EqualValues(t, 2, countByPubKey())
	})

	t.Run("Delete by token removes only one row", func(t *testing.T) {
		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req1.PubKey, "", def1.Address(), ""))
		require.EqualValues(t, 1, countByPubKey())
	})

	t.Run("Delete by device removes remaining rows", func(t *testing.T) {
		require.NoError(t, db.DeletePriceChangeSubscriber(t.Context(), req1.PubKey, "", "", ""))
		require.EqualValues(t, 0, countByPubKey())
	})
}
