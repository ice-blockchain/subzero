-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS events
(
    rid               integer primary key,
    kind              integer not null,
    created_at        integer not null,
    system_created_at integer not null,
    id                text    not null UNIQUE,
    pubkey            text    not null,
    master_pubkey     text    not null,
    sig               text    not null,
    sig_alg           text    not null DEFAULT '',
    key_alg           text    not null DEFAULT '',
    content           text    not null,
    content_metadata  text    not null DEFAULT '',
    d_tag             text    not null DEFAULT '',
    h_tag             text    not null DEFAULT '',
    reference_id      text    references events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tags              text    not null DEFAULT '[]',
    hidden            integer not null default 0
) strict;
--------
create unique index if not exists replaceable_event_uk on events(master_pubkey, kind)
where (10000 <= kind AND kind < 20000 ) OR kind = 0 OR kind = 3;
--------
create unique index if not exists parameterized_replaceable_event_uk on events(master_pubkey, kind, d_tag)
where 30000 <= kind AND kind < 40000;
--------
drop index if exists uix_events_h_tag;
create unique index if not exists transferable_replaceable_event_uk on events(h_tag)
where kind = 31750;
--------

-- Where order:
--   system_created_at
--   id
--   kind
--   pubkey
--   master_pubkey
--   created_at
-- Order by:
--   system_created_at DESC

CREATE INDEX IF NOT EXISTS idx_events_system_created_at                           ON events(system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_kind_system_created_at                      ON events(kind, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_system_created_at                    ON events(pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_master_pubkey_system_created_at             ON events(master_pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_kind_pubkey_system_created_at               ON events(kind, pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_kind_master_pubkey_system_created_at        ON events(kind, master_pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_system_created_at                   ON events(id, kind, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_created_at_system_created_at             ON events(id, created_at DESC, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_pubkey_system_created_at                 ON events(id, pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_master_pubkey_system_created_at          ON events(id, master_pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_pubkey_created_at_system_created_at ON events(id, kind, pubkey, created_at DESC, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_master_pubkey_created_at_system_created_at ON events(id, kind, master_pubkey, created_at DESC, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_system_created_at_id_created_at             ON events(system_created_at DESC, id, created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_reference_id_system_created_at              ON events(reference_id, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_master_pubkey_system_created_at      ON events(pubkey, master_pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_h_tag_system_created_at                     ON events(h_tag, system_created_at DESC) where kind = 1753;

-- Special index for inserts.
CREATE INDEX IF NOT EXISTS idx_events_reference_id ON events(reference_id);

--------
CREATE TABLE IF NOT EXISTS event_tags
(
    event_id          text not null references events (id) ON UPDATE RESTRICT ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_tag_key     text not null,
    event_tag_value1  text not null DEFAULT '',
    event_tag_value2  text not null DEFAULT '',
    event_tag_value3  text not null DEFAULT '',
    event_tag_value4  text not null DEFAULT '',
    event_tag_value5  text not null DEFAULT '',
    event_tag_value6  text not null DEFAULT '',
    event_tag_value7  text not null DEFAULT '',
    event_tag_value8  text not null DEFAULT '',
    event_tag_value9  text not null DEFAULT '',
    event_tag_value10 text not null DEFAULT '',
    event_tag_value11 text not null DEFAULT '',
    event_tag_value12 text not null DEFAULT '',
    event_tag_value13 text not null DEFAULT '',
    event_tag_value14 text not null DEFAULT '',
    event_tag_value15 text not null DEFAULT '',
    event_tag_value16 text not null DEFAULT '',
    event_tag_value17 text not null DEFAULT '',
    event_tag_value18 text not null DEFAULT '',
    event_tag_value19 text not null DEFAULT '',
    event_tag_value20 text not null DEFAULT '',
    event_tag_value21 text not null DEFAULT '',
    primary key (event_id, event_tag_key, event_tag_value1)
) strict, WITHOUT ROWID;
--------
--- TODO: optimize index size and usage.
create index if not exists idx_event_tags_key_value1                  on event_tags(event_tag_key, event_tag_value1);
create index if not exists idx_event_tags_key_value1_expiration       on event_tags(event_tag_key, event_tag_value1) where (event_tag_key = 'expiration' and cast(event_tag_value1 as integer) > 0);
create index if not exists idx_event_tags_key_value2                  on event_tags(event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_key_value3                  on event_tags(event_tag_key, event_tag_value3);
create index if not exists idx_event_tags_id_key_value2               on event_tags(event_id, event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value2        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value3        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value3);
create index if not exists idx_event_tags_id_key_value1_value2_value3 on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2, event_tag_value3);
--------
drop   trigger if     exists trigger_events_after_insert_generate_tags;
create trigger if not exists trigger_events_after_insert_generate_tags
    after insert
    on events
    for each row
begin
    insert into event_tags(
        event_id,
        event_tag_key,
        event_tag_value1,
        event_tag_value2,
        event_tag_value3,
        event_tag_value4,
        event_tag_value5,
        event_tag_value6,
        event_tag_value7,
        event_tag_value8,
        event_tag_value9,
        event_tag_value10,
        event_tag_value11,
        event_tag_value12,
        event_tag_value13,
        event_tag_value14,
        event_tag_value15,
        event_tag_value16,
        event_tag_value17,
        event_tag_value18,
        event_tag_value19,
        event_tag_value20,
        event_tag_value21)
    select
        new.id,
        value ->> 0,
        coalesce(value ->> 1,''),
        coalesce(value ->> 2,''),
        coalesce(value ->> 3,''),
        coalesce(value ->> 4,''),
        coalesce(value ->> 5,''),
        coalesce(value ->> 6,''),
        coalesce(value ->> 7,''),
        coalesce(value ->> 8,''),
        coalesce(value ->> 9,''),
        coalesce(value ->> 10,''),
        coalesce(value ->> 11,''),
        coalesce(value ->> 12,''),
        coalesce(value ->> 13,''),
        coalesce(value ->> 14,''),
        coalesce(value ->> 15,''),
        coalesce(value ->> 16,''),
        coalesce(value ->> 17,''),
        coalesce(value ->> 18,''),
        coalesce(value ->> 19,''),
        coalesce(value ->> 20,''),
        coalesce(value ->> 21,'')
    from
        json_each(jsonb(subzero_nostr_tags_reorder(coalesce(new.tags, ''))))
    where
        value ->> 0 is not null
    on conflict do nothing;
end
;
--------
create trigger if not exists trigger_events_before_update_remove_old_data
    before update
    on events
    for each row
    when (new.tags != old.tags) OR (new.id != old.id)
begin
    delete from event_tags where event_id in (new.id, old.id);
end
;
--------
drop   trigger if     exists trigger_events_after_update_generate_tags;
create trigger if not exists trigger_events_after_update_generate_tags
    after update
    on events
    for each row
    when (new.tags != old.tags) OR (new.id != old.id)
begin
    insert into event_tags(
        event_id,
        event_tag_key,
        event_tag_value1,
        event_tag_value2,
        event_tag_value3,
        event_tag_value4,
        event_tag_value5,
        event_tag_value6,
        event_tag_value7,
        event_tag_value8,
        event_tag_value9,
        event_tag_value10,
        event_tag_value11,
        event_tag_value12,
        event_tag_value13,
        event_tag_value14,
        event_tag_value15,
        event_tag_value16,
        event_tag_value17,
        event_tag_value18,
        event_tag_value19,
        event_tag_value20,
        event_tag_value21)
    select
        new.id,
        value ->> 0,
        coalesce(value ->> 1,''),
        coalesce(value ->> 2,''),
        coalesce(value ->> 3,''),
        coalesce(value ->> 4,''),
        coalesce(value ->> 5,''),
        coalesce(value ->> 6,''),
        coalesce(value ->> 7,''),
        coalesce(value ->> 8,''),
        coalesce(value ->> 9,''),
        coalesce(value ->> 10,''),
        coalesce(value ->> 11,''),
        coalesce(value ->> 12,''),
        coalesce(value ->> 13,''),
        coalesce(value ->> 14,''),
        coalesce(value ->> 15,''),
        coalesce(value ->> 16,''),
        coalesce(value ->> 17,''),
        coalesce(value ->> 18,''),
        coalesce(value ->> 19,''),
        coalesce(value ->> 20,''),
        coalesce(value ->> 21,'')
    from
        json_each(jsonb(subzero_nostr_tags_reorder(coalesce(new.tags, ''))))
    where
        value ->> 0 is not null
    on conflict do nothing;
end
;
--------
drop   trigger if     exists trigger_events_before_insert_unwind_repost;
create trigger if not exists trigger_events_before_insert_unwind_repost
    before insert
    on events
    for each row
    when new.kind in (6, 16)
begin
insert into events
    (kind, created_at, system_created_at, id, pubkey, master_pubkey, sig, content, content_metadata, tags, d_tag, h_tag, hidden)
select
    json_extract(b, '$.kind'),
    0,
    0,
    json_extract(b, '$.id'),
    '',
    '',
    '',
    json_extract(b, '$.content'),
    coalesce(json_extract(b, '$.content_metadata'), ''),
    json_extract(b, '$.tags'),
    '',
    json_extract(b, '$.id'),
    1
from
    (select NEW.content as b)
where
    NEW.content != '' AND json_valid(NEW.content)
on conflict do nothing;
end
;
--------
drop   trigger if     exists trigger_events_after_insert_link_repost;
create trigger if not exists trigger_events_after_insert_link_repost
    after insert
    on events
    for each row
    when new.kind in (6, 16)
begin
update events set
    reference_id = json_extract(NEW.content, '$.id')
where
    id = new.id AND
    NEW.content != '' AND
    json_valid(NEW.content) AND
    json_extract(NEW.content, '$.id') != '';
end
;
--------
create trigger if not exists trigger_events_before_insert_check_onbehalf_permission
    before insert
    on events
    for each row
    when new.master_pubkey != new.pubkey
begin
    select raise(ABORT, 'onbehalf permission denied') where not subzero_nostr_onbehalf_is_allowed(
        coalesce((select tags from events where events.kind = 10100 and events.pubkey = new.master_pubkey and hidden = 0), '[]'),
        new.pubkey,
        new.master_pubkey,
        new.kind,
        unixepoch()
    );
end
;
--------
create trigger if not exists trigger_events_before_update_check_attestation_list_content
    before update
    on events
    for each row
    when (new.kind = 10100) AND (new.tags != old.tags)
begin
    select raise(ABORT, 'attestation list update must be linear') where not subzero_nostr_attestation_update_is_allowed(
        coalesce(old.tags, '[]'),
        coalesce(new.tags, '[]')
    );
end
;
--------
drop   trigger if     exists trigger_events_before_delete_remove_tags_explicit;
create trigger if not exists trigger_events_before_delete_remove_tags_explicit
    before delete
    on events
    for each row
begin
    delete from event_tags where event_id = OLD.id;
    delete from event_counters where reference_id = OLD.id;
end
;
--------
CREATE TABLE IF NOT EXISTS event_counters
(
    reference_id   text    not null,
    reference_type text    not null DEFAULT '', -- for kind 7 events, it contains the actual reaction to the event, like `+` or `-`.
    kind           integer not null,
    value          integer not null DEFAULT 0,
    primary key (kind, reference_type, reference_id)
) strict, WITHOUT ROWID;
--------
create index if not exists idx_event_counters_reference_id on event_counters(reference_id);
--------
drop   trigger if     exists trigger_event_tags_after_insert_inc_counter;
create trigger if not exists trigger_event_tags_after_insert_inc_counter
    after insert
    on event_tags
    for each row
    when (NEW.event_tag_key in ('a', 'q', 'e', 'p', 'Q', 'h')) AND (NEW.event_tag_value1 != '')
begin
    insert into event_counters (reference_id, reference_type, kind, value)
    with is_community_closed as (
        select
            community.h_tag,
            case
                when exists (
                    select 1
                    from events e
                    where e.h_tag = community.h_tag and e.kind = 1753 and exists (
                        select 1 from json_each(e.tags) where json_valid(e.tags) and json_extract(value, '$[0]') = 'closed'
                    )
                    order by e.system_created_at desc
                    limit 1
                ) or exists (
                    select 1
                    from json_each(community.tags)
                    where json_valid(community.tags) and json_extract(value, '$[0]') = 'closed'
                ) then 1
                else 0
            end as closed_status
        from events community
        where community.h_tag = NEW.event_tag_value1 and community.kind = 31750
    )
    select
        NEW.event_tag_value1,
        case
            when e.kind = 1750 and NEW.event_tag_key = 'h' then 'members'
            when e.kind in (1, 6, 16, 30023, 30175) and NEW.event_tag_key in ('a', 'e') and NEW.event_tag_value3 in ('reply', 'root') then NEW.event_tag_value3
            when e.kind in (1, 6, 16, 30023, 30175) and NEW.event_tag_key in ('q', 'Q') then 'quote'
            when e.kind = 3 and NEW.event_tag_key = 'p' then 'follower'
            when e.kind = 7 then e.content -- reaction type
            else ''
        end,
        e.kind,
        1
    from
        events e
    left join events community on community.h_tag = e.h_tag and community.kind = 31750
    left join is_community_closed c on c.h_tag = NEW.event_tag_value1
    where
            e.id = NEW.event_id
        and e.kind in (1, 3, 6, 7, 16, 1750, 30023, 30175)
        and (e.kind = 3 OR NEW.event_tag_key in ('a', 'Q', 'h') OR exists (select 1 from events where id = NEW.event_tag_value1))
        and (
            case
                when e.kind = 7 then
                    -- As per NIP25, we want only the value of the last `e` tag here OR `a` tag.
                    NEW.event_tag_value1 = (
                        select
                            json_group_array(json_extract(value, '$[1]'))->>'$[#-1]'
                        from
                            json_each(e.tags)
                        where
                            json_valid(e.tags) and json_extract(value, '$[0]') = 'e'
                    ) OR NEW.event_tag_key = 'a'
                when e.kind in (1, 6, 16, 30023, 30175) and NEW.event_tag_key in ('a', 'e') and NEW.event_tag_value3 != '' then
                    ((NEW.event_tag_value3 = 'root' AND NEW.event_tag_value5 = '') OR (NEW.event_tag_value3 = 'reply'))
                when e.kind = 1750 and NEW.event_tag_key = 'h' and NEW.event_tag_value1 = community.h_tag then
                    (
                        c.closed_status = 1 AND
                        (
                            -- Filtering invitations for closed communities, take into the account only joins.
                            EXISTS(
                                select
                                    1
                                from
                                    json_each(e.tags)
                                where
                                    json_valid(e.tags) and json_extract(value, '$[0]') = 'authorization'
                            )
                            -- Increase counter for the owner's join event without authorization tag also when community is closed.
                            OR (
                                (
                                    e.pubkey = community.pubkey OR
                                    e.master_pubkey = community.master_pubkey
                                )
                                AND
                                (
                                    EXISTS(
                                        select
                                            1
                                        from
                                            json_each(e.tags)
                                        where
                                            json_valid(e.tags) and json_extract(value, '$[0]') = 'p' and (json_extract(value, '$[1]') = community.pubkey or json_extract(value, '$[0]') == community.master_pubkey)
                                    )
                                )
                            )
                        )
                    )
                    OR c.closed_status = 0
                else true
            end
        )
    on conflict do update
    set
        value = value + 1;
end
;
--------
drop   trigger if     exists trigger_event_tags_after_delete_dec_counter;
create trigger if not exists trigger_event_tags_after_delete_dec_counter
    after delete
    on event_tags
    for each row
    when (OLD.event_tag_key in ('a', 'q', 'e', 'p', 'Q', 'h')) AND (OLD.event_tag_value1 != '')
begin
    update event_counters set
        value = max(value - 1, 0)
    from
        events e
    left join events community
        ON community.h_tag = e.h_tag AND community.kind = 31750
    where
            e.id = OLD.event_id
        and event_counters.reference_id = OLD.event_tag_value1
        and event_counters.kind = e.kind
        and event_counters.reference_type = case
            when e.kind in (1, 6, 16, 30023, 30175) and OLD.event_tag_key in ('a', 'e') and
                ((OLD.event_tag_value3 = 'root' AND OLD.event_tag_value5 = '') OR (OLD.event_tag_value3 = 'reply')) then OLD.event_tag_value3
            when e.kind in (1, 6, 16, 30023, 30175) and OLD.event_tag_key in ('q', 'Q') then 'quote'
            when e.kind = 3 and OLD.event_tag_key = 'p' then 'follower'
            when e.kind = 7 then e.content
            when e.kind = 1750 and OLD.event_tag_key = 'h' then 'members'
            else ''
        end
        and (
            case
                when e.kind = 7 then
                    OLD.event_tag_value1 = (
                        select
                            json_group_array(json_extract(je.value, '$[1]'))->>'$[#-1]'
                        from
                            json_each(e.tags) je
                        where
                            json_valid(e.tags) and json_extract(je.value, '$[0]') = 'e'
                    ) OR OLD.event_tag_key = 'a'
                when e.kind = 1750 and OLD.event_tag_key = 'h' and OLD.event_tag_value1 = community.h_tag then
                    exists(
                        select
                            1
                        from
                            json_each(e.tags)
                        where
                            json_valid(e.tags) and json_extract(value, '$[0]') = 'p' and (json_extract(value, '$[1]') = e.pubkey or json_extract(value, '$[0]') == e.master_pubkey)
                    )
                else true
            end
        );
        delete from event_counters where reference_id = OLD.event_tag_value1 and value = 0;
end
;
--------
CREATE VIRTUAL TABLE if not exists events_search USING fts5(content, content_metadata, content='events', content_rowid=rid);
--------
drop   trigger if     exists trigger_events_after_insert_search_index;
create trigger if not exists trigger_events_after_insert_search_index
    after insert
    ON events
    for each row
    when (NEW.kind = 0 and NEW.content != '' and json_valid(NEW.content) and (json_extract(NEW.content, '$.name') != '' or json_extract(NEW.content, '$.display_name') != ''))
        or (NEW.kind in (1, 30175, 30023) and (NEW.content != '' or NEW.content_metadata != '')) or (NEW.kind in (1063) and NEW.content_metadata != '')
begin
  INSERT INTO events_search(rowid, content, content_metadata) VALUES (NEW.rid, NEW.content, NEW.content_metadata);
end;
--------
drop   trigger if     exists trigger_events_after_delete_search_index;
create trigger if not exists trigger_events_after_delete_search_index
    after delete
    ON events
begin
    DELETE FROM events_search WHERE rowid = OLD.rid;
end;
--------
PRAGMA foreign_keys = on;
