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
    temp_tags         text
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

CREATE INDEX IF NOT EXISTS idx_events_kind_system_created_at                      ON events(kind, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_pubkey_system_created_at                    ON events(pubkey, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_kind_pubkey_system_created_at               ON events(kind, pubkey, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_id_kind_system_created_at                   ON events(id, kind, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_id_created_at_system_created_at             ON events(id, created_at DESC, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_id_pubkey_system_created_at                 ON events(id, pubkey, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_id_kind_pubkey_created_at_system_created_at ON events(id, kind, pubkey, created_at DESC, system_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_system_created_at_id_created_at             ON events(system_created_at DESC, id, created_at DESC);

--------
CREATE TABLE IF NOT EXISTS event_tags
(
    event_id         text    not null references events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_tag_key    text    not null,
    event_tag_value1 text    not null DEFAULT '',
    event_tag_value2 text    not null DEFAULT '',
    event_tag_value3 text    not null DEFAULT '',
    event_tag_value4 text    not null DEFAULT '',
    primary key (event_id, event_tag_key, event_tag_value1)
) strict, WITHOUT ROWID;
--------
create index if not exists event_tags_lookup1_ix on event_tags(event_tag_key,event_tag_value1);
-- TODO add more indexes
--------
create trigger if not exists generate_event_tags
    after insert
    on events
    for each row
begin
    insert into event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2, event_tag_value3, event_tag_value4)
    select
        new.id,
        value ->> 0,
        coalesce(value ->> 1,''),
        coalesce(value ->> 2,''),
        coalesce(value ->> 3,''),
        coalesce(value ->> 4,'')
    from json_each(jsonb(new.temp_tags));
    update events set temp_tags = null where id = new.id;
end
;
--------
PRAGMA foreign_keys = on;
