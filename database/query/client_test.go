// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"testing"

	postgres "github.com/ice-blockchain/subzero/database/query/internal/postgres"
	"github.com/stretchr/testify/require"
)

func TestSubZeroEventReorder(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	// TODO: fixme.
	// result, err := postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta", "foo", "bar", "", "m media", "url http://example.com"]]'::JSONB)`)
	// require.NoError(t, err)
	// require.Equal(t, `[["imeta", "url http://example.com", "m media", "foo", "bar", ""]]`, *result[0])

	result, err := postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["e", "event_ref", "", "", "reply"],["a", "action_ref", "", "", "root"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["e", "event_ref", "", "", "reply"], ["a", "action_ref", "", "", "root"]]`, *result[0])

	result, err = postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["e", "event_ref", "", "reply"],["a", "action_ref", "", "root"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["e", "event_ref", "", "reply"], ["a", "action_ref", "", "root", "", "reply_of_root"]]`, *result[0])

	result, err = postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta", "m media"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["imeta", "", "m media"]]`, *result[0])

	result, err = postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta",  "foo", "bar", "", "m media"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["imeta", "foo", "m media", "bar", ""]]`, *result[0])

	result, err = postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta", "bar", "", "url http://example.com", "foo"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["imeta", "url http://example.com", "bar", "", "foo"]]`, *result[0])
}

func TestSubZeroGetEventAddress(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_get_event_address('event1', 30023, 'pubkey1', 'tag1');`)
	require.NoError(t, err)
	require.Equal(t, `30023:pubkey1:tag1`, *result[0])

	result, err = postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_get_event_address('event2', 10100, 'pubkey2', NULL);`)
	require.NoError(t, err)
	require.Equal(t, `10100:pubkey2:`, *result[0])

	result, err = postgres.Select[string](context.Background(), db.dbPostgres, `SELECT subzero_nostr_get_event_address('event3', 1, 'pubkey3', 'tag3');`)
	require.NoError(t, err)
	require.Equal(t, `event3`, *result[0])
}

func TestSubZeroOnBehalfAllowed(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pub", "", "active:1672531200"]]'::JSONB, 'pub', 0, EXTRACT(EPOCH FROM CURRENT_TIMESTAMP))`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey2", "", "active:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3"]]'::JSONB,
		'pubkey1',
		2,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3"]]'::JSONB,
		'pubkey1',
		3,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "revoked:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "1672531200", "active:1672531200"], ["p", "pubkey1", "1675209600", "inactive:1675209600"]]'::JSONB,
		'pubkey1',
		1,
		1674000000 -- Unix-время между start и end.
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "1672531200", "active:1672531200"], ["p", "pubkey1", "1675209600", "inactive:1675209600"]]'::JSONB,
		'pubkey1',
		1,
		1676000000 -- Unix-время после окончания разрешения.
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		1676000000 -- Unix-время после активации разрешения.
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	// TODO: fixme
	// result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
	// 	'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
	// 	'pubkey1',
	// 	31750, -- CustomIONKindAttestation
	// 	EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	// );`)
	// require.NoError(t, err)
	// require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"], ["p", "pubkey2", "", "active:1672531200"]]'::JSONB,
		'pubkey2',
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3,6"]]'::JSONB,
		'pubkey1',
		6,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3"]]'::JSONB,
		'pubkey1',
		6,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "1672531200", "active:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		1671000000 -- Unix-время до активации разрешения.
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "1672531200", "active:1672531200"], ["p", "pubkey1", "1675209600", "inactive:1675209600"]]'::JSONB,
		'pubkey1',
		1,
		1676000000 -- Unix-время после окончания разрешения.
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	// TODO: fixme
	// result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
	// 	'[["p", "pubkey1", "1672531200", "active:1672531200"], ["p", "pubkey1", "1675209600", "inactive:1675209600"]]'::JSONB,
	// 	'pubkey1',
	// 	1,
	// 	1672531200 -- Unix-время точно на момент активации.
	// );`)
	// require.NoError(t, err)
	// require.True(t, *result)

	result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "1672531200", "active:1672531200"], ["p", "pubkey1", "1675209600", "inactive:1675209600"]]'::JSONB,
		'pubkey1',
		1,
		1675209600 -- Unix-время точно на момент окончания действия.
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	// TODO: fixme
	// result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
	// 	'[]'::JSONB,
	// 	'master_pubkey1',
	// 	1,
	// 	EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	// );`)
	// require.NoError(t, err)
	// require.True(t, *result)

	// result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
	// 	'[]'::JSONB,
	// 	'pubkey1',
	// 	1,
	// 	EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	// );`)
	// require.NoError(t, err)
	// require.True(t, *result)

	// result, err = postgres.Get[bool](context.Background(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
	// 	'[["p", "pubkey1", "", "invalid_string"]]'::JSONB,
	// 	'pubkey1',
	// 	1,
	// 	EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
	// );`)
	// require.NoError(t, err)
	// require.False(t, *result)
}
