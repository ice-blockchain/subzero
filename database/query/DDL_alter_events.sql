-- SPDX-License-Identifier: ice License 1.0

ALTER TABLE events ADD COLUMN h_tag text not null DEFAULT '';
--------
UPDATE events SET h_tag = id WHERE h_tag = NULL OR h_tag = '';
--------
CREATE UNIQUE INDEX IF NOT EXISTS uix_events_h_tag ON events(h_tag);
--------
drop   trigger if     exists trigger_events_after_update_generate_tags;
--------
create trigger if not exists trigger_events_before_insert_unwind_repost
    before insert
    on events
    for each row
    when new.kind in (6, 16)
begin
insert into events
    (kind, created_at, system_created_at, id, pubkey, master_pubkey, sig, content, tags, d_tag, h_tag, hidden)
select
    json_extract(b, '$.kind'),
    0,
    0,
    json_extract(b, '$.id'),
    '',
    '',
    '',
    '',
    json_extract(b, '$.tags'),
    '',
    json_extract(b, '$.id'),
    1
from
    (select NEW.content as b)
where
    NEW.content != '' AND json_valid(NEW.content)
on conflict do nothing;
end
;