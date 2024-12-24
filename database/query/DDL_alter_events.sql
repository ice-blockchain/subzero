-- SPDX-License-Identifier: ice License 1.0

ALTER TABLE events ADD COLUMN h_tag text not null DEFAULT '';
--------
UPDATE events SET h_tag = id WHERE h_tag = NULL OR h_tag = '';
--------
CREATE UNIQUE INDEX IF NOT EXISTS uix_events_h_tag ON events(h_tag);