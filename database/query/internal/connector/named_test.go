// SPDX-License-Identifier: ice License 1.0

package connector_test

import (
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/database/query/internal/connector"
)

func TestNamedSelect(t *testing.T) {
	t.Parallel()

	addr, release := mainTestContainer.MustTempDB(t.Context())
	defer release()

	schema := fstest.MapFS{
		"001_create_table.sql": {
			Data: []byte(`
			CREATE TABLE IF NOT EXISTS testnamed (id SERIAL PRIMARY KEY, name TEXT);
		`),
		},
	}

	ctx, cancel := appcontext.NewAppContext(t.Context())
	defer cancel()

	conn, err := connector.New(ctx,
		connector.WithWriteURLs(addr),
		connector.WithDDL(&schema),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)

	defer conn.Close()

	type entry struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	t.Run("Generate data", func(t *testing.T) {
		rows, err := connector.Exec(t.Context(), conn, `
			INSERT INTO testnamed (name)
			SELECT 'User ' || i || ' ' || md5(random()::text)
			FROM generate_series(1, 1000) AS i
		`)
		require.NoError(t, err)
		require.EqualValues(t, 1000, rows)
	})
	t.Run("Select named", func(t *testing.T) {
		const stmt = `SELECT id, name FROM testnamed WHERE id in (:id1, :id2, :id3) ORDER BY id`
		params := map[string]any{
			"id1": 1,
			"id2": 20,
			"id3": 42,
		}

		data, err := connector.SelectNamed[entry](t.Context(), conn, stmt, params)
		require.NoError(t, err)
		for _, d := range data {
			t.Logf("ID: %d, Name: %s", d.ID, d.Name)
		}
		require.Len(t, data, 3)
		require.Equal(t, 1, data[0].ID)
		require.Equal(t, 20, data[1].ID)
		require.Equal(t, 42, data[2].ID)
	})
	t.Run("Select named iterator", func(t *testing.T) {
		selectData := func(t *testing.T, limit int) (data []*entry) {
			t.Helper()

			stmt := `SELECT id, name FROM testnamed ORDER BY id`

			it, err := connector.SelectNamedIterator[entry](t.Context(), conn, stmt, map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, it)

			for e, err := range it {
				require.NoError(t, err)
				require.NotNil(t, e)

				data = append(data, e)
				if limit > 0 && len(data) >= limit {
					break
				}
			}
			return data
		}
		t.Run("Limit", func(t *testing.T) {
			for _, limit := range []int{1, 10, 15, 100, 125, 200, 222, 300} {
				t.Run(strconv.Itoa(limit), func(t *testing.T) {
					records := selectData(t, limit)
					t.Logf("fetched %d records(s)", len(records))
					require.Len(t, records, limit)
				})
			}
		})
		t.Run("All", func(t *testing.T) {
			records := selectData(t, -1)
			t.Logf("fetched %d record(s)", len(records))
			require.Len(t, records, 1000)
		})
	})
}
