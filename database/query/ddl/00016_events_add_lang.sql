-- SPDX-License-Identifier: ice License 1.0

ALTER TABLE events ADD COLUMN IF NOT EXISTS lang TEXT NOT NULL DEFAULT '';
ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS lang TEXT;

-- Populate lang column based on tags.
-- Format: [["l", "en", "ISO-639-1"]].
UPDATE events e
    SET lang = coalesce((
        select
            event_tag_value1
        from
            event_tags et
        where
            et.event_id = e.id
            and et.event_tag_key = 'l'
            and et.event_tag_value2 = 'ISO-639-1'
        limit 1
    ), '')
WHERE
    e.lang = ''
    and e.kind in (1, 6, 16, 30175, 30023)
    and e.hidden=false
    and e.deleted=false;

CREATE INDEX IF NOT EXISTS idx_events_verified_lookup_created_at_kind_lang ON events (verified desc, lookup_created_at DESC, kind, lang)
    WHERE hidden = FALSE AND deleted = false and kind in (1, 6, 16, 30175, 30023) and lang != '';
