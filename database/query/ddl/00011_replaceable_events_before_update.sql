-- SPDX-License-Identifier: ice License 1.0
DO $$
BEGIN
    CREATE TABLE IF NOT EXISTS replaceable_events_before_update AS TABLE events;
    IF NOT EXISTS (select constraint_name from information_schema.table_constraints where table_name = 'replaceable_events_before_update' and constraint_type = 'PRIMARY KEY') then
        ALTER TABLE replaceable_events_before_update
            ADD COLUMN IF NOT EXISTS replaced_by_id TEXT NOT NULL DEFAULT '';
        ALTER TABLE replaceable_events_before_update
            ADD CONSTRAINT replaceable_events_before_update_pkey PRIMARY KEY(id, replaced_by_id);
        ALTER TABLE replaceable_events_before_update
            ALTER COLUMN replaced_by_id DROP DEFAULT;
    END IF;
END $$;
--------
CREATE INDEX IF NOT EXISTS idx_replaceable_events_before_update_ ON replaceable_events_before_update(replaced_by_id);
--------
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS has_ephemeral_attestation BOOLEAN;
--------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'replaceable_events_before_update' AND column_name = 'system_id') THEN
      ALTER TABLE replaceable_events_before_update ADD COLUMN system_id TEXT;
      UPDATE replaceable_events_before_update SET system_id = id;
    END IF;
END $$;
