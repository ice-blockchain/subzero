-- SPDX-License-Identifier: ice License 1.0

CREATE INDEX IF NOT EXISTS idx_events_verified_lookup_created_at_kind_expiration_is_null ON events (verified desc, lookup_created_at DESC, kind, expiration)
    WHERE expiration IS NULL AND hidden = FALSE AND deleted = false and kind in (1, 6, 16, 30175, 30023);
