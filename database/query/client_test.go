// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"

	postgres "github.com/ice-blockchain/subzero/database/query/internal/postgres"
	"github.com/stretchr/testify/require"
)

func TestSubZeroEventReorder(t *testing.T) {
	db := helperNewDatabase(t)
	defer db.Close()

	// TODO: fixme.
	// result, err := postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta", "foo", "bar", "", "m media", "url http://example.com"]]'::JSONB)`)
	// require.NoError(t, err)
	// require.Equal(t, `[["imeta", "url http://example.com", "m media", "foo", "bar", ""]]`, *result[0])

	result, err := postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["e", "event_ref", "", "", "reply"],["a", "action_ref", "", "", "root"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["e", "event_ref", "", "", "reply"], ["a", "action_ref", "", "", "root"]]`, *result[0])

	result, err = postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["e", "event_ref", "", "reply"],["a", "action_ref", "", "root"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["e", "event_ref", "", "reply"], ["a", "action_ref", "", "root", "", "reply_of_root"]]`, *result[0])

	result, err = postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta", "m media"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["imeta", "", "m media"]]`, *result[0])

	result, err = postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta",  "foo", "bar", "", "m media"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["imeta", "foo", "m media", "bar", ""]]`, *result[0])

	result, err = postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_tags_reorder('[["imeta", "bar", "", "url http://example.com", "foo"]]'::JSONB)`)
	require.NoError(t, err)
	require.Equal(t, `[["imeta", "url http://example.com", "bar", "", "foo"]]`, *result[0])
}

func TestSubZeroGetEventAddress(t *testing.T) {
	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_get_event_address('event1', 30023, 'pubkey1', 'tag1');`)
	require.NoError(t, err)
	require.Equal(t, `30023:pubkey1:tag1`, *result[0])

	result, err = postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_get_event_address('event2', 10100, 'pubkey2', NULL);`)
	require.NoError(t, err)
	require.Equal(t, `10100:pubkey2:`, *result[0])

	result, err = postgres.Select[string](t.Context(), db.dbPostgres, `SELECT subzero_nostr_get_event_address('event3', 1, 'pubkey3', 'tag3');`)
	require.NoError(t, err)
	require.Equal(t, `event3`, *result[0])
}

func TestParseAttestationString(t *testing.T) {
	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_string('login:1622550000:1,2,3')::text`)
	require.NoError(t, err)
	require.Equal(t, `(login,"2021-06-01 12:20:00","{1,2,3}")`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_string('action:2622550000:1')::text`)
	require.NoError(t, err)
	require.Equal(t, `(action,"2053-02-07 14:06:40",{1})`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_string('action:2622550000')::text`)
	require.NoError(t, err)
	require.Equal(t, `(action,"2053-02-07 14:06:40",)`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_string('action:0')::text`)
	require.NoError(t, err)
	require.Equal(t, `(action,"1970-01-01 00:00:00",)`, *result)
}

func TestParseAttestationTags(t *testing.T) {
	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","pub1","wss://relay","active:1717041600:1,2"],
		["p","pub1","","inactive:1717128000"],
		["p","pub1","","revoked:1717150000"]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{"pub1": {"end": "2024-05-31T04:00:00", "kinds": [1, 2], "start": "2024-05-30T04:00:00", "revoked": "2024-05-31T10:06:40"}}`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","pubA","","active:1717000000:3,4"],
		["p","pubB","","active:1717000001:5"],
		["p","pubA","","inactive:1717000002"]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{"pubA": {"end": "2024-05-29T16:26:42", "kinds": [3, 4], "start": "2024-05-29T16:26:40"}, "pubB": {"end": null, "kinds": [5], "start": "2024-05-29T16:26:41"}}`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","pubErr","","active:invalid_ts:1"],
		["p","pubErr","","active:999999999999999999999:1"]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{}`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","test1","","active:1717041600:100"]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{"test1": {"end": null, "kinds": [100], "start": "2024-05-30T04:00:00"}}`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","test2","","revoked:1717041600"]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{"test2": {"revoked": "2024-05-30T04:00:00"}}`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","test3","","active:1717041600"],
	    ["p","test3","","inactive:1717041700"]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{"test3": {"end": "2024-05-30T04:01:40", "start": "2024-05-30T04:00:00"}}`, *result)

	result, err = postgres.Get[string](t.Context(), db.dbPostgres, `SELECT parse_attestation_tags('[
		["p","key1",""]
	]'::jsonb)::text AS result;`)
	require.NoError(t, err)
	require.Equal(t, `{}`, *result)
}

func TestSubZeroAttestationUpdateIsAllowed(t *testing.T) {
	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
        '["p1", "p2"]'::JSONB,
        '["p1"]'::JSONB
    )`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
	    '[["p","key1","","revoked:123"]]'::JSONB,
	    '[["p","key1","","modified:456"]]'::JSONB
	)`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
	    '[["p","key1","","revoked:123"]]'::JSONB,
	    '[["p","key1","","revoked:123"], ["p","key1","","new_action:456"]]'::JSONB
	)`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
	    '[["p","key1","","action:123"]]'::JSONB,
	    '[["p","key1","","action:123"], ["p","key2","","new_action:456"]]'::JSONB
	)`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
	    '[["p","key1","","action:123"], ["p","key2","","new_action:456"]]'::JSONB,
	    '[["p","key1","","action:123"]]'::JSONB
	)`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
	    '[]'::JSONB,
	    '[["p","key1",""]]'::JSONB
	)`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_attestation_update_is_allowed(
	    '[["p","pub1","","revoked:1672531200"]]'::jsonb,
    	'[["p","pub1","","revoked:1672531200"],["p","pub2","","active:1672531200"]]'::jsonb
	)`)
	require.NoError(t, err)
	require.True(t, *result)
}

func TestSubZeroOnBehalfAllowed(t *testing.T) {
	db := helperNewDatabase(t)
	defer db.Close()

	result, err := postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[
			["p","test1","","active:1741090232:100,200"]
		]'::jsonb,
        'test1'::text,
        100,
        1741090233)`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey2", "", "active:1672531200"]]'::JSONB,
		'pubkey1'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
		'pubkey1'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3"]]'::JSONB,
		'pubkey1'::text,
		2,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3"]]'::JSONB,
		'pubkey1'::text,
		3,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "revoked:1672531200"]]'::JSONB,
		'pubkey1'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"], ["p", "pubkey1", "", "inactive:1675209600"]]'::JSONB,
		'pubkey1'::text,
		1,
		1674000000
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "1672531200", "active:1672531200"], ["p", "pubkey1", "1675209600", "inactive:1675209600"]]'::JSONB,
		'pubkey1'::text,
		1,
		1676000000
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
		'pubkey1',
		1,
		1676000000
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"], ["p", "pubkey2", "", "active:1672531200"]]'::JSONB,
		'pubkey2'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3,6"]]'::JSONB,
		'pubkey1'::text,
		6,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.True(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200:1,3"]]'::JSONB,
		'pubkey1'::text,
		6,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"]]'::JSONB,
		'pubkey1'::text,
		1,
		1671000000
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"], ["p", "pubkey1", "", "inactive:1675209600"]]'::JSONB,
		'pubkey1'::text,
		1,
		1676000000
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"], ["p", "pubkey1", "", "inactive:1675209600"]]'::JSONB,
		'pubkey1'::text,
		1,
		1672531200
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "active:1672531200"], ["p", "pubkey1", "", "inactive:1675209600"]]'::JSONB,
		'pubkey1'::text,
		1,
		1675209600
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[]'::JSONB,
		'master_pubkey1'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[]'::JSONB,
		'pubkey1'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)

	result, err = postgres.Get[bool](t.Context(), db.dbPostgres, `SELECT subzero_nostr_onbehalf_is_allowed(
		'[["p", "pubkey1", "", "invalid_string"]]'::JSONB,
		'pubkey1'::text,
		1,
		EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
	);`)
	require.NoError(t, err)
	require.False(t, *result)
}
