package query

import (
	"context"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gookit/goutil/errorx"
	"github.com/jamiealquiza/tachymeter"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

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

	dbPath := os.Getenv("BENCH_DB_PATH")
	if dbPath == "" {
		t.Skip("BENCH_DB_PATH is not set")
	}

	db := openDatabase(dbPath+"?_foreign_keys=on", false)
	benchData.Do(func() {
		t.Logf("loading test data")
		benchData.Events = helperBenchPreloadDataForFilter(t, db)
		t.Logf("loaded %d event(s)", len(benchData.Events))

		db.stmtCacheMx.Lock()
		defer db.stmtCacheMx.Unlock()
		clear(db.stmtCache)
	})

	return db
}

func helperRandomEvent(t interface{ Helper() }) *model.Event {
	t.Helper()

	return benchData.Events[rand.IntN(len(benchData.Events))]
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
	t.ReportMetric(float64(len(db.stmtCache)), "stmt-cache-len")
	db.stmtCacheMx.RUnlock()
}

func helperBenchPreloadDataForFilter(
	t interface {
		Helper()
		require.TestingT
	},
	db *dbClient,
) (events []*model.Event) {
	const stmt = `select
	e.kind,
	e.created_at,
	e.system_created_at,
	e.id,
	e.pubkey,
	e.sig,
	e.content,
	'[]' as tags,
	json_group_array(
		json_array(event_tag_key, event_tag_value1,event_tag_value2,event_tag_value3,event_tag_value4)
	) filter (where event_tag_key != '') as jtags
from
	events e
left join event_tags on
	event_id = id
group by
	id
order by
	random()
limit 1000`

	it := &eventIterator{
		oneShot: true,
		fetch: func(int64) (*sqlx.Rows, error) {
			stmt, err := db.prepare(context.TODO(), stmt, hashSQL(stmt))
			if err != nil {
				return nil, errorx.Withf(err, "failed to prepare query sql: %v", stmt)
			}

			return stmt.QueryxContext(context.TODO(), map[string]any{})
		}}

	err := it.Each(context.TODO(), func(ev *model.Event) error {
		events = append(events, ev)

		return nil
	})
	require.NoError(t, err)
	rand.Shuffle(len(events), func(i, j int) { events[i], events[j] = events[j], events[i] })

	return events
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
			filters[0].IDs[0] = helperRandomEvent(b).ID
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
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
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByCreatedAt(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{}}
		for pb.Next() {
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			filters[0].Until = &helperRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndCreatedAt(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{}}
		for pb.Next() {
			filters[0].Kinds = []int{generateKind()}
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			filters[0].Until = &helperRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].Kinds = []int{generateKind()}
			filters[0].IDs[0] = helperRandomEvent(b).ID
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndAuthor(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Kinds = []int{generateKind()}
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByCreatedAtAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			filters[0].IDs[0] = helperRandomEvent(b).ID
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByCreatedAtAndAuthor(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}}}
		for pb.Next() {
			filters[0].IDs[0] = helperRandomEvent(b).ID
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndCreatedAtAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{IDs: []string{""}}}
		for pb.Next() {
			filters[0].IDs[0] = helperRandomEvent(b).ID
			filters[0].Kinds = []int{generateKind()}
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndCreatedAtAndAuthor(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			filters[0].Kinds = []int{generateKind()}
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			filters[0].IDs[0] = helperRandomEvent(b).ID
			filters[0].Kinds = []int{generateKind()}
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByCreatedAtAndAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}}}
		for pb.Next() {
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			filters[0].IDs[0] = helperRandomEvent(b).ID
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}

func BenchmarkSelectByKindAndCreatedAtAndAuthorAndID(b *testing.B) {
	db, meter := helperBenchPrepare(b)
	b.RunParallel(func(pb *testing.PB) {
		filters := model.Filters{model.Filter{Authors: []string{""}, IDs: []string{""}, Kinds: []int{0}}}
		for pb.Next() {
			filters[0].Kinds[0] = generateKind()
			filters[0].Authors[0] = helperRandomEvent(b).PubKey
			filters[0].IDs[0] = helperRandomEvent(b).ID
			filters[0].Since = &helperRandomEvent(b).CreatedAt
			helperBenchSelectBy(b, db, meter, filters)
		}
	})
	helperBenchReportMetrics(b, db, meter)
}
