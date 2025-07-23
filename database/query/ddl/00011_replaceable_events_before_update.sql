-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS replaceable_events_before_update AS TABLE events;

--------
CREATE INDEX IF NOT EXISTS idx_replaceable_events_before_update_ ON replaceable_events_before_update(replaced_by_id);
