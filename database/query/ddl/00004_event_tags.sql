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

