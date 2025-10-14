-- SPDX-License-Identifier: ice License 1.0

-- Will be covered by new indexes below.
DROP INDEX IF EXISTS idx_events_lookup_created_at;
DROP INDEX IF EXISTS idx_events_kind_lookup_created_at;
DROP INDEX IF EXISTS idx_events_gift_receiver_pubkey;

-- Not used.
DROP INDEX IF EXISTS idx_events_id_kind_lookup_created_at;
DROP INDEX IF EXISTS idx_events_expiration_id;
DROP INDEX IF EXISTS idx_events_kind_has_references_expiration_is_reply_lookup_creat;

CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at_kind_expiration_is_not_null ON events (lookup_created_at DESC, kind, expiration)
    WHERE expiration IS NOT NULL AND hidden = FALSE AND deleted = FALSE;

CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at_kind_expiration_is_null ON events (lookup_created_at DESC, kind, expiration)
    WHERE expiration IS NULL AND hidden = FALSE AND deleted = FALSE;

CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at_kind ON events (lookup_created_at DESC, kind)
    WHERE hidden = FALSE AND deleted = FALSE;

CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at_gift_receiver_pubkey ON events(lookup_created_at desc, gift_receiver_pubkey NULLS LAST)
    WHERE hidden = FALSE AND deleted = FALSE;

CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at_kind_is_quote ON events(lookup_created_at desc, kind)
    WHERE hidden = FALSE AND deleted = FALSE AND is_quote=TRUE;

CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at_kind_is_reply ON events(lookup_created_at desc, kind)
    WHERE hidden = FALSE AND deleted = FALSE AND is_reply=TRUE;
