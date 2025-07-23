-- SPDX-License-Identifier: ice License 1.0

--------
CREATE TABLE IF NOT EXISTS ranked_events
(
    event_created_at  bigint    not null,
    score             bigint    not null,
    points            bigint    not null,
    event_kind        integer   not null,
    event_id          text      not null primary key REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
) WITH (FILLFACTOR = 70);
--------