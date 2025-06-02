-- SPDX-License-Identifier: ice License 1.0

DO $$ BEGIN
    -- Check if events table has expiration field of type BIGINT.
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'events'
        AND column_name = 'expiration'
        AND data_type = 'bigint'
    ) THEN
        ALTER TABLE events ADD COLUMN expiration bigint;

        with cte as (
            select
                event_id,
                to_timestamp_nano(cast(event_tag_value1 as bigint)) as val
            from
                event_tags
            where event_tag_key = 'expiration'
        )
        UPDATE events e
        SET
            expiration = cte.val
        FROM
            cte
        WHERE
            e.id = cte.event_id;
    END IF;
END $$;
--------
DO $$ BEGIN
    -- Check if events table has is_reply/is_quote fields.
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'events'
        AND column_name = 'is_reply'
    ) THEN
        -- Quote.
        ALTER TABLE events ADD COLUMN is_quote boolean NOT NULL DEFAULT FALSE;
        with cte as (
            select
                event_id
            from
                event_tags
            where
                event_tag_key in ('Q', 'q')
                and event_tag_value1 != ''
        )
        UPDATE events e
        SET
            is_quote = true
        FROM
            cte
        WHERE
            e.id = cte.event_id;

        -- Reply.
        ALTER TABLE events ADD COLUMN is_reply boolean NOT NULL DEFAULT FALSE;
        with cte as (
            select
                event_id
            from
                event_tags
            where
                event_tag_key in ('a', 'e')
                and event_tag_value1 != ''
                and event_tag_value3 = 'reply'
        )
        UPDATE events e
        SET
            is_reply = true
        FROM
            cte
        WHERE
            e.id = cte.event_id;

        -- Has references.
        ALTER TABLE events ADD COLUMN has_references boolean NOT NULL DEFAULT FALSE;
        with cte as (
            select
                event_id
            from
                event_tags
            where
                event_tag_key in ('a', 'e')
                and event_tag_value1 != ''
        )
        UPDATE events e
        SET
            has_references = true
        FROM
            cte
        WHERE
            e.id = cte.event_id;
    END IF;
END $$;
--------
-- Feed request:
--   kinds":[1, 30175, 6, 30023]
--   !amarker:reply
--   !emarker:reply
--   references:false
--   expiration:false
CREATE INDEX IF NOT EXISTS
    idx_events_kind_has_references_expiration_is_reply_lookup_created_at ON
        events(kind, has_references, expiration, is_reply, lookup_created_at DESC)
        WHERE is_reply = FALSE AND has_references = FALSE AND expiration is NULL AND hidden = FALSE;
