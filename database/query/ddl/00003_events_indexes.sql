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
CREATE INDEX IF NOT EXISTS idx_events_lookup ON events USING GIN(lookup);
CREATE INDEX IF NOT EXISTS idx_events_address ON events(address) WHERE hidden = FALSE;
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
