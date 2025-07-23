-- SPDX-License-Identifier: ice License 1.0

--------
CREATE TABLE IF NOT EXISTS events (
    created_at        BIGINT NOT NULL,
    lookup_created_at BIGINT GENERATED ALWAYS AS (to_timestamp_nano(created_at)) STORED,
    expiration        BIGINT,
    kind           INTEGER   NOT NULL,
    lookup         TEXT    NOT NULL DEFAULT '',
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
    gift_receiver_pubkey TEXT,
    tags           JSONB   NOT NULL DEFAULT '[]',
    t_tags         TEXT[]  NOT NULL DEFAULT ARRAY[]::TEXT[],
    has_images     BOOLEAN NOT NULL DEFAULT FALSE,
    has_videos     BOOLEAN NOT NULL DEFAULT FALSE,
    is_reply       BOOLEAN NOT NULL DEFAULT FALSE,
    is_root_reply  BOOLEAN NOT NULL DEFAULT FALSE,
    is_quote       BOOLEAN NOT NULL DEFAULT FALSE,
    has_references BOOLEAN NOT NULL DEFAULT FALSE,
    deleted        BOOLEAN NOT NULL DEFAULT FALSE,
    hidden         BOOLEAN NOT NULL DEFAULT FALSE
) WITH (FILLFACTOR = 70);
--------
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_events_address ON events(address);
--------
create unique index if not exists transferable_replaceable_event_uk on events(h_tag)
  where kind = 31750;