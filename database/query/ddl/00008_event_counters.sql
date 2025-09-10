-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS event_counters
(
    kind           INTEGER NOT NULL,
    value          INTEGER NOT NULL DEFAULT 0,
    reference_id   TEXT NOT NULL,
    reference_type TEXT NOT NULL DEFAULT '', -- for kind 7 events, it contains the actual reaction to the event, like `+` or `-`.
    PRIMARY KEY (kind, reference_type, reference_id)
) WITH (FILLFACTOR = 70);
--------
create index if not exists idx_event_counters_reference_id on event_counters(reference_id);
