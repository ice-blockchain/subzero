// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jamiealquiza/tachymeter"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

func helperBenchmarkEventEnsureDatabase(t interface {
	Helper()
	Skip(...any)
	Logf(string, ...any)
	require.TestingT
}) *dbClient {
	t.Helper()

	if os.Getenv("BENCHDB") != "yes" {
		t.Skip("BENCHDB is not set to 'yes'")
	}

	return openDatabase(":memory:?_foreign_keys=on", true)
}

func helperBenchmarkEventPrepare(b *testing.B) (*dbClient, *tachymeter.Tachymeter) {
	b.Helper()

	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(benchParallelism)

	return helperBenchmarkEventEnsureDatabase(b), meter
}

func BenchmarkEventInsert(b *testing.B) {
	db, meter := helperBenchmarkEventPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var ev model.Event
			ev.ID = generateHexString()
			ev.PubKey = generateHexString()
			ev.CreatedAt = model.Timestamp(generateCreatedAt())
			ev.Kind = generateKind()
			ev.Content = generateRandomString(rand.Intn(1024))
			ev.Tags = []model.Tag{
				{"e", generateHexString()},
				{"p", generateHexString()},
				{"d", generateHexString(), generateRandomString(rand.Intn(10))},
			}
			start := time.Now()
			db.AcceptEvents(context.Background(), &ev)
			meter.AddTime(time.Since(start))
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}
