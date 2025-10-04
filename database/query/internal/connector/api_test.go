// SPDX-License-Identifier: ice License 1.0

package connector_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/database/query/internal/postgres/fixture"
)

var (
	mainTestContainer *fixture.Container
)

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

func TestAPI(t *testing.T) {
	t.Parallel()

	schema := fstest.MapFS{
		"001_create_table.sql": {
			Data: []byte(`
			CREATE TABLE IF NOT EXISTS test (id SERIAL PRIMARY KEY, name TEXT);
		`),
		},
	}

	addr, release := mainTestContainer.MustTempDB(t.Context())
	defer release()

	conn, err := connector.New(t.Context(),
		connector.WithWriteURLs(addr),
		connector.WithDDL(&schema),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)

	t.Run("Tx", func(t *testing.T) {
		err := connector.DoInTransaction(t.Context(), conn, func(tx connector.QueryExecer) error {
			_, err = connector.Get[bool](t.Context(), tx, `SELECT true`)
			return err
		})
		require.NoError(t, err)
	})
	t.Run("Ping", func(t *testing.T) {
		err := conn.Ping(t.Context())
		require.NoError(t, err)
	})
	t.Run("Exec", func(t *testing.T) {
		const stmt = `INSERT INTO test (name) VALUES ($1)`

		r, err := connector.Exec(t.Context(), conn, stmt, "test1")
		require.NoError(t, err)
		require.EqualValues(t, 1, r)
	})
	t.Run("Get", func(t *testing.T) {
		const stmt = `SELECT name FROM test WHERE id = $1`

		r, err := connector.Get[string](t.Context(), conn, stmt, 1)
		require.NoError(t, err)
		require.NotNil(t, r)
		require.EqualValues(t, "test1", *r)
	})

	require.NoError(t, conn.Close())
}
