-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION trigger_events_before_insert_tc_link_first_buy_action()
RETURNS TRIGGER AS $$
BEGIN
    NEW.first_1175_address := (
        select
            e.id
        from
            events e
        inner join event_tags et on et.event_id = e.id
        where
            e.kind = NEW.kind
            and e.hidden = FALSE
            and e.first_1175_address is NULL
            and et.event_tag_key IN ('e', 'a')
            and et.event_tag_value1 = (
                select
                    elem ->> 1 AS tag_value
                from
                    jsonb_array_elements(NEW.tags) elem
                where
                    elem ->> 0 = et.event_tag_key
                    and jsonb_array_length(elem) > 1
                limit 1
            )
        order by
            e.lookup_created_at asc
        limit
            1
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_tc_link_first_buy_action
BEFORE INSERT ON events
FOR EACH ROW
WHEN (
    NEW.kind = 1175
    AND NEW.first_1175_address IS NULL
    AND NEW.hidden = FALSE
)
EXECUTE FUNCTION trigger_events_before_insert_tc_link_first_buy_action();

CREATE INDEX IF NOT EXISTS idx_events_kind_master_pubkey_first_1175_address_lookup_created_at
    ON events (kind, master_pubkey, first_1175_address NULLS FIRST, lookup_created_at ASC)
    where
        kind = 1175
        and hidden = FALSE;
