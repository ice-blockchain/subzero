// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestTokenActivityFlow(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	baseTime := nostr.Now().Add(-time.Hour * 48)

	t.Run("No definition event", func(t *testing.T) {
		buyEvent := helperNewPriceActionEvent(t, "31175:foo:bar", "10", baseTime.Add(time.Second*100))
		require.NoError(t, db.AcceptEvents(t.Context(), buyEvent))

		candidates, _, err := db.CollectTokenActivityCandidates(t.Context(), time.Now(), 0, 100)
		require.NoError(t, err)
		require.Empty(t, candidates)
	})

	t.Run("Regular flow with buy and sell events", func(t *testing.T) {
		var post, def model.Event
		post.Kind = nostr.KindArticle
		post.Content = "test post for token"
		post.CreatedAt = baseTime
		require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = baseTime + 1
		def.Content = "token definition"
		def.Tags = model.Tags{
			{"a", post.Address()},
		}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &post, &def))

		// Create 35 buy events spread over 1 hour to exceed threshold (30).
		var firstBuyEvent *model.Event
		const buyCount = 35
		for i := range buyCount {
			eventTime := baseTime.Add(time.Hour).Add(time.Second * time.Duration(i))
			buyEvent := helperNewPriceActionEvent(t, def.Address(), "10", eventTime, "buy")
			require.NoError(t, db.AcceptEvents(t.Context(), buyEvent))
			if i == 0 {
				firstBuyEvent = buyEvent
			}
		}

		const sellCount = 5
		for i := range sellCount {
			eventTime := baseTime.Add(time.Hour * 2).Add(time.Second * time.Duration(i))
			sellEvent := helperNewPriceActionEvent(t, def.Address(), "9", eventTime, "sell")
			require.NoError(t, db.AcceptEvents(t.Context(), sellEvent)) // Should be ignored in the trigger.
		}

		now := time.Now()
		candidates, lastID, err := db.CollectTokenActivityCandidates(t.Context(), now, 0, 100)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Equal(t, def.Address(), candidates[0])
		require.Greater(t, lastID, uint64(0))

		data, err := db.FetchAndUpdateTokenActivityNotification(t.Context(), now, candidates)
		require.NoError(t, err)
		require.Len(t, data, 1)

		notification := data[0]
		require.EqualValues(t, buyCount, notification.Counter)
		require.NotEmpty(t, notification.Definition)
		require.NotEmpty(t, notification.FirstBuy)

		var notifiedFirstBuy, notifiedDef model.Event
		require.NoError(t, notifiedFirstBuy.UnmarshalJSON([]byte(notification.FirstBuy)))
		require.Equal(t, firstBuyEvent, &notifiedFirstBuy)

		require.NoError(t, notifiedDef.UnmarshalJSON([]byte(notification.Definition)))
		require.Equal(t, def, notifiedDef)

		candidates2, _, err := db.CollectTokenActivityCandidates(t.Context(), now, 0, 100)
		require.NoError(t, err)
		require.Empty(t, candidates2)
	})
}

func TestTokenActivityFlowEdgeCases(t *testing.T) {
	t.Parallel()

	const buyCountThreshold = 30

	db := helperNewDatabase(t)
	defer db.Close()

	var post model.Event
	post.Kind = nostr.KindArticle
	post.Content = "under threshold post"
	post.CreatedAt = nostr.Now().Add(-time.Hour * 100)
	require.NoError(t, post.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &post))

	t.Run("Under threshold is not collected", func(t *testing.T) {
		baseTime := nostr.Now().Add(-time.Hour * 48)

		var def model.Event
		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = baseTime + 1
		def.Content = "under threshold def"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &def))

		for i := range buyCountThreshold - 1 {
			eventTime := baseTime.Add(time.Hour).Add(time.Second * time.Duration(i))
			require.NoError(t, db.AcceptEvents(t.Context(), helperNewPriceActionEvent(t, def.Address(), "10", eventTime, "buy")))
		}

		candidates, _, err := db.CollectTokenActivityCandidates(t.Context(), time.Now(), 0, 100)
		require.NoError(t, err)
		require.Empty(t, candidates)
	})

	t.Run("Old events are ignored", func(t *testing.T) {
		baseTime := nostr.Now().Add(-time.Hour * 48)

		var def model.Event
		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = baseTime + 1
		def.Content = "old events def"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &def))

		for i := range buyCountThreshold {
			eventTime := baseTime.Add(time.Hour).Add(time.Second * time.Duration(i))
			buyEvent := helperNewPriceActionEvent(t, def.Address(), "10", eventTime, "buy")
			require.NoError(t, db.AcceptEvents(t.Context(), buyEvent))
		}

		oldBuyEvent := helperNewPriceActionEvent(t, def.Address(), "11", baseTime.Add(time.Minute*30), "buy")
		require.NoError(t, db.AcceptEvents(t.Context(), oldBuyEvent))

		now := time.Now()
		candidates, _, err := db.CollectTokenActivityCandidates(t.Context(), now, 0, 100)
		require.NoError(t, err)
		require.Equal(t, []string{def.Address()}, candidates)

		data, err := db.FetchAndUpdateTokenActivityNotification(t.Context(), now, candidates)
		require.NoError(t, err)
		require.Len(t, data, 1)
		require.EqualValues(t, buyCountThreshold, data[0].Counter)
	})

	t.Run("Window switching after notification", func(t *testing.T) {
		baseTime := nostr.Now().Add(-time.Hour * 96)

		var def model.Event
		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = baseTime + 1
		def.Content = "window switching def"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &def))

		for i := range buyCountThreshold {
			eventTime := baseTime.Add(time.Hour).Add(time.Second * time.Duration(i))
			require.NoError(t, db.AcceptEvents(t.Context(), helperNewPriceActionEvent(t, def.Address(), "10", eventTime, "buy")))
		}

		firstNow := baseTime.Add(time.Hour * 26)
		candidates, _, err := db.CollectTokenActivityCandidates(t.Context(), firstNow.Time(), 0, 100)
		require.NoError(t, err)
		require.Equal(t, []string{def.Address()}, candidates)

		data, err := db.FetchAndUpdateTokenActivityNotification(t.Context(), firstNow.Time(), candidates)
		require.NoError(t, err)
		require.Len(t, data, 1)
		require.EqualValues(t, buyCountThreshold, data[0].Counter)

		for i := range buyCountThreshold {
			eventTime := baseTime.Add(time.Hour * 28).Add(time.Second * time.Duration(i))
			require.NoError(t, db.AcceptEvents(t.Context(), helperNewPriceActionEvent(t, def.Address(), "12", eventTime, "buy")))
		}

		secondNow := baseTime.Add(time.Hour * 54)
		candidates, _, err = db.CollectTokenActivityCandidates(t.Context(), secondNow.Time(), 0, 100)
		require.NoError(t, err)
		require.Equal(t, []string{def.Address()}, candidates)

		data, err = db.FetchAndUpdateTokenActivityNotification(t.Context(), secondNow.Time(), candidates)
		require.NoError(t, err)
		require.Len(t, data, 1)
		require.EqualValues(t, buyCountThreshold, data[0].Counter)
	})

	t.Run("Pending processing sees overflow event before fetch", func(t *testing.T) {
		baseTime := nostr.Now().Add(-time.Hour * 96)

		var def model.Event
		def.Kind = model.CustomIONKindTokenizedCommunityDefinition
		def.CreatedAt = baseTime + 1
		def.Content = "pending processing def"
		def.Tags = model.Tags{{"a", post.Address()}}
		require.NoError(t, def.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &def))

		for i := range buyCountThreshold {
			eventTime := baseTime.Add(time.Hour).Add(time.Second * time.Duration(i))
			require.NoError(t, db.AcceptEvents(t.Context(), helperNewPriceActionEvent(t, def.Address(), "10", eventTime, "buy")))
		}

		collectNow := baseTime.Add(time.Hour * 26)
		candidates, _, err := db.CollectTokenActivityCandidates(t.Context(), collectNow.Time(), 0, 100)
		require.NoError(t, err)
		require.Equal(t, []string{def.Address()}, candidates)

		pendingEvent := helperNewPriceActionEvent(t, def.Address(), "15", baseTime.Add(time.Hour*25).Add(time.Minute*2).Add(time.Second*5), "buy")
		require.NoError(t, db.AcceptEvents(t.Context(), pendingEvent))

		data, err := db.FetchAndUpdateTokenActivityNotification(t.Context(), collectNow.Time(), candidates)
		require.NoError(t, err)
		require.Len(t, data, 1)
		require.EqualValues(t, buyCountThreshold+1, data[0].Counter)

		candidates, _, err = db.CollectTokenActivityCandidates(t.Context(), collectNow.Time(), 0, 100)
		require.NoError(t, err)
		require.Empty(t, candidates)
	})
}
