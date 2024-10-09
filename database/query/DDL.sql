-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS events
(
    kind              integer not null,
    created_at        integer not null,
    system_created_at integer not null,
    id                text    not null primary key,
    pubkey            text    not null,
    sig               text    not null,
    content           text    not null,
    d_tag             text    not null DEFAULT '',
    reference_id      text    references events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tags              text    not null DEFAULT '[]',
    hidden            integer not null default 0
) strict, WITHOUT ROWID;
--------
create unique index if not exists replaceable_event_uk on events(pubkey, kind)
where (10000 <= kind AND kind < 20000 ) OR kind = 0 OR kind = 3;
--------
create unique index if not exists parameterized_replaceable_event_uk on events(pubkey, kind, d_tag)
where 30000 <= kind AND kind < 40000;

-- Where order:
--   system_created_at
--   id
--   kind
--   pubkey
--   created_at
-- Order by:
--   system_created_at DESC

CREATE INDEX IF NOT EXISTS idx_events_system_created_at                           ON events(system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_kind_system_created_at                      ON events(kind, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_system_created_at                    ON events(pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_kind_pubkey_system_created_at               ON events(kind, pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_system_created_at                   ON events(id, kind, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_created_at_system_created_at             ON events(id, created_at DESC, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_pubkey_system_created_at                 ON events(id, pubkey, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_pubkey_created_at_system_created_at ON events(id, kind, pubkey, created_at DESC, system_created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_system_created_at_id_created_at             ON events(system_created_at DESC, id, created_at DESC) where hidden = 0;
CREATE INDEX IF NOT EXISTS idx_events_reference_id_system_created_at              ON events(reference_id, system_created_at DESC) where hidden = 0;

--------
CREATE TABLE IF NOT EXISTS event_tags
(
    event_id          text not null references events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
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
create index if not exists idx_event_tags_key_value2                  on event_tags(event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_key_value3                  on event_tags(event_tag_key, event_tag_value3);
create index if not exists idx_event_tags_id_key_value2               on event_tags(event_id, event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value2        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value3        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value3);
create index if not exists idx_event_tags_id_key_value1_value2_value3 on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2, event_tag_value3);
--------
create trigger if not exists generate_event_tags
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
    (
        select subzero_nostr_tags_reoder(coalesce(cast(value as text), '')) as value from json_each(jsonb(new.tags))
    ) where value ->> 0 is not null;
end
;
--------
create trigger if not exists trigger_events_unwind_repost
    before insert
    on events
    for each row
    when new.kind = 6
begin
insert into events
    (kind, created_at, system_created_at, id, pubkey, sig, content, tags, d_tag, hidden)
select
    json_extract(b, '$.kind'),
    0,
    0,
    json_extract(b, '$.id'),
    '',
    '',
    '',
    json_extract(b, '$.tags'),
    '',
    1
from
    (select NEW.content as b)
where
    NEW.content != '' AND json_valid(NEW.content)
on conflict do nothing;
end
;
--------
create trigger if not exists trigger_events_link_repost
    after insert
    on events
    for each row
    when new.kind = 6
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
PRAGMA foreign_keys = on;
