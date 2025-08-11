-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION trigger_events_after_insert_generate_tags()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO event_tags (
        event_id,
        event_tag_key,
        event_tag_value1,
        event_tag_value2,
        event_tag_value3,
        event_tag_value4,
        event_tag_value5
    )
    SELECT
        NEW.id,
        value->>0,
        COALESCE(value->>1, ''),
        COALESCE(value->>2, ''),
        COALESCE(value->>3, ''),
        COALESCE(value->>4, ''),
        COALESCE(value->>5, '')
    FROM jsonb_array_elements(COALESCE(NEW.tags, '[]'::jsonb)) AS value
    WHERE
        length(value->>0) = 1 OR value->>0 in ('summary', 'name', 'description', 'title', 'poll', 'ox', 'token', 'relay')
    ON CONFLICT(event_id, event_tag_key, event_tag_value1, event_tag_value3) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_generate_tags
AFTER INSERT ON events
FOR EACH ROW
EXECUTE FUNCTION trigger_events_after_insert_generate_tags();
--------
CREATE OR REPLACE FUNCTION trigger_events_before_update_remove_old_data()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM event_tags WHERE event_id IN (NEW.id, OLD.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_update_remove_old_data
BEFORE UPDATE ON events 
FOR EACH ROW
WHEN (NEW.tags != OLD.tags OR NEW.id != OLD.id)
EXECUTE FUNCTION trigger_events_before_update_remove_old_data();    
--------
CREATE OR REPLACE FUNCTION trigger_events_after_update_generate_tags()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO event_tags (
        event_id,
        event_tag_key,
        event_tag_value1,
        event_tag_value2,
        event_tag_value3,
        event_tag_value4,
        event_tag_value5
    )
    SELECT
        NEW.id,
        value->>0,
        COALESCE(value->>1, ''),
        COALESCE(value->>2, ''),
        COALESCE(value->>3, ''),
        COALESCE(value->>4, ''),
        COALESCE(value->>5, '')
    FROM jsonb_array_elements(COALESCE(NEW.tags, '[]'::jsonb)) AS value
    WHERE
        length(value->>0) = 1 OR value->>0 in ('summary', 'name', 'description', 'title', 'poll', 'token', 'relay')
    ON CONFLICT(event_id, event_tag_key, event_tag_value1, event_tag_value3) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_update_generate_tags
AFTER UPDATE ON events
FOR EACH ROW
WHEN (NEW.tags != OLD.tags OR NEW.id != OLD.id)
EXECUTE FUNCTION trigger_events_after_update_generate_tags();
--------
CREATE OR REPLACE FUNCTION trigger_events_before_insert_unwind_repost()
RETURNS TRIGGER AS $$
DECLARE
    val integer;
BEGIN
    INSERT INTO events (
        kind,
        created_at,
        id,
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
    WHERE NEW.content != '' AND jsonb_valid(NEW.content) AND NEW.content::JSONB ? 'kind'
    ON CONFLICT DO NOTHING;

    select
       raise_repost_error()
    into val
    from
        events x
    where
        jsonb_valid(NEW.content)
        and (x.id = NEW.content::JSONB->>'id' OR x.address = subzero_nostr_get_event_address_json(NEW.content::JSONB))
        and x.deleted = true;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_unwind_repost
BEFORE INSERT ON events
FOR EACH ROW
WHEN (NEW.kind IN (6, 16))
EXECUTE FUNCTION trigger_events_before_insert_unwind_repost();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_insert_link_repost()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind IN (6, 16) AND NEW.content != '' AND jsonb_valid(NEW.content) AND NEW.content::jsonb ? 'id' THEN
        UPDATE events
        SET reference_id = NEW.content::jsonb ->> 'id'
        WHERE
            id = NEW.id
            AND EXISTS (select 1 from events ee where ee.id = NEW.content::jsonb ->> 'id');
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_link_repost
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind IN (6, 16))
EXECUTE FUNCTION trigger_events_after_insert_link_repost();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_update_mark_refereces_deleted()
RETURNS TRIGGER AS $$
BEGIN
    delete from events where reference_id in (new.id, old.id) and kind in (6, 16);
    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_update_mark_refereces_deleted
AFTER UPDATE ON events
FOR EACH ROW
WHEN (new.deleted = true AND old.deleted = false AND ((new.tags != old.tags) OR (new.id != old.id)) AND new.kind in (30023, 30024, 30175) AND new.reference_id is null)
EXECUTE FUNCTION trigger_events_after_update_mark_refereces_deleted();
--------
CREATE OR REPLACE FUNCTION trigger_events_before_update_check_attestation_list_content()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT subzero_nostr_attestation_update_is_allowed(
        COALESCE(OLD.tags, '[]'::jsonb),
        COALESCE(NEW.tags, '[]'::jsonb)
    ) THEN
        RAISE EXCEPTION 'attestation list update must be linear';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_update_check_attestation_list_content
BEFORE UPDATE ON events
FOR EACH ROW
WHEN (NEW.kind = 10100 AND NEW.tags IS DISTINCT FROM OLD.tags)
EXECUTE FUNCTION trigger_events_before_update_check_attestation_list_content();
--------
CREATE OR REPLACE FUNCTION trigger_events_before_delete_remove_tags_explicit()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM event_tags     WHERE event_id = OLD.id;
    DELETE FROM event_counters WHERE reference_id = OLD.id;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_delete_remove_tags_explicit
BEFORE DELETE ON events
FOR EACH ROW
EXECUTE FUNCTION trigger_events_before_delete_remove_tags_explicit();
--------
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
        has_references,
        hidden,
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
            old.has_references,
            old.hidden,
            new.id
           )
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_store_replaceable_data_before_update
    AFTER UPDATE ON events
    FOR EACH ROW
    WHEN (((10000 <= old.kind AND old.kind < 20000 ) OR old.kind = 0 OR old.kind = 3 OR (30000 <= old.kind AND old.kind < 40000)) AND old.id != new.id)
    EXECUTE FUNCTION events_store_replaceable_data_before_update();
--------
CREATE OR REPLACE FUNCTION trigger_events_before_insert_check_onbehalf_permission()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.master_pubkey != NEW.pubkey THEN
        IF NOT subzero_nostr_onbehalf_is_allowed_on_time(
            COALESCE((
                SELECT tags
                FROM events
                WHERE kind = 10100 AND pubkey = NEW.master_pubkey AND hidden = FALSE
            ), '[]'::JSONB),
            NEW.pubkey::text,
            NEW.kind,
            get_current_timestamp_nano()
        ) THEN
            RAISE EXCEPTION 'onbehalf permission denied';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_check_onbehalf_permission
BEFORE INSERT ON events
FOR EACH ROW
WHEN (NEW.master_pubkey != NEW.pubkey AND (NEW.pubkey != '' AND NEW.master_pubkey != ''))
EXECUTE FUNCTION trigger_events_before_insert_check_onbehalf_permission();

CREATE OR REPLACE TRIGGER trigger_events_before_update_check_onbehalf_permission
BEFORE UPDATE ON events
FOR EACH ROW
WHEN ((NEW.master_pubkey != NEW.pubkey AND (NEW.pubkey != '' AND NEW.master_pubkey != '')) AND (OLD.id != NEW.id))
EXECUTE FUNCTION trigger_events_before_insert_check_onbehalf_permission();
--------
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
    insert into ranked_events(event_id, event_kind, event_created_at, points, score)
    select
        e.id,
        e.kind,
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

CREATE OR REPLACE TRIGGER trigger_events_after_insert_score_add
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind in (1, 6, 7, 16, 30023, 30175) and NEW.hidden = false and NEW.deleted = false)
EXECUTE FUNCTION trigger_events_after_insert_score_add();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_delete_score_dec()
RETURNS TRIGGER AS $$
BEGIN
    with affected_events as (
        select
            e.id AS event_id
        from events e
        join lateral jsonb_array_elements(OLD.tags) je on true
        where
            je->>0 IN ('a', 'e', 'q', 'Q')
            and e.address = je->>1
            and e.hidden = false
            and e.deleted = false
            and e.lookup_created_at > 0
            and e.kind IN (1, 30023, 30175)
            and event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply) > 0
            and case when OLD.is_root_reply then je->>3 = 'root' else true end
            limit 1
    )
    update ranked_events
    set
        points = points - event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply),
        score  = event_calculate_score_int(points - event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply), event_created_at)
    from
        affected_events
    where
        ranked_events.event_id = affected_events.event_id;
    delete from ranked_events where points <= 0;
    return OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_delete_score_dec
AFTER DELETE ON events
FOR EACH ROW
WHEN (OLD.kind in (1, 6, 7, 16, 30023, 30175) and OLD.hidden = false and OLD.deleted = false)
EXECUTE FUNCTION trigger_events_after_delete_score_dec();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_update_score_add()
RETURNS TRIGGER AS $$
BEGIN
    update ranked_events
    set
        points = points - event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply),
        score =  event_calculate_score_int(points - event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply), event_created_at)
    where exists (
        select 1
        from
            jsonb_array_elements(OLD.tags) je
        where
            je->>0 in ('a', 'e', 'q', 'Q')
        and event_id in (
            select e.id
            from events e
            where e.address = je->>1
            and e.hidden = false
            and e.lookup_created_at > 0
            and e.kind in (1, 30023, 30175)
            and case when OLD.is_root_reply then je->>3 = 'root' else true end
            and event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply) > 0
        )
    );
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
    insert into ranked_events(event_id, event_kind, event_created_at, points, score)
    select
        e.id,
        e.kind,
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
        and NEW.deleted = false
        and event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply) > 0
    on conflict (event_id) do update
    set
        points = ranked_events.points + excluded.points,
        score  = event_calculate_score_int(ranked_events.points + excluded.points, excluded.event_created_at);
    delete from ranked_events where points <= 0;
    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_update_score_add
AFTER UPDATE ON events
FOR EACH ROW
WHEN (((NEW.kind in (1, 6, 7, 16, 30023, 30175)) and (NEW.hidden = false) and (NEW.tags != OLD.tags)) OR (OLD.deleted != NEW.deleted))
EXECUTE FUNCTION trigger_events_after_update_score_add();
