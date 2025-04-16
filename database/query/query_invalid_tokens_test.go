// SPDX-License-Identifier: ice License 1.0

package query

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidTokens(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	deviceID := "test-device-1"
	masterPubKey := "test-pubkey-1"
	token := "test-token-1"

	require.False(t, helperIsTokenInvalid(t, db, deviceID))

	t.Run("MarkTokenAsInvalid", func(t *testing.T) {
		err := db.markTokenAsInvalid(t.Context(), deviceID, masterPubKey, token)
		require.NoError(t, err)

		require.True(t, helperIsTokenInvalid(t, db, deviceID))
	})

	t.Run("GetInvalidTokens", func(t *testing.T) {
		deviceID2 := "test-device-2"
		masterPubKey2 := "test-pubkey-2"
		token2 := "test-token-2"

		err := db.markTokenAsInvalid(t.Context(), deviceID2, masterPubKey2, token2)
		require.NoError(t, err)

		tokens, err := db.getInvalidTokens(t.Context())
		require.NoError(t, err)
		require.Len(t, tokens, 2)

		found1, found2 := false, false
		for _, tok := range tokens {
			if tok.DeviceID == deviceID && tok.MasterPubKey == masterPubKey && tok.Token == token {
				found1 = true
			}
			if tok.DeviceID == deviceID2 && tok.MasterPubKey == masterPubKey2 && tok.Token == token2 {
				found2 = true
			}
		}
		require.True(t, found1, "First token not found in results")
		require.True(t, found2, "Second token not found in results")
	})

	t.Run("IsTokenInvalid", func(t *testing.T) {
		require.True(t, helperIsTokenInvalid(t, db, deviceID))

		require.False(t, helperIsTokenInvalid(t, db, "non-existent-device"))
	})

	t.Run("CleanupOldInvalidTokens", func(t *testing.T) {
		err := db.cleanupOldInvalidTokens(t.Context())
		require.NoError(t, err)

		tokens, err := db.getInvalidTokens(t.Context())
		require.NoError(t, err)
		require.NotEmpty(t, tokens)

		_, err = db.DB.ExecContext(t.Context(), `
			DELETE FROM invalid_device_tokens
			WHERE device_id = $1 OR device_id = $2
		`, deviceID, "test-device-2")
		require.NoError(t, err)

		tokens, err = db.getInvalidTokens(t.Context())
		require.NoError(t, err)
		require.Empty(t, tokens)
	})

	t.Run("UpdateExistingToken", func(t *testing.T) {
		err := db.markTokenAsInvalid(t.Context(), deviceID, masterPubKey, token)
		require.NoError(t, err)

		tokens, err := db.getInvalidTokens(t.Context())
		require.NoError(t, err)
		require.NotEmpty(t, tokens)
		require.Equal(t, token, tokens[0].Token)

		newToken := "updated-token"
		err = db.markTokenAsInvalid(t.Context(), deviceID, masterPubKey, newToken)
		require.NoError(t, err)

		tokens, err = db.getInvalidTokens(t.Context())
		require.NoError(t, err)
		require.NotEmpty(t, tokens)

		var foundToken *InvalidTokenInfo
		for i := range tokens {
			if tokens[i].DeviceID == deviceID {
				foundToken = &tokens[i]
				break
			}
		}

		require.NotNil(t, foundToken, "Token not found after update")
		require.Equal(t, newToken, foundToken.Token, "Token was not updated")
	})

	t.Run("BatchProcessingMoreThan10000", func(t *testing.T) {
		_, err := db.DB.ExecContext(t.Context(), `DELETE FROM invalid_device_tokens`)
		require.NoError(t, err)

		const totalRecords = 15000

		tx, err := db.DB.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(t.Context(), `
			INSERT INTO invalid_device_tokens (device_id, master_pubkey, token, created_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		`)
		require.NoError(t, err)
		defer stmt.Close()

		for i := 0; i < totalRecords; i++ {
			deviceID := fmt.Sprintf("batch-device-%d", i)
			masterPubKey := fmt.Sprintf("batch-pubkey-%d", i)
			token := fmt.Sprintf("batch-token-%d", i)

			_, err := stmt.ExecContext(t.Context(), deviceID, masterPubKey, token)
			require.NoError(t, err)
		}

		err = tx.Commit()
		require.NoError(t, err)

		tokens, err := db.getInvalidTokens(t.Context())
		require.NoError(t, err)

		require.Len(t, tokens, totalRecords, "Received incorrect number of tokens. Expected %d, got %d", totalRecords, len(tokens))

		deviceIDsMap := make(map[string]bool)
		for _, token := range tokens {
			deviceIDsMap[token.DeviceID] = true
		}

		require.True(t, deviceIDsMap["batch-device-0"], "Missing first record")
		require.True(t, deviceIDsMap["batch-device-9999"], "Missing record at the boundary of the first batch")
		require.True(t, deviceIDsMap["batch-device-10000"], "Missing first record of the second batch")
		require.True(t, deviceIDsMap["batch-device-14999"], "Missing last record")
	})
}

func helperIsTokenInvalid(t *testing.T, db *dbClient, deviceID string) bool {
	t.Helper()

	var exists bool
	err := db.DB.QueryRowContext(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM invalid_device_tokens WHERE device_id = $1)
	`, deviceID).Scan(&exists)
	require.NoError(t, err)

	return exists
}
