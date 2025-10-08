-- SPDX-License-Identifier: ice License 1.0

--- TODO: optimize index size and usage.

-- Where order:
--   id
--   kind
--   pubkey
--   master_pubkey
--   created_at
-- Order by:
--   created_at DESC
CREATE INDEX IF NOT EXISTS idx_events_lookup_created_at ON events(lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_lookup_created_at ON events(id, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_lookup_created_at ON events(kind, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_lookup_created_at ON events(pubkey, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_master_pubkey_lookup_created_at ON events(master_pubkey, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_pubkey_lookup_created_at ON events(kind, pubkey, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_master_pubkey_lookup_created_at ON events(kind, master_pubkey, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_lookup_created_at ON events(id, kind, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_pubkey_lookup_created_at ON events(id, pubkey, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_master_pubkey_lookup_created_at ON events(id, master_pubkey, lookup_created_at DESC) WHERE hidden = FALSE;;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_master_pubkey_lookup_created_at ON events(pubkey, master_pubkey, lookup_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_h_tag_lookup_created_at  ON events(h_tag, lookup_created_at DESC) WHERE kind = 1753 AND hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_has_videos_has_images_lookup_created_at ON events(has_videos, has_images, lookup_created_at DESC) WHERE hidden = FALSE;

-- Special index for inserts.
CREATE INDEX IF NOT EXISTS idx_events_reference_id ON events(reference_id);

-- T tags index.
CREATE INDEX IF NOT EXISTS idx_events_ttags ON events USING GIN(t_tags);

-- Verified posts must come first in the feed.
CREATE INDEX IF NOT EXISTS idx_events_verified_lookup_created_at ON events(verified desc, lookup_created_at DESC) WHERE hidden = false;

-- Expiration.
CREATE INDEX IF NOT EXISTS idx_events_expiration_id ON events(expiration, id) WHERE expiration IS NOT NULL;

-- TODO: remove pgroonga DROPs after the migration on all envs.
DROP INDEX IF EXISTS idx_events_lookup_pgroonga;

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_events_lookup_trgm ON events USING gin (lookup gin_trgm_ops);

-- Feed request:
--   kinds":[1, 30175, 6, 30023]
--   !amarker:reply
--   !emarker:reply
--   references:false
--   expiration:false
CREATE INDEX IF NOT EXISTS
    idx_events_kind_has_references_expiration_is_reply_lookup_created_at ON
        events(kind, has_references, expiration, is_reply, lookup_created_at DESC)
        WHERE is_reply = FALSE AND has_references = FALSE AND expiration is NULL AND hidden = FALSE;

CREATE INDEX IF NOT EXISTS idx_events_gift_receiver_pubkey ON events(gift_receiver_pubkey NULLS FIRST) WHERE hidden = FALSE;
