-- SPDX-License-Identifier: ice License 1.0

CREATE VIRTUAL TABLE if not exists events_search USING fts5(content, content_metadata, content='events', content_rowid=rid);
--------
CREATE TRIGGER if not exists trigger_events_after_insert_search_index 
    AFTER INSERT
    ON events 
    for each row
    when (NEW.kind = 0 and NEW.content != '' and json_valid(NEW.content) and (json_extract(NEW.content, '$.name') != '' or json_extract(NEW.content, '$.display_name') != ''))
    or (NEW.kind in (1, 30175, 30023) and (NEW.content != '' or NEW.content_metadata != '')) or (NEW.kind in (1063) and NEW.content_metadata != '')
BEGIN
  INSERT INTO events_search(rowid, content, content_metadata) VALUES (NEW.rid, NEW.content, NEW.content_metadata);
END;
--------
CREATE TRIGGER if not exists trigger_events_after_delete_search_index AFTER DELETE ON events BEGIN
    DELETE FROM events_search WHERE rowid = old.rid;
END;
