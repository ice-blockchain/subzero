-- SPDX-License-Identifier: ice License 1.0

CREATE EXTENSION IF NOT EXISTS pgroonga;

DO $$
DECLARE
    lookup_type_name text;
BEGIN
    SELECT data_type INTO lookup_type_name
    FROM information_schema.columns 
    WHERE table_name = 'events' 
    AND column_name = 'lookup';

    IF lookup_type_name = 'tsvector' THEN
        ALTER TABLE events ADD COLUMN lookup_text text;

        UPDATE events 
        SET lookup_text = array_to_string(tsvector_to_array(lookup), ' ') 
        WHERE lookup IS NOT NULL;

        DROP INDEX IF EXISTS idx_events_lookup;

        ALTER TABLE events DROP COLUMN lookup;
        ALTER TABLE events RENAME COLUMN lookup_text TO lookup;

        ALTER TABLE events ALTER COLUMN lookup SET NOT NULL;
        ALTER TABLE events ALTER COLUMN lookup SET DEFAULT '';

        CREATE INDEX IF NOT EXISTS idx_events_lookup_pgroonga ON events USING pgroonga (lookup) WITH (tokenizer='TokenBigramSplitSymbolAlphaDigit');
    END IF;
END $$; 