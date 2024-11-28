// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jamiealquiza/tachymeter"
	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"
)

var (
	benchdbFTS5Once sync.Once
)

func helperBenchPrepareFts5(b *testing.B, f func() *dbClient) (*dbClient, *tachymeter.Tachymeter) {
	b.Helper()
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.SetParallelism(1)
	db := f()
	b.ResetTimer()
	b.ReportAllocs()

	return db, meter
}

func BenchmarkSQLiteFTS5ExtensionNoPrefixes(b *testing.B) {
	dbPrepare, meter := helperBenchPrepareFts5(b, func() *dbClient {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)

		return openDatabase(path+"?_journal_mode=WAL", false)
	})
	defer dbPrepare.Close()
	searchValues, err := helperSelectRandomSearchValuesNoPrefixes(dbPrepare)
	require.NoError(b, err)
	b.RunParallel(func(pb *testing.PB) {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)

		db := openDatabase(path, false)
		defer db.Close()

		for pb.Next() {
			helperMakeSearch(b, db, searchValues[rand.Intn(len(searchValues))], meter)
		}
	})
	helperBenchReportMetrics(b, dbPrepare, meter)
	b.ReportMetric(meter.Calc().Rate.Second, "ops/sec")
}

func BenchmarkSQLiteFTS5ExtensionStartAndAsteriksPrefixes(b *testing.B) {
	dbPrepare, meter := helperBenchPrepareFts5(b, func() *dbClient {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)

		return openDatabase(path+"?_journal_mode=WAL", false)
	})
	defer dbPrepare.Close()
	searchValues, err := helperSelectRandomSearchValuesWithStartAndEndPrefixes(dbPrepare)
	require.NoError(b, err)
	b.RunParallel(func(pb *testing.PB) {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)
		db := openDatabase(path, false)
		defer db.Close()

		for pb.Next() {
			helperMakeSearch(b, db, searchValues[rand.Intn(len(searchValues))], meter)
		}
	})
	helperBenchReportMetrics(b, dbPrepare, meter)
	b.ReportMetric(meter.Calc().Rate.Second, "ops/sec")
}

func BenchmarkSQLiteFTS5ExtensionOnlyStartPrefix(b *testing.B) {
	dbPrepare, meter := helperBenchPrepareFts5(b, func() *dbClient {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)

		return openDatabase(path+"?_journal_mode=WAL", false)
	})
	defer dbPrepare.Close()
	searchValues, err := helperSelectRandomSearchValuesWithStartPrefix(dbPrepare)
	require.NoError(b, err)
	b.RunParallel(func(pb *testing.PB) {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)
		db := openDatabase(path, false)
		defer db.Close()
		for pb.Next() {
			helperMakeSearch(b, db, searchValues[rand.Intn(len(searchValues))], meter)
		}
	})
	helperBenchReportMetrics(b, dbPrepare, meter)
	b.ReportMetric(meter.Calc().Rate.Second, "ops/sec")
}

func BenchmarkSQLiteFTS5ExtensionWithOr(b *testing.B) {
	dbPrepare, meter := helperBenchPrepareFts5(b, func() *dbClient {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)

		return openDatabase(path+"?_journal_mode=WAL", false)
	})
	defer dbPrepare.Close()
	searchValues, err := helperSelectRandomSearchValuesWithPrefixOR(dbPrepare)
	require.NoError(b, err)
	b.RunParallel(func(pb *testing.PB) {
		path := os.Getenv("TESTDB_FTS5")
		if path == "" {
			b.Skip("TESTDB_FTS5 is not set")
		}
		b.Logf("using source database: %q", path)
		db := openDatabase(path, false)
		defer db.Close()

		for pb.Next() {
			helperMakeSearch(b, db, searchValues[rand.Intn(len(searchValues))], meter)
		}
	})
	helperBenchReportMetrics(b, dbPrepare, meter)
	b.ReportMetric(meter.Calc().Rate.Second, "ops/sec")
}

func helperSelectRandomSearchValues(db *dbClient) (rows *sqlx.Rows, err error) {
	ctx := context.Background()
	sql := "SELECT content FROM events ORDER BY RANDOM() LIMIT 100000"
	stmt, err := db.prepare(ctx, sql, hashSQL(sql))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare query sql: %q", sql)
	}
	params := make(map[string]any)
	rows, err = stmt.QueryxContext(ctx, params)
	if err != nil {
		err = errors.Wrapf(err, "failed to query query events sql: %q", sql)
	}

	return rows, nil
}

func helperSelectRandomSearchValuesWithPrefixOR(db *dbClient) (str []string, err error) {
	rows, err := helperSelectRandomSearchValues(db)
	if err != nil {
		return nil, err
	}
	var contentSlice []string
	for rows.Next() {
		var content string
		if err = rows.Scan(&content); err == nil {
			splitted := strings.Split(content, " ")
			if len(splitted) == 0 {
				continue
			}
			val := splitted[0]
			shortedLen := int(len(val) / 2)
			if shortedLen < 1 {
				continue
			}
			doubleShortedLen := int(len(val[0:shortedLen]) / 2)
			if doubleShortedLen < 1 {
				continue
			}
			val = fmt.Sprintf("^%v* OR %v", val[0:shortedLen], val[0:doubleShortedLen])

			contentSlice = append(contentSlice, val)
		}
	}

	return contentSlice, err
}

func helperSelectRandomSearchValuesWithStartAndEndPrefixes(db *dbClient) (str []string, err error) {
	rows, err := helperSelectRandomSearchValues(db)
	if err != nil {
		return nil, err
	}
	var contentSlice []string
	for rows.Next() {
		var content string
		if err = rows.Scan(&content); err == nil {
			splitted := strings.Split(content, " ")
			if len(splitted) == 0 {
				continue
			}
			val := splitted[0]
			shortedLen := int(len(val) / 2)
			if shortedLen < 1 {
				continue
			}
			val = fmt.Sprintf("^%v*", val[0:shortedLen])

			contentSlice = append(contentSlice, val)
		}
	}

	return contentSlice, err
}

func helperSelectRandomSearchValuesWithStartPrefix(db *dbClient) (str []string, err error) {
	rows, err := helperSelectRandomSearchValues(db)
	if err != nil {
		return nil, err
	}
	var contentSlice []string
	for rows.Next() {
		var content string
		if err = rows.Scan(&content); err == nil {
			splitted := strings.Split(content, " ")
			if len(splitted) == 0 {
				continue
			}

			contentSlice = append(contentSlice, fmt.Sprintf("^%v", splitted[0]))
		}
	}

	return contentSlice, err
}

func helperSelectRandomSearchValuesNoPrefixes(db *dbClient) (str []string, err error) {
	ctx := context.Background()
	sql := "SELECT content FROM events ORDER BY RANDOM() LIMIT 100000"
	stmt, err := db.prepare(ctx, sql, hashSQL(sql))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare query sql: %q", sql)
	}
	params := make(map[string]any)
	rows, err := stmt.QueryxContext(ctx, params)
	if err != nil {
		err = errors.Wrapf(err, "failed to query events sql: %q", sql)
	}
	var contentSlice []string
	for rows.Next() {
		var content string
		if err = rows.Scan(&content); err == nil {
			splitted := strings.Split(content, " ")
			if len(splitted) == 0 {
				continue
			}
			val := splitted[rand.Intn(len(splitted))]
			if val == "" {
				continue
			}
			contentSlice = append(contentSlice, val)
		}
	}

	return contentSlice, err
}

func helperMakeSearch(b *testing.B, db *dbClient, match string, meter *tachymeter.Tachymeter) {
	start := time.Now()
	ctx := context.Background()
	params := make(map[string]interface{}, 1)
	params["match"] = match
	sql := "SELECT * FROM events_fts5_index WHERE events_fts5_index MATCH :match ORDER BY bm25(events_fts5_index) LIMIT 100"
	stmt, err := db.prepare(ctx, sql, hashSQL(sql))
	require.NoError(b, err)

	rows, err := stmt.QueryxContext(ctx, params)
	require.NoError(b, err)

	meter.AddTime(time.Since(start))
	rowsCount := 0
	for rows.Next() {
		rowsCount++
	}
	require.GreaterOrEqual(b, rowsCount, 1)
}

func TestGenerateDataForFile3M_FTS5Extension(t *testing.T) {
	const amount = 100000

	if os.Getenv("GENDB") != "yes" {
		t.Skip("skipping test; to enable, set GENDB=yes")
	}

	dbPath := `.testdata/testdb_3M_fts5.sqlite3`
	if n := os.Getenv("TESTDB_FTS5"); n != "" {
		t.Logf("using custom database path %q from env (TESTDB_FTS5)", n)
		dbPath = n
	}

	t.Logf("generating test database at %q with %d event(s)", dbPath, amount)
	db := openDatabase(dbPath+"?_foreign_keys=on&_journal_mode=off&_synchronous=off", true)
	require.NotNil(t, db)
	defer db.Close()
	helperFillDatabase(t, db, amount, func() int { return nostr.KindTextNote })
}
