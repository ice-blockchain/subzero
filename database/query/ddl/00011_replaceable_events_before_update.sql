-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS replaceable_events_before_update AS TABLE events;

DO $$ 
DECLARE
    lookup_type_name text;
BEGIN
    ALTER TABLE replaceable_events_before_update
        ADD COLUMN IF NOT EXISTS replaced_by_id TEXT NOT NULL DEFAULT '';
    if NOT exists (select constraint_name from information_schema.table_constraints where table_name = 'replaceable_events_before_update' and constraint_type = 'PRIMARY KEY') then
        ALTER TABLE replaceable_events_before_update
            ADD CONSTRAINT replaceable_events_before_update_pkey PRIMARY KEY(id, replaced_by_id);
    end if;
    ALTER TABLE replaceable_events_before_update ALTER COLUMN replaced_by_id DROP DEFAULT;

    SELECT data_type INTO lookup_type_name
    FROM information_schema.columns 
    WHERE table_name = 'replaceable_events_before_update' 
    AND column_name = 'lookup';

    IF lookup_type_name = 'tsvector' THEN
        ALTER TABLE replaceable_events_before_update ADD COLUMN lookup_text text;

        UPDATE replaceable_events_before_update 
        SET lookup_text = array_to_string(tsvector_to_array(lookup), ' ') 
        WHERE lookup IS NOT NULL;

        ALTER TABLE replaceable_events_before_update DROP COLUMN lookup;
        ALTER TABLE replaceable_events_before_update RENAME COLUMN lookup_text TO lookup;

        ALTER TABLE replaceable_events_before_update ALTER COLUMN lookup SET NOT NULL;
        ALTER TABLE replaceable_events_before_update ALTER COLUMN lookup SET DEFAULT '';
    END IF;
END $$;
--------
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS expiration bigint;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS has_references boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS is_quote boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS is_reply boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS is_root_reply boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update DROP COLUMN IF EXISTS system_kind;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS t_tags text[] NOT NULL DEFAULT ARRAY[]::text[];
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS gift_receiver_pubkey TEXT;
--------
CREATE INDEX IF NOT EXISTS idx_replaceable_events_before_update_ ON replaceable_events_before_update(replaced_by_id);
