-- SPDX-License-Identifier: ice License 1.0

DROP INDEX IF EXISTS idx_events_lookup_trgm;
CREATE INDEX IF NOT EXISTS idx_events_lookup_gist_trgm ON events USING gist (lookup gist_trgm_ops);
