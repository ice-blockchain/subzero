-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION trigger_events_set_profile_badges()
RETURNS TRIGGER AS $$
DECLARE
    has_verified_badge_def BOOLEAN;
    has_verified_award_id BOOLEAN;
    verified_badge_address TEXT;
BEGIN
    IF NOT (NEW.tags IS DISTINCT FROM OLD.tags) THEN
        RETURN NEW;
    END IF;

    -- Unpack the address of the verified badge definition from the event tags.
    SELECT
        event_tag_value1
    INTO
        verified_badge_address
    FROM
        event_tags
    WHERE
        event_id = NEW.id
        AND event_tag_key = 'a'
        AND starts_with(event_tag_value1, '30009:')
        AND event_tag_value1 LIKE '%:username_proof_of_ownership~%';

    IF verified_badge_address IS NULL THEN
        RETURN NEW;
    END IF;

    -- Check that unpacked badge definition exists and belongs to the current user.
    has_verified_badge_def := EXISTS(
        SELECT 1
        FROM
            events
        INNER JOIN event_tags ON events.id = event_tags.event_id
        WHERE
            kind = 30009
            AND address = verified_badge_address
            AND event_tags.event_tag_key = 'p'
            AND event_tags.event_tag_value1 = NEW.master_pubkey
            AND hidden = FALSE
    );

    IF NOT has_verified_badge_def THEN
        RETURN NEW;
    END IF;

    -- Find the award event issued for current user using the verified badge definition.
    has_verified_award_id := EXISTS(
        SELECT
            events.id
        FROM
            events
        INNER JOIN event_tags et_ref
            ON et_ref.event_id = NEW.id
            AND et_ref.event_tag_key = 'e'
            AND events.id = et_ref.event_tag_value1
        WHERE
            kind = 8
            AND EXISTS (
                SELECT 1
                FROM event_tags
                WHERE
                    event_id = events.id
                    AND event_tag_key = 'p'
                    AND event_tag_value1 = NEW.master_pubkey
            )
            AND EXISTS (
                SELECT 1
                FROM event_tags
                WHERE
                    event_id = events.id
                    AND event_tag_key = 'a'
                    AND event_tag_value1 = verified_badge_address
            )
            AND events.hidden = FALSE
    );

    IF NOT has_verified_award_id THEN
        RETURN NEW;
    END IF;

    -- Mark all relevant events of the user as verified.
    UPDATE events
    SET
        verified = TRUE
    WHERE
        master_pubkey = NEW.master_pubkey
        AND kind in (0, 1, 6, 16, 30008, 30023, 30175)
        AND hidden = FALSE
        AND verified = FALSE;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE TRIGGER trigger_events_after_insert_set_profile_badges
AFTER INSERT OR UPDATE ON events
FOR EACH ROW
WHEN (
    NEW.kind = 30008
    AND NEW.d_tag = 'profile_badges'
    AND NEW.has_ephemeral_attestation = FALSE
    AND NEW.verified = FALSE
)
EXECUTE FUNCTION trigger_events_set_profile_badges();
