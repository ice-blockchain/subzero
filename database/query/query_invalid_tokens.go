// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"time"
)

type InvalidTokenInfo struct {
	DeviceID     string
	MasterPubKey string
	Token        string
	CreatedAt    time.Time
}

func (db *dbClient) markTokenAsInvalid(ctx context.Context, deviceID, masterPubKey, token string) error {
	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO invalid_device_tokens (device_id, master_pubkey, token, created_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (device_id, master_pubkey) DO UPDATE SET
			token = $3,
			created_at = CURRENT_TIMESTAMP
	`, deviceID, masterPubKey, token)

	if err != nil {
		return fmt.Errorf("failed to mark token as invalid: %w", err)
	}

	return nil
}

func (db *dbClient) cleanupOldInvalidTokens(ctx context.Context) error {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		DELETE FROM invalid_device_tokens
		WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '24 hours'
	`)
	if err != nil {
		return fmt.Errorf("error deleting expired invalid tokens: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func (db *dbClient) getInvalidTokens(ctx context.Context) ([]InvalidTokenInfo, error) {
	var allTokens []InvalidTokenInfo
	batchSize := 10000
	offset := 0

	for {
		query := `
			SELECT device_id, master_pubkey, token, created_at
			FROM invalid_device_tokens
			ORDER BY device_id
			LIMIT $1 OFFSET $2
		`

		rows, err := db.DB.QueryContext(ctx, query, batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to query invalid tokens: %w", err)
		}

		batchTokens := make([]InvalidTokenInfo, 0, batchSize)
		for rows.Next() {
			var token InvalidTokenInfo
			if err := rows.Scan(&token.DeviceID, &token.MasterPubKey, &token.Token, &token.CreatedAt); err != nil {
				rows.Close()
				return nil, fmt.Errorf("error scanning invalid token: %w", err)
			}
			batchTokens = append(batchTokens, token)
		}

		rows.Close()

		if err = rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating over invalid tokens: %w", err)
		}

		allTokens = append(allTokens, batchTokens...)

		if len(batchTokens) < batchSize {
			break
		}

		offset += batchSize
	}

	return allTokens, nil
}
