-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS event_tags
(
    id                bigserial,
    event_id          text not null references events (id) ON UPDATE RESTRICT ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_tag_key     text not null,
    event_tag_value1  text not null DEFAULT '',
    event_tag_value2  text not null DEFAULT '',
    event_tag_value3  text not null DEFAULT '',
    event_tag_value4  text not null DEFAULT '',
    event_tag_value5  text not null DEFAULT '',
    primary key (event_id, event_tag_key, event_tag_value1, event_tag_value3)
);
--------
drop index if exists idx_event_tags_key_value1_expiration;
drop index if exists idx_event_tags_id_key_value1_value3;
drop index if exists idx_event_tags_expired;
--------
--- TODO: optimize index size and usage.
create index if not exists idx_event_tags_key_value1                  on event_tags(event_tag_key, event_tag_value1);
create index if not exists idx_event_tags_key_value2                  on event_tags(event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_key_value3                  on event_tags(event_tag_key, event_tag_value3);
create index if not exists idx_event_tags_id_key_value2               on event_tags(event_id, event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value2        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value2_value3 on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2, event_tag_value3);
create index if not exists idx_event_tags_token_valid on event_tags(event_tag_key, event_tag_value2, id) where
    event_tag_key = 'token' AND event_tag_value2 != 'invalid';
