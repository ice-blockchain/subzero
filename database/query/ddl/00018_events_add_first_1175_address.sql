-- SPDX-License-Identifier: ice License 1.0

ALTER TABLE events ADD COLUMN IF NOT EXISTS first_1175_address TEXT
    REFERENCES events (id) ON UPDATE CASCADE ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE replaceable_events_before_update ADD COLUMN IF NOT EXISTS first_1175_address TEXT;

CREATE OR REPLACE FUNCTION trigger_events_before_insert_tc_link_first_buy_action()
RETURNS TRIGGER AS $$
BEGIN
    NEW.first_1175_address := (
        select
            e.id
        from
            events e
        where
            e.master_pubkey = NEW.master_pubkey
            and e.kind = NEW.kind
            and e.hidden = FALSE
            and e.first_1175_address is NULL
            and e.tags @> (
                SELECT
                    jsonb_agg(elem)
                FROM
                    jsonb_array_elements(NEW.tags) elem
                WHERE
                    elem ->> 0 IN ('network', 'token_address')
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
