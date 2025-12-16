-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE TRIGGER trigger_events_store_replaceable_data_before_update
    AFTER UPDATE ON events
    FOR EACH ROW
    WHEN (((10000 <= old.kind AND old.kind < 20000 ) OR old.kind = 0 OR old.kind = 3 OR (30000 <= old.kind AND old.kind < 40000)) AND old.id != new.id)
    EXECUTE FUNCTION events_store_replaceable_data_before_update();
--------
CREATE OR REPLACE FUNCTION events_store_replaceable_data_before_update()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.reference_id = NEW.id THEN
        NEW.reference_id = NULL;
        RETURN NEW; -- dont insert into replaceable_events_before_update, its replay
    END IF;
    insert into replaceable_events_before_update (
        created_at,
        lookup_created_at,
        expiration,
        kind,
        lookup,
        key_alg,
        content,
        d_tag,
        h_tag,
        address,
        id,
        system_id,
        pubkey,
        master_pubkey,
        sig,
        sig_alg,
        reference_id,
        tags,
        t_tags,
        gift_receiver_pubkey,
        has_images,
        has_videos,
        deleted,
        is_reply,
        is_root_reply,
        is_quote,
        has_ephemeral_attestation,
        has_references,
        hidden,
        verified,
        lang,
        replaced_by_id
    )
    values (
            old.created_at,
            old.lookup_created_at,
            old.expiration,
            old.kind,
            old.lookup,
            old.key_alg,
            old.content,
            old.d_tag,
            old.h_tag,
            old.address,
            old.id,
            old.system_id,
            old.pubkey,
            old.master_pubkey,
            old.sig,
            old.sig_alg,
            old.reference_id,
            old.tags,
            old.t_tags,
            old.gift_receiver_pubkey,
            old.has_images,
            old.has_videos,
            old.deleted,
            old.is_reply,
            old.is_root_reply,
            old.is_quote,
            old.has_ephemeral_attestation,
            old.has_references,
            old.hidden,
            old.verified,
            old.lang,
            new.id
           )
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
