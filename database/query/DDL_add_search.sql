-- SPDX-License-Identifier: ice License 1.0

ALTER TABLE events ADD COLUMN metadata TEXT NOT NULL DEFAULT '';
--------
CREATE TABLE IF NOT EXISTS new_events (
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
    metadata          text    not null DEFAULT '',
    d_tag             text    not null DEFAULT '',
    h_tag             text    not null UNIQUE,
    reference_id      text    references new_events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tags              text    not null DEFAULT '[]',
    hidden            integer not null default 0
) strict;
--------
CREATE TABLE IF NOT EXISTS new_event_tags (
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
drop   trigger if     exists trigger_events_after_insert_generate_tags;
drop   trigger if     exists trigger_events_before_update_remove_old_data;
drop   trigger if     exists trigger_events_after_update_generate_tags;
drop   trigger if     exists trigger_events_before_insert_unwind_repost;
drop   trigger if     exists trigger_events_after_insert_link_repost;
drop   trigger if     exists trigger_events_before_insert_check_onbehalf_permission;
drop   trigger if     exists trigger_events_before_update_check_attestation_list_content;
drop   trigger if     exists trigger_events_before_delete_remove_tags_explicit;
drop   trigger if     exists trigger_event_tags_after_insert_inc_counter;
drop   trigger if     exists trigger_event_tags_after_delete_dec_counter;
drop   trigger if     exists trigger_events_search_insert;
--------
INSERT INTO new_events (
    rid,
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
    metadata,
    d_tag,
    h_tag,
    reference_id,
    tags,
    hidden
)
SELECT
    rowid,
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
    metadata,
    d_tag,
    h_tag,
    reference_id,
    tags,
    hidden
FROM events;
--------
INSERT INTO new_event_tags SELECT * FROM event_tags;
--------
ALTER TABLE events RENAME TO old_events;
ALTER TABLE new_events RENAME TO events;
--------
ALTER TABLE event_tags RENAME TO old_event_tags;
ALTER TABLE new_event_tags RENAME TO event_tags;
--------
ALTER TABLE events_search RENAME TO old_events_search;
--------
CREATE VIRTUAL TABLE if not exists events_search USING fts5(content, metadata, content='events', content_rowid=rid);
