-- SPDX-License-Identifier: ice License 1.0

PRAGMA foreign_keys = OFF;
ALTER TABLE events ADD COLUMN content_metadata TEXT NOT NULL DEFAULT '';
--------
ALTER TABLE events RENAME TO old_events;
--------
ALTER TABLE event_tags RENAME TO old_event_tags;
--------
CREATE TABLE events (
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
CREATE TABLE IF NOT EXISTS event_tags (
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
INSERT INTO events (
    kind,
    created_at,
    system_created_at,
    id,
    pubkey,
    master_pubkey,
    sig,
    sig_alg,
    key_alg,
    content,
    content_metadata,
    d_tag,
    h_tag,
    reference_id,
    tags,
    hidden
)
SELECT
    kind,
    created_at,
    system_created_at,
    id,
    pubkey,
    master_pubkey,
    sig,
    sig_alg,
    key_alg,
    content,
    content_metadata,
    d_tag,
    h_tag,
    reference_id,
    tags,
    hidden
FROM old_events;
--------
INSERT INTO event_tags SELECT * FROM old_event_tags;
--------
PRAGMA foreign_keys = ON;
--------
CREATE VIRTUAL TABLE if not exists events_search USING fts5(content, content_metadata, content='events', content_rowid=rid);
CREATE TRIGGER if not exists trigger_events_after_insert_search_index 
    AFTER INSERT
    ON events 
    for each row
    when (NEW.kind = 0 and NEW.content != '' and json_valid(NEW.content) and (json_extract(NEW.content, '$.name') != '' or json_extract(NEW.content, '$.display_name') != ''))
    or (NEW.kind in (1, 30175, 30023) and (NEW.content != '' or NEW.content_metadata != '')) or (NEW.kind in (1063) and NEW.content_metadata != '')
BEGIN
  INSERT INTO events_search(rowid, content, content_metadata) VALUES (NEW.rid, NEW.content, NEW.content_metadata);
END;
CREATE TRIGGER if not exists trigger_events_after_delete_search_index AFTER DELETE ON events BEGIN
    DELETE FROM events_search WHERE rowid = old.rid;
END;
