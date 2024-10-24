// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

var (
	benchdbInsertOnce sync.Once
)

func helperLoadDatabaseIntoMemory(dst, src *sql.DB) error {
	destConn, err := dst.Conn(context.Background())
	if err != nil {
		return err
	}

	srcConn, err := src.Conn(context.Background())
	if err != nil {
		return err
	}

	return destConn.Raw(func(destConn interface{}) error {
		return srcConn.Raw(func(srcConn interface{}) error {
			destSQLiteConn, ok := destConn.(*sqlite3.SQLiteConn)
			if !ok {
				return errors.Errorf("can't convert destination connection to SQLiteConn")
			}

			srcSQLiteConn, ok := srcConn.(*sqlite3.SQLiteConn)
			if !ok {
				return errors.Errorf("can't convert source connection to SQLiteConn")
			}

			b, err := destSQLiteConn.Backup("main", srcSQLiteConn, "main")
			if err != nil {
				return errors.Wrap(err, "error initializing SQLite backup")
			}

			done, err := b.Step(-1)
			if !done {
				return errors.Errorf("step of -1, but not done")
			}
			if err != nil {
				return errors.Wrap(err, "error stepping backup")
			}

			err = b.Finish()
			if err != nil {
				return errors.Wrap(err, "error finishing backup")
			}

			return err
		})
	})
}

func BenchmarkEventInsert(b *testing.B) {
	db, meter := helperBenchPrepare(b, func() *dbClient {
		benchdbInsertOnce.Do(func() {
			path := os.Getenv("TESTDB")
			if path == "" {
				b.Skip("TESTDB is not set")
			}
			b.Logf("using source database: %q", path)

			diskdb := openDatabase(path, false)
			require.NotNil(b, diskdb)
			defer diskdb.Close()

			memdb := openDatabase("file::memory:?cache=shared", false)
			require.NotNil(b, memdb)
			defer memdb.Close()

			b.Log("loading database into memory ...")
			err := helperLoadDatabaseIntoMemory(memdb.DB.DB, diskdb.DB.DB)
			require.NoError(b, err)
			b.Log("in-memory database ready")
		})

		return openDatabase("file::memory:?cache=shared", false)
	})
	b.ResetTimer()
	b.ReportAllocs()

	var counter atomic.Uint32
	benchStart := time.Now()
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
			counter.Add(1)
		}
	})
	b.ReportMetric(float64(counter.Load())/time.Since(benchStart).Seconds(), "ops/sec")
	helperBenchReportMetrics(b, db, meter)
	db.Close()
}
