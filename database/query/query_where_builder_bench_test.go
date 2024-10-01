// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jamiealquiza/tachymeter"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

const (
	benchParallelism = 100
)

var (
	benchData struct {
		sync.Once
		Events []*model.Event
	}
)

func helperBenchEnsureDatabase(t interface {
	Helper()
	Skip(...any)
	Logf(string, ...any)
	require.TestingT
}) *dbClient {
	t.Helper()

	if os.Getenv("BENCHDB") != "yes" {
		t.Skip("BENCHDB is not set to 'yes'")
	}

	dbPath := os.Getenv("TESTDB")
	if dbPath == "" {
		t.Skip("TESTDB env is not set")
	}

	db := openDatabase(dbPath+"?_foreign_keys=on", false)
	benchData.Do(func() {
		t.Logf("loading test data from %q", dbPath)
		benchData.Events = helperPreloadDataForFilter(t, db)
		t.Logf("loaded %d event(s)", len(benchData.Events))

		db.stmtCacheMx.Lock()
		defer db.stmtCacheMx.Unlock()
		clear(db.stmtCache)
	})

	return db
}

func helperBenchRandomEvent(t interface{ Helper() }) *model.Event {
	t.Helper()

	return benchData.Events[rand.Int31n(int32(len(benchData.Events)))]
}

func helperBenchSelectBy(t interface{ Helper() }, db *dbClient, meter *tachymeter.Tachymeter, filters []model.Filter) {
	t.Helper()

	const limiter = 2500
	if len(filters) > 0 {
		filters[0].Limit = limiter
	}

	start := time.Now()
	for ev, err := range db.SelectEvents(context.TODO(), &model.Subscription{Filters: filters}) {
		_, _ = ev, err
	}
	meter.AddTime(time.Since(start))
}

func helperBenchReportMetrics(
	t interface {
		Helper()
		ReportMetric(float64, string)
	},
	db *dbClient,
	meter *tachymeter.Tachymeter,
) {
	t.Helper()

	metric := meter.Calc()
	t.ReportMetric(float64(metric.Time.Avg.Milliseconds()), "avg-ms/op")
	t.ReportMetric(float64(metric.Time.StdDev.Milliseconds()), "stddev-ms/op")
	t.ReportMetric(float64(metric.Time.P50.Milliseconds()), "p50-ms/op")
	t.ReportMetric(float64(metric.Time.P95.Milliseconds()), "p95-ms/op")

	db.stmtCacheMx.RLock()
	t.ReportMetric(float64(len(db.stmtCache)), "stmt-cache-size")
	db.stmtCacheMx.RUnlock()
}

func helperBenchPrepare(b *testing.B) (*dbClient, *tachymeter.Tachymeter) {
	b.Helper()

	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(benchParallelism)

	return helperBenchEnsureDatabase(b), meter
}

func BenchmarkSelectByKind(b *testing.B) {
	db := helperBenchEnsureDatabase(b)
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(benchParallelism)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Kinds: []int{0}}}
		for pb.Next() {
			filters[0].Kinds[0] = generateKind()
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByID(b *testing.B) {
	db := helperBenchEnsureDatabase(b)
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(benchParallelism)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByAuthor(b *testing.B) {
	db := helperBenchEnsureDatabase(b)
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(benchParallelism)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func helperBenchEnsureValidRange(t interface{ Helper() }, f *model.Filter) {
	t.Helper()

	if *f.Since > *f.Until {
		f.Since, f.Until = f.Until, f.Since
	}
}

func BenchmarkSelectByCreatedAtRange(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{}}
		for pb.Next() {
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			filters[0].Until = &helperBenchRandomEvent(b).CreatedAt
			helperBenchEnsureValidRange(b, &filters[0])
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndCreatedAtRange(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{}}
		for pb.Next() {
			filters[0].Kinds = []int{generateKind()}
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			filters[0].Until = &helperBenchRandomEvent(b).CreatedAt
			helperBenchEnsureValidRange(b, &filters[0])
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].Kinds = []int{generateKind()}
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndAuthor(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Kinds = []int{generateKind()}
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByCreatedAtAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByCreatedAtAndAuthor(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}}}
		for pb.Next() {
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndCreatedAtAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			filters[0].Kinds = []int{generateKind()}
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndCreatedAtAndAuthor(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			filters[0].Kinds = []int{generateKind()}
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			filters[0].Kinds = []int{generateKind()}
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByCreatedAtAndAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}

func BenchmarkSelectByKindAndCreatedAtAndAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}, Kinds: []int{0}}}
		for pb.Next() {
			filters[0].Kinds[0] = generateKind()
			filters[0].Authors[0] = helperBenchRandomEvent(b).PubKey
			filters[0].IDs[0] = helperBenchRandomEvent(b).ID
			filters[0].Since = &helperBenchRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}
