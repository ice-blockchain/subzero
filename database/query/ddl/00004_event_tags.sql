-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS event_tags
(
    id                bigserial,
    event_id          text not null references events (id) ON UPDATE RESTRICT ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_tag_key     text not null,
    event_tag_value1  text not null DEFAULT '',
    event_tag_value2  text not null DEFAULT '',
    event_tag_value3  text not null DEFAULT '',
    event_tag_value4  text not null DEFAULT '',
    event_tag_value5  text not null DEFAULT '',
    primary key (event_id, event_tag_key, event_tag_value1, event_tag_value3)
) WITH (FILLFACTOR = 90);
--------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'event_tags_event_id_fkey_v2') THEN
        ALTER TABLE event_tags
            DROP CONSTRAINT IF EXISTS event_tags_event_id_fkey;
        ALTER TABLE event_tags
            ADD CONSTRAINT event_tags_event_id_fkey_v2 FOREIGN KEY (event_id)
            REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED;
    END IF;
END $$;

