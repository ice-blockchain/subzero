// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jamiealquiza/tachymeter"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
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
	Context() context.Context
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

	db := openDatabase(appcontext.TestContext(t), []string{dbPath + "?_foreign_keys=on"}, []string{}, false)
	benchData.Do(func() {
		t.Logf("loading test data from %q", dbPath)
		benchData.Events = helperPreloadDataForFilter(t, db)
		t.Logf("loaded %d event(s)", len(benchData.Events))
	})

	return db
}

func helperBenchRandomEvent(t interface{ Helper() }) *model.Event {
	t.Helper()

	return benchData.Events[rand.Int32N(int32(len(benchData.Events)))]
}

func helperBenchSelectBy(t interface{ Helper() }, db *dbClient, meter *tachymeter.Tachymeter, filters []model.Filter) {
	t.Helper()

	start := time.Now()
	for ev, err := range db.SelectEvents(context.TODO(), filters...) {
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
	t.ReportMetric(float64(metric.Time.Max.Milliseconds()), "max-ms/op")
	t.ReportMetric(float64(metric.Time.Min.Milliseconds()), "min-ms/op")
}

func helperBenchPrepare(b *testing.B, f func() *dbClient) (*dbClient, *tachymeter.Tachymeter) {
	b.Helper()

	parallelism := benchParallelism
	if v := os.Getenv("BENCHDB_PARALLELISM"); v != "" {
		x, err := strconv.ParseInt(v, 10, 64)
		require.NoError(b, err)
		parallelism = int(x)
	}

	meter := tachymeter.New(&tachymeter.Config{Size: b.N})

	b.SetParallelism(parallelism)

	db := f()
	b.ResetTimer()
	b.ReportAllocs()

	return db, meter
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

	if f.Since == nil || f.Until == nil {
		return
	}

	if *f.Since > *f.Until {
		f.Since, f.Until = f.Until, f.Since
	}
}

func BenchmarkSelectByCreatedAtRange(b *testing.B) {
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}, Authors: []string{""}}}
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}, Authors: []string{""}}}
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}, Authors: []string{""}}}
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
	db, meter := helperBenchPrepare(b, func() *dbClient { return helperBenchEnsureDatabase(b) })
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}, Authors: []string{""}, Kinds: []int{0}}}
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
