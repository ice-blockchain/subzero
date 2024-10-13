// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubZeroEventReorder(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var result string
	err := db.QueryRowContext(context.Background(), `SELECT subzero_nostr_tag_reorder('["imeta", "foo", "bar", "", "m media", "url http://example.com"]')`).
		Scan(&result)
	require.NoError(t, err)
	require.Equal(t, `["imeta","url http://example.com","m media","","bar","foo"]`, result)
}

func TestSubZeroOnBehalfAllowed(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var result bool
	err := db.QueryRowContext(context.Background(), `SELECT subzero_nostr_onbehalf_is_allowed('[["p", "pub", "", "active:1"]]', 'pub', 'masterpub', 0, 2)`).
		Scan(&result)
	require.NoError(t, err)
	require.True(t, result)
}
