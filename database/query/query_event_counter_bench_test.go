// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperBenchmarkEventCounterPrepareDatabase(b *testing.B) *dbClient {
	b.Helper()

	db := openDatabase("file::memory:?cache=shared", true)

	var ev model.Event
	ev.ID = "1"
	ev.Kind = nostr.KindTextNote
	ev.PubKey = "pubkey1"
	ev.CreatedAt = 1
	ev.Content = "content"
	require.NoError(b, db.AcceptEvents(context.Background(), &ev))

	for i := range 3 {
		var q model.Event
		q.Kind = nostr.KindTextNote
		q.ID = "q" + strconv.Itoa(i)
		q.PubKey = "pubkey" + strconv.Itoa(i)
		q.CreatedAt = model.Timestamp(i)
		q.Tags = model.Tags{{"q", "1"}}
		require.NoError(b, db.AcceptEvents(context.Background(), &q))
	}

	return db
}

func helperBenchmarkEventCounterExecute(b *testing.B, op string) {
	b.Helper()

	where, params, err := newWhereBuilder().BuildForPrecalculatedCounters(model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{"q": nil}, IDs: []string{"1"}})
	require.NoError(b, err)

	sql := `select ` + op + ` from event_counters where ` + where
	b.Log("SQL:", sql)

	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchmarkEventCounterPrepareDatabase(b) })
	stmt, err := db.PrepareNamed(sql)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var counter int64
			start := time.Now()
			err = stmt.QueryRowx(params).Scan(&counter)
			meter.AddTime(time.Since(start))
			require.NoError(b, err)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkEventCounterSelect(b *testing.B) {
	helperBenchmarkEventCounterExecute(b, `value`)
}

func BenchmarkEventCounterSum(b *testing.B) {
	helperBenchmarkEventCounterExecute(b, `coalesce(sum(value), 0)`)
}
