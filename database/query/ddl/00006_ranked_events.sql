-- SPDX-License-Identifier: ice License 1.0

--------
CREATE TABLE IF NOT EXISTS ranked_events
(
    event_created_at  bigint    not null,
    score             bigint    not null,
    points            bigint    not null,
    event_kind        integer   not null,
    event_id          text      not null primary key REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
--------
SET CONSTRAINTS ALL IMMEDIATE;
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'ranked_events'
        AND column_name = 'score'
        AND data_type = 'bigint'
    ) THEN
        -- Score.
        ALTER TABLE ranked_events ADD COLUMN score_temp bigint;
        UPDATE ranked_events SET score_temp = cast(score * 10000 as bigint);
        ALTER TABLE ranked_events DROP COLUMN score;
        ALTER TABLE ranked_events RENAME COLUMN score_temp TO score;
        ALTER TABLE ranked_events ALTER COLUMN score SET NOT NULL;

        -- Points.
        ALTER TABLE ranked_events ADD COLUMN points_temp bigint;
        UPDATE ranked_events SET points_temp = points;
        ALTER TABLE ranked_events DROP COLUMN points;
        ALTER TABLE ranked_events RENAME COLUMN points_temp TO points;
        ALTER TABLE ranked_events ALTER COLUMN points SET NOT NULL;
    END IF;
END $$;
--------
