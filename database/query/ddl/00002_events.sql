-- SPDX-License-Identifier: ice License 1.0

-- systemKindQuote        = 1
-- systemKindCommentRoot  = 2
-- systemKindCommentReply = 3

DO
$$BEGIN
   CREATE TEXT SEARCH CONFIGURATION fts ( COPY = pg_catalog.english );
EXCEPTION
   WHEN unique_violation THEN NULL;
END;$$;
--------
CREATE TABLE IF NOT EXISTS events (
    created_at        BIGINT NOT NULL,
    lookup_created_at BIGINT GENERATED ALWAYS AS (to_timestamp_nano(created_at)) STORED,
    kind           INTEGER   NOT NULL,
    system_kind    INTEGER,
    lookup         tsvector NOT NULL DEFAULT to_tsvector('fts', ''),
    key_alg        TEXT    NOT NULL DEFAULT '',
    content        TEXT    NOT NULL,
    d_tag          TEXT    NOT NULL DEFAULT '',
    h_tag          TEXT    NOT NULL DEFAULT '',
    address        TEXT    NOT NULL GENERATED ALWAYS AS (
                      CASE
                        WHEN (10000 <= kind AND kind < 20000) OR kind = 0 OR kind = 3
                          THEN coalesce(kind, 0) || ':' || coalesce(master_pubkey, pubkey, '') || ':'
                        WHEN 30000 <= kind AND kind < 40000
                          THEN coalesce(kind, 0) || ':' || coalesce(master_pubkey, pubkey, '') || ':' || coalesce(d_tag, '')
                        ELSE id
                      END
                  ) STORED,
    id             TEXT    PRIMARY KEY,
    pubkey         TEXT    NOT NULL,
    master_pubkey  TEXT    NOT NULL,
    sig            TEXT    NOT NULL,
    sig_alg        TEXT    NOT NULL DEFAULT '',
    reference_id   TEXT    DEFAULT NULL REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tags           JSONB   NOT NULL DEFAULT '[]',
    has_images     BOOLEAN NOT NULL DEFAULT FALSE,
    has_videos     BOOLEAN NOT NULL DEFAULT FALSE,
    deleted        BOOLEAN NOT NULL DEFAULT FALSE,
    hidden         BOOLEAN NOT NULL DEFAULT FALSE
);

--------
create unique index if not exists replaceable_event_uk on events(master_pubkey, kind)
where (10000 <= kind AND kind < 20000 ) OR kind = 0 OR kind = 3;
--------
create unique index if not exists parameterized_replaceable_event_uk on events(master_pubkey, kind, d_tag)
where 30000 <= kind AND kind < 40000;
--------
create unique index if not exists transferable_replaceable_event_uk on events(h_tag)
where kind = 31750;
--------
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
