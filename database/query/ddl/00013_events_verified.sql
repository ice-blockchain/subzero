-- SPDX-License-Identifier: ice License 1.0

-- Add verified column to events and ranked_events tables.
ALTER TABLE events                           ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS verified BOOLEAN;
ALTER TABLE ranked_events                    ADD COLUMN IF NOT EXISTS event_verified BOOLEAN NOT NULL DEFAULT FALSE;
--------

-- Verified posts must come first in the feed.
create index if not exists idx_ranked_events_verified_score            on ranked_events(event_verified desc, score desc);
create index if not exists idx_ranked_events_verified_created_at_score on ranked_events(event_verified desc, event_created_at desc, score desc);
create index if not exists idx_events_verified_lookup_created_at       on events(verified desc, lookup_created_at desc) where hidden = false;
--------

-- Remove old trigger that will be replaced with new one.
DROP TRIGGER IF EXISTS trigger_events_before_insert_unwind_repost ON events;

CREATE OR REPLACE FUNCTION trigger_events_before_insert_unwind_repost_and_verify()
RETURNS TRIGGER AS $$
DECLARE
    val integer;
BEGIN
    INSERT INTO events (
        kind,
        created_at,
        id,
        system_id,
        pubkey,
        master_pubkey,
        sig,
        content,
        tags,
        d_tag,
        h_tag,
        hidden
    )
    SELECT
        x.kind AS kind,
        0 AS created_at,
        x.id AS id,
        x.id AS system_id,
        x.pubkey AS pubkey,
        COALESCE((SELECT value->>1 FROM jsonb_array_elements(x.tags) AS value WHERE value->>0 = 'b' LIMIT 1), '') AS master_pubkey,
        '' AS sig,
        x.content AS content,
        x.tags AS tags,
        COALESCE((SELECT value->>1 FROM jsonb_array_elements(x.tags) AS value WHERE value->>0 = 'd' LIMIT 1), '') AS d_tag,
        x.id AS h_tag,
        TRUE AS hidden
    FROM
        jsonb_to_record(
            CASE
                WHEN NEW.content != '' AND jsonb_valid(NEW.content) THEN NEW.content::JSONB
                ELSE '{}'::JSONB
            END
        ) AS x(kind int, pubkey TEXT, id TEXT, content TEXT, tags JSONB)
    WHERE
        NEW.kind IN (6, 16)
        AND NEW.content != ''
        AND jsonb_valid(NEW.content)
        AND NEW.content::JSONB ? 'kind'
    ON CONFLICT DO NOTHING;

    select
       raise_repost_error()
    into val
    from
        events x
    where
        jsonb_valid(NEW.content)
        and (x.id = NEW.content::JSONB->>'id' OR x.address = subzero_nostr_get_event_address_json(NEW.content::JSONB))
        and NEW.kind in (6, 16)
        and x.deleted = true;

    IF NEW.has_ephemeral_attestation = FALSE THEN
        NEW.verified = EXISTS(
            select 1
            from events
            where
                kind=0
                and master_pubkey=NEW.master_pubkey
                and hidden=false
                and verified=true
            limit 1
        );
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_unwind_repost_and_verify
BEFORE INSERT ON events
FOR EACH ROW
WHEN (
    (NEW.kind IN (6, 16))
        OR
    (NEW.kind IN (30023, 30175) AND NEW.is_reply=false AND NEW.is_quote=false)
)
EXECUTE FUNCTION trigger_events_before_insert_unwind_repost_and_verify();
--------
-- Update scores calculation to account for verified status.
CREATE OR REPLACE FUNCTION trigger_events_after_insert_score_add()
RETURNS TRIGGER AS $$
BEGIN
    with cte as (
        select
            je->>1 as event_address
        from
            jsonb_array_elements(NEW.tags) je
        where
            je->>0 in ('a', 'e', 'q', 'Q')
            and event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply) > 0
            and case when NEW.is_root_reply then je->>3 = 'root' else true end
    )
    insert into ranked_events(event_id, event_kind, event_verified, event_created_at, points, score)
    select
        e.id,
        e.kind,
        e.verified,
        e.created_at,
        event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply),
        event_calculate_score_int(event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply), e.created_at)
    from
        events e
    inner join cte on e.address = cte.event_address
    where
        e.hidden = false
        and e.deleted = false
        and e.lookup_created_at > 0
        and e.kind in (1, 30023, 30175)
        and event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply) > 0
    on conflict (event_id) do update
    set
        points = ranked_events.points + excluded.points,
        score  = event_calculate_score_int(ranked_events.points + excluded.points, excluded.event_created_at);
    return NEW;
END;
$$ LANGUAGE plpgsql;
--------

-- Include new verified column in the replaceable events history.
CREATE OR REPLACE FUNCTION events_store_replaceable_data_before_update()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.reference_id = NEW.id THEN
        NEW.reference_id = NULL;
        RETURN NEW; -- dont insert into replaceable_events_before_update, its replay
    END IF;
    insert into replaceable_events_before_update (
        created_at,
        expiration,
        kind,
        lookup,
        key_alg,
        content,
        d_tag,
        h_tag,
        address,
        id,
        system_id,
        pubkey,
        master_pubkey,
        sig,
        sig_alg,
        reference_id,
        tags,
        t_tags,
        gift_receiver_pubkey,
        has_images,
        has_videos,
        deleted,
        is_reply,
        is_root_reply,
        is_quote,
        has_ephemeral_attestation,
        has_references,
        hidden,
        verified,
        replaced_by_id
    )
    values (
            old.created_at,
            old.expiration,
            old.kind,
            old.lookup,
            old.key_alg,
            old.content,
            old.d_tag,
            old.h_tag,
            old.address,
            old.id,
            old.system_id,
            old.pubkey,
            old.master_pubkey,
            old.sig,
            old.sig_alg,
            old.reference_id,
            old.tags,
            old.t_tags,
            old.gift_receiver_pubkey,
            old.has_images,
            old.has_videos,
            old.deleted,
            old.is_reply,
            old.is_root_reply,
            old.is_quote,
            old.has_ephemeral_attestation,
            old.has_references,
            old.hidden,
            old.verified,
            new.id
           )
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--------

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'trigger_events_set_profile_badges') THEN
        RETURN;
    END IF;

    RAISE NOTICE 'Starting migration to set verified status based on profile badges...';

    WITH
    -- Find all unique users who have the `verified` badge.
    users_to_verify AS (
        SELECT DISTINCT
            e.master_pubkey
        FROM
            events e
        JOIN
            event_tags et ON e.id = et.event_id
        WHERE
            e.kind = 30008
            AND e.hidden = FALSE
            AND e.d_tag = 'profile_badges'
            AND et.event_tag_key = 'a'
            AND starts_with(et.event_tag_value1, '30009:')
            AND et.event_tag_value1 LIKE '%:verified'
    ),
    -- Update their primary events, returning the IDs of the changed rows.
    updated_events AS (
        UPDATE
            events
        SET
            verified = TRUE
        WHERE
            master_pubkey IN (SELECT master_pubkey FROM users_to_verify)
            AND kind IN (0, 1, 6, 16, 30008, 30023, 30175)
            AND hidden = FALSE
            AND verified = FALSE
        RETURNING id
    )
    -- Update the corresponding ranked_events using the returned IDs.
    UPDATE
        ranked_events
    SET
        event_verified = TRUE
    WHERE
        event_id IN (SELECT id FROM updated_events);

    RAISE NOTICE 'Migration for profile badges finished.';
END $$;
--------
CREATE OR REPLACE FUNCTION trigger_events_set_profile_badges()
RETURNS TRIGGER AS $$
DECLARE
    has_verified_badge_def BOOLEAN;
    has_verified_award_id BOOLEAN;
    verified_badge_address TEXT;
BEGIN
    IF NOT (NEW.tags IS DISTINCT FROM OLD.tags) THEN
        RETURN NEW;
    END IF;

    -- Unpack the address of the verified badge definition from the event tags.
    SELECT
        event_tag_value1
    INTO
        verified_badge_address
    FROM
        event_tags
    WHERE
        event_id = NEW.id
        AND event_tag_key = 'a'
        AND starts_with(event_tag_value1, '30009:')
        AND event_tag_value1 LIKE '%:verified';

    IF verified_badge_address IS NULL THEN
        RETURN NEW;
    END IF;

    -- Check that unpacked badge definition exists.
    -- This is a global/system badge, it will NOT contain a 'p' tag with the user's pubkey.
    has_verified_badge_def := EXISTS(
        SELECT 1
        FROM
            events
        WHERE
            kind = 30009
            AND address = verified_badge_address
            AND hidden = FALSE
    );

    IF NOT has_verified_badge_def THEN
        RETURN NEW;
    END IF;

    -- Find the award event issued for current user using the verified badge definition.
    has_verified_award_id := EXISTS(
        SELECT
            events.id
        FROM
            events
        INNER JOIN event_tags et_ref
            ON et_ref.event_id = NEW.id
            AND et_ref.event_tag_key = 'e'
            AND events.id = et_ref.event_tag_value1
        WHERE
            kind = 8
            AND EXISTS (
                SELECT 1
                FROM event_tags
                WHERE
                    event_id = events.id
                    AND event_tag_key = 'p'
                    AND event_tag_value1 = NEW.master_pubkey
            )
            AND EXISTS (
                SELECT 1
                FROM event_tags
                WHERE
                    event_id = events.id
                    AND event_tag_key = 'a'
                    AND event_tag_value1 = verified_badge_address
            )
            AND events.hidden = FALSE
    );

    IF NOT has_verified_award_id THEN
        RETURN NEW;
    END IF;

    -- Mark all relevant events of the user as verified.
    with cte as (
        UPDATE events
        SET
            verified = TRUE
        WHERE
            master_pubkey = NEW.master_pubkey
            AND kind in (0, 1, 6, 16, 30008, 30023, 30175)
            AND hidden = FALSE
            AND verified = FALSE
        RETURNING id
    )
    UPDATE ranked_events
    SET
        event_verified = TRUE
    WHERE
        event_id IN (SELECT id FROM cte)
        AND event_verified = FALSE;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE TRIGGER trigger_events_after_insert_set_profile_badges
AFTER INSERT OR UPDATE ON events
FOR EACH ROW
WHEN (
    NEW.kind = 30008
    AND NEW.d_tag = 'profile_badges'
    AND NEW.has_ephemeral_attestation = FALSE
    AND NEW.verified = FALSE
)
EXECUTE FUNCTION trigger_events_set_profile_badges();
--------
