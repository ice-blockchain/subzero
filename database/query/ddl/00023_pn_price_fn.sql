-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION subzero_get_token_price_from_tx(tags JSONB)
RETURNS NUMERIC
LANGUAGE sql
IMMUTABLE
RETURNS NULL ON NULL INPUT
AS $$
    WITH base AS (
        SELECT
            MAX(CASE WHEN tag->>0 = 'token_symbol' THEN tag->>1 END)                                   AS sym,
            MAX(CASE WHEN tag->>0 = 'tx_amount' AND tag->>2 = 'USD' THEN CAST(tag->>1 AS NUMERIC) END) AS usd_amount
        FROM
            jsonb_array_elements(tags) AS tag
        WHERE
            tag->>0 IN ('token_symbol', 'tx_amount')
    )
    SELECT
        base.usd_amount / NULLIF(subzero_get_tx_amount(tags, base.sym), 0)
    FROM
        base
    WHERE
        base.sym IS NOT NULL
$$;
