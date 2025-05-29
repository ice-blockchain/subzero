-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS ranked_events
(
    event_id          text      not null primary key REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_kind        integer   not null,
    points            integer   not null,
    event_created_at  bigint    not null,
    score             real      not null
);

