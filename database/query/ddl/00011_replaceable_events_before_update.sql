-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS replaceable_events_before_update AS TABLE events;
--------
DO $$ BEGIN
    ALTER TABLE replaceable_events_before_update
        ADD COLUMN IF NOT EXISTS replaced_by_id TEXT NOT NULL DEFAULT '';
    if NOT exists (select constraint_name from information_schema.table_constraints where table_name = 'replaceable_events_before_update' and constraint_type = 'PRIMARY KEY') then
        ALTER TABLE replaceable_events_before_update
            ADD CONSTRAINT replaceable_events_before_update_pkey PRIMARY KEY(id, replaced_by_id);
    end if;
    ALTER TABLE replaceable_events_before_update ALTER COLUMN replaced_by_id DROP DEFAULT;
END $$;
--------
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS expiration bigint;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS has_references boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS is_quote boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS is_reply boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS is_root_reply boolean NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update DROP COLUMN IF EXISTS system_kind;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS ttags text[] NOT NULL DEFAULT ARRAY[]::text[];
--------
CREATE INDEX IF NOT EXISTS idx_replaceable_events_before_update_ ON replaceable_events_before_update(replaced_by_id);
