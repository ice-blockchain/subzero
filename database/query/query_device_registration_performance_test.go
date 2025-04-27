// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestLargeDatasetPerformance(t *testing.T) {
	t.Skip()
	var (
		totalEvents            int
		deviceRegEvents        int
		tokenTagDeviceRegCount int
		invalidTokensCount     int
		insertBatchSize        int
		otherTagsCount         int
	)

	// full dataset
	totalEvents = 1000000
	deviceRegEvents = 400000 // 40% of total
	tokenTagDeviceRegCount = 1000000
	invalidTokensCount = 250000
	insertBatchSize = 1000
	otherTagsCount = 9000000

	// Small dataset for testing
	// totalEvents = 10000
	// deviceRegEvents = 2000
	// tokenTagDeviceRegCount = 1000
	// invalidTokensCount = 200
	// insertBatchSize = 50
	// otherTagsCount = 1000

	t.Logf("Test parameters:")
	t.Logf("- Events: total %d, DeviceRegistration %d", totalEvents, deviceRegEvents)
	t.Logf("- Token tags: planned %d, invalid %d", tokenTagDeviceRegCount, invalidTokensCount)
	t.Logf("- Other tags: %d", otherTagsCount)
	t.Logf("- Insertion batch size: %d", insertBatchSize)

	db := helperNewDatabase(t)
	defer db.Close()

	_, err := db.DB.ExecContext(t.Context(), `
		CREATE INDEX IF NOT EXISTS idx_event_tags_token_invalid ON event_tags(event_tag_key, event_tag_value2) 
		WHERE event_tag_key = 'token' AND event_tag_value2 = 'invalid'`)
	require.NoError(t, err, "Failed to create token_invalid index")

	_, err = db.DB.ExecContext(t.Context(), `
		CREATE INDEX IF NOT EXISTS idx_event_tags_composite ON event_tags(event_id, event_tag_key, event_tag_value2)`)
	require.NoError(t, err, "Failed to create composite event_tags index")

	_, err = db.DB.ExecContext(t.Context(), `
		CREATE INDEX IF NOT EXISTS idx_event_tags_token_empty ON event_tags(event_id) 
		WHERE event_tag_key = 'token' AND event_tag_value2 = ''`)
	require.NoError(t, err, "Failed to create token_empty index")

	deviceRegKind := model.CustomIONKindDeviceRegistration
	_, err = db.DB.ExecContext(t.Context(),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_events_kind_token_filter ON events(kind, created_at, id) WHERE kind = %d`, deviceRegKind))
	require.NoError(t, err, "Failed to create kind_filter index")

	t.Run("generate_large_dataset", func(t *testing.T) {
		counts, err := helperGenerateLargeDataset(t, t.Context(), db, totalEvents, deviceRegEvents,
			tokenTagDeviceRegCount, invalidTokensCount, insertBatchSize, otherTagsCount)
		require.NoError(t, err, "Failed to generate dataset")

		require.Greater(t, counts.totalEvents, 0, "Total events count should be > 0")
		require.Greater(t, counts.deviceRegEvents, 0, "DeviceRegistration events count should be > 0")

		var tokenTagsCount int
		err = db.DB.QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM event_tags WHERE event_tag_key = 'token'`).Scan(&tokenTagsCount)
		require.NoError(t, err)
		require.Greater(t, tokenTagsCount, 0, "Token tags count should be > 0")

		var invalidTokensActual int
		err = db.DB.QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM event_tags WHERE event_tag_key = 'token' AND event_tag_value2 = 'invalid'`).Scan(&invalidTokensActual)
		require.NoError(t, err)
		require.Greater(t, invalidTokensActual, 0, "Invalid tokens count should be > 0")

		var validTokensActual int
		err = db.DB.QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM event_tags WHERE event_tag_key = 'token' AND event_tag_value2 != 'invalid'`).Scan(&validTokensActual)
		require.NoError(t, err)
		require.Greater(t, validTokensActual, 0, "Valid tokens count should be > 0")

		var otherTagsActual int
		err = db.DB.QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM event_tags WHERE event_tag_key != 'token'`).Scan(&otherTagsActual)
		require.NoError(t, err)

		var totalTagsActual int
		err = db.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_tags`).Scan(&totalTagsActual)
		require.NoError(t, err)

		t.Logf("Test dataset data:")
		t.Logf("- Events: total %d, DeviceRegistration %d", counts.totalEvents, counts.deviceRegEvents)
		t.Logf("- Token tags: total %d, valid %d, invalid %d", tokenTagsCount, validTokensActual, invalidTokensActual)
		t.Logf("- Other tags: %d", otherTagsActual)
		t.Logf("- Total tags: %d", totalTagsActual)

		t.Logf("Requirements compliance:")
		t.Logf("- Events: %d of 1,000,000 (%.1f%%)", counts.totalEvents, float64(counts.totalEvents)/1000000.0*100)
		t.Logf("- Token tags: %d of 1,000,000 (%.1f%%)", tokenTagsCount, float64(tokenTagsCount)/1000000.0*100)
		t.Logf("- Other tags: %d of 9,000,000 (%.1f%%)", otherTagsActual, float64(otherTagsActual)/9000000.0*100)

		counts.invalidTokensCount = invalidTokensActual
	})

	t.Run("compare_performance", func(t *testing.T) {
		validEvents := helperCountValidDeviceRegistrationEvents(t, t.Context(), db)

		t.Logf("Found %d valid device registration events to test with", validEvents)

		// Method 1: No join
		helperClearCache(t, db)
		start := time.Now()
		eventsCount1 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsNoJoin(t, db))
		duration1 := time.Since(start)
		helperLogMethodPerformance(t, "Method 1 (NO JOIN)", eventsCount1, duration1)

		// Method 2: INNER JOIN
		helperClearCache(t, db)
		start = time.Now()
		eventsCount2 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsJoin(t, db))
		duration2 := time.Since(start)
		helperLogMethodPerformance(t, "Method 2 (FROM events JOIN event_tags WHERE val2 != 'invalid')", eventsCount2, duration2)

		// Method 3: INNER JOIN with subquery
		helperClearCache(t, db)
		start = time.Now()
		eventsCount3 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsInnerJoinWithSubquery(t, db))
		duration3 := time.Since(start)
		helperLogMethodPerformance(t, "Method 3 (INNER JOIN with subquery)", eventsCount3, duration3)

		// Method 4: LATERAL JOIN
		helperClearCache(t, db)
		start = time.Now()
		eventsCount4 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsLateral(t, db))
		duration4 := time.Since(start)
		helperLogMethodPerformance(t, "Method 4 (LATERAL JOIN)", eventsCount4, duration4)

		// Method 5: EXISTS
		helperClearCache(t, db)
		start = time.Now()
		eventsCount5 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsExists(t, db))
		duration5 := time.Since(start)
		helperLogMethodPerformance(t, "Method 5 (EXISTS)", eventsCount5, duration5)

		// Method 6: RIGHT JOIN LOOKING FOR TOKENS != 'invalid'
		helperClearCache(t, db)
		start = time.Now()
		eventsCount6 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsRightJoin(t, db))
		duration6 := time.Since(start)
		helperLogMethodPerformance(t, "Method 6 (FROM event_tags RIGHT JOIN val2 != 'invalid')", eventsCount6, duration6)

		// Method 7: EMPTY VALUE2 FROM event_tags JOIN
		helperClearCache(t, db)
		start = time.Now()
		eventsCount7 := helperCountEventsFromIterator(helperCollectDeviceRegistrationEventsEmptyValueJoin(t, db))
		duration7 := time.Since(start)
		helperLogMethodPerformance(t, "Method 7 (FROM event_tags RIGHT JOIN val2 = '')", eventsCount7, duration7)

		// Check that all methods return the same number of events
		for i := 2; i <= 7; i++ {
			require.Equal(t, eventsCount1, helperCountEventsByMethod(i, eventsCount2, eventsCount3, eventsCount4, eventsCount5, eventsCount6, eventsCount7),
				"Method %d returns different number of events than Method 1", i)
		}

		t.Logf("All methods return %d events with valid token", eventsCount1)

		t.Log("Performance comparison:")

		expectedEvents := validEvents
		// For large datasets allow small deviation (±1%)
		tolerance := expectedEvents / 100
		require.InDelta(t, expectedEvents, eventsCount1, float64(tolerance), "Method 1 returned wrong number of events")
		require.InDelta(t, expectedEvents, eventsCount2, float64(tolerance), "Method 2 returned wrong number of events")
		require.InDelta(t, expectedEvents, eventsCount3, float64(tolerance), "Method 3 returned wrong number of events")
		require.InDelta(t, expectedEvents, eventsCount4, float64(tolerance), "Method 4 returned wrong number of events")
		require.InDelta(t, expectedEvents, eventsCount5, float64(tolerance), "Method 5 returned wrong number of events")
		require.InDelta(t, expectedEvents, eventsCount6, float64(tolerance), "Method 6 returned wrong number of events")
		require.InDelta(t, expectedEvents, eventsCount7, float64(tolerance), "Method 7 returned wrong number of events")

		durations := []struct {
			name     string
			duration time.Duration
			count    int
		}{
			{"Method 1 (NO JOIN)", duration1, eventsCount1},
			{"Method 2 (FROM events JOIN event_tags WHERE val2 != 'invalid')", duration2, eventsCount2},
			{"Method 3 (INNER JOIN with subquery)", duration3, eventsCount3},
			{"Method 4 (LATERAL JOIN)", duration4, eventsCount4},
			{"Method 5 (EXISTS)", duration5, eventsCount5},
			{"Method 6 (FROM event_tags RIGHT JOIN val2 != 'invalid')", duration6, eventsCount6},
			{"Method 7 (FROM event_tags RIGHT JOIN val2 = '')", duration7, eventsCount7},
		}

		sort.Slice(durations, func(i, j int) bool {
			return durations[i].duration < durations[j].duration
		})

		fastestDuration := durations[0].duration
		fastestMethod := durations[0].name

		t.Logf("%s is fastest:", fastestMethod)

		for i := 1; i < len(durations); i++ {
			speedup := float64(durations[i].duration) / float64(fastestDuration)
			t.Logf("- %.2fx faster than %s", speedup, durations[i].name)
		}
	})
}

type EventCounts struct {
	totalEvents        int
	deviceRegEvents    int
	invalidTokensCount int
	validTokensCount   int
	otherTagsCount     int
}

func helperGenerateLargeDataset(t *testing.T, ctx context.Context, db *dbClient, totalEvents, deviceRegEvents,
	tokenTagDeviceRegCount, invalidTokensCount, insertBatchSize, otherTagsCount int) (*EventCounts, error) {
	t.Helper()

	deviceRegEventIDs := make([]string, 0, deviceRegEvents)
	allEventIDs := make([]string, 0, totalEvents)

	fmt.Printf("Generating %d events (%d device registrations)...\n", totalEvents, deviceRegEvents)

	for i := 0; i < deviceRegEvents; i++ {
		eventID := fmt.Sprintf("devreg%d", i)
		deviceRegEventIDs = append(deviceRegEventIDs, eventID)
		allEventIDs = append(allEventIDs, eventID)
	}

	regularEventIDs := make([]string, 0, totalEvents-deviceRegEvents)
	for i := 0; i < totalEvents-deviceRegEvents; i++ {
		eventID := fmt.Sprintf("regular%d", i)
		regularEventIDs = append(regularEventIDs, eventID)
		allEventIDs = append(allEventIDs, eventID)
	}

	for i := 0; i < deviceRegEvents; i += insertBatchSize {
		endIdx := helperMinInt(t, i+insertBatchSize, deviceRegEvents)
		count := endIdx - i

		query := `
			INSERT INTO events (
				id, 
				pubkey, 
				master_pubkey,
				created_at, 
				kind, 
				content,
				d_tag,
				tags,
				sig,
				sig_alg,
				key_alg
			) VALUES 
		`

		values := make([]interface{}, 0, count*11)
		paramIdx := 1

		for j := 0; j < count; j++ {
			idx := i + j
			pubKey := "pubkey" + strconv.Itoa(idx%1000)
			masterPubKey := pubKey
			deviceID := "device" + strconv.Itoa(idx)
			eventID := deviceRegEventIDs[idx]
			createdAt := time.Now().Unix() - int64(deviceRegEvents) + int64(idx)
			content := `{"kinds":[1]}`
			tags := fmt.Sprintf(`[["d","%s"],["t","android"],["token","token_%d"]]`, deviceID, idx)

			if j > 0 {
				query += ", "
			}

			query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
				paramIdx, paramIdx+1, paramIdx+2, paramIdx+3, paramIdx+4, paramIdx+5, paramIdx+6, paramIdx+7, paramIdx+8, paramIdx+9, paramIdx+10)

			values = append(values,
				eventID,
				pubKey,
				masterPubKey,
				time.Unix(createdAt, 0),
				model.CustomIONKindDeviceRegistration,
				content,
				deviceID,
				tags,
				"sig_"+eventID,
				"sig_alg_v1",
				"key_alg_v1",
			)
			paramIdx += 11
		}

		_, err := db.DB.ExecContext(ctx, query, values...)
		require.NoError(t, err, "failed to insert device registration events")

		fmt.Printf("Generated %d/%d device registration events\n", endIdx, deviceRegEvents)
	}

	addedTokens := 0
	totalTokensToAdd := helperMinInt(t, tokenTagDeviceRegCount, deviceRegEvents)

	for i := 0; i < totalTokensToAdd; i++ {
		eventID := deviceRegEventIDs[i]
		tokenValue := "token_" + strconv.Itoa(i)

		_, err := db.DB.ExecContext(ctx, `
			INSERT INTO event_tags (
				event_id, 
				event_tag_key, 
				event_tag_value1,
				event_tag_value2
			) VALUES ($1, $2, $3, $4)
			ON CONFLICT (event_id, event_tag_key, event_tag_value1) DO NOTHING
		`, eventID, "token", tokenValue, "")

		require.NoError(t, err, "failed to insert valid token tags")
		addedTokens++
	}

	fmt.Printf("Added %d valid token tags\n", addedTokens)

	invalidTokensToAdd := helperMinInt(t, invalidTokensCount, totalTokensToAdd)

	if invalidTokensToAdd >= totalTokensToAdd {
		invalidTokensToAdd = totalTokensToAdd / 2
	}

	_, err := db.DB.ExecContext(ctx, `
		UPDATE event_tags 
		SET event_tag_value2 = 'invalid'
		WHERE event_id IN (
			SELECT event_id
			FROM event_tags
			WHERE event_tag_key = 'token' 
			ORDER BY event_id
			LIMIT $1
		)
	`, invalidTokensToAdd)

	require.NoError(t, err, "failed to mark tokens as invalid")

	fmt.Printf("Marked %d tokens as invalid\n", invalidTokensToAdd)

	validDeviceRegCount := addedTokens - invalidTokensToAdd

	otherEvents := totalEvents - deviceRegEvents
	for i := 0; i < otherEvents; i += insertBatchSize {
		endIdx := helperMinInt(t, i+insertBatchSize, otherEvents)
		count := endIdx - i

		query := `
			INSERT INTO events (
				id, 
				pubkey,
				master_pubkey,
				created_at, 
				kind, 
				content,
				tags,
				sig,
				sig_alg,
				key_alg
			) VALUES 
		`

		values := make([]interface{}, 0, count*10)
		paramIdx := 1

		for j := 0; j < count; j++ {
			idx := i + j
			pubKey := "pubkey" + strconv.Itoa(idx)
			masterPubKey := pubKey
			eventID := regularEventIDs[idx]
			createdAt := time.Now().Unix() - int64(otherEvents) + int64(idx)
			content := `{"data":"test"}`

			possibleKinds := []int{1, 2, 4, 5}
			kind := possibleKinds[idx%len(possibleKinds)]

			if j > 0 {
				query += ", "
			}

			query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
				paramIdx, paramIdx+1, paramIdx+2, paramIdx+3, paramIdx+4, paramIdx+5, paramIdx+6, paramIdx+7, paramIdx+8, paramIdx+9)

			values = append(values,
				eventID,
				pubKey,
				masterPubKey,
				time.Unix(createdAt, 0),
				kind,
				content,
				"[]",
				"sig_"+eventID,
				"sig_alg_v1",
				"key_alg_v1",
			)
			paramIdx += 10
		}

		_, err := db.DB.ExecContext(ctx, query, values...)
		require.NoError(t, err, "failed to insert regular events")

		fmt.Printf("Generated %d/%d regular events\n", endIdx, otherEvents)
	}

	if otherTagsCount > 0 {
		fmt.Printf("Adding %d additional tags...\n", otherTagsCount)
		allEventIDsCombined := append(deviceRegEventIDs, regularEventIDs...)

		for i := 0; i < otherTagsCount; i += insertBatchSize {
			endIdx := helperMinInt(t, i+insertBatchSize, otherTagsCount)
			count := endIdx - i

			query := `
				INSERT INTO event_tags (
					event_id, 
					event_tag_key, 
					event_tag_value1, 
					event_tag_value2
				) VALUES 
			`

			values := make([]interface{}, 0, count*4)
			paramIdx := 1

			for j := 0; j < count; j++ {
				idx := i + j
				eventIdx := idx % len(allEventIDsCombined)
				eventID := allEventIDsCombined[eventIdx]

				tagType := idx % 5
				tagKey := fmt.Sprintf("test_tag_%d", tagType)
				tagValue := fmt.Sprintf("value_%d", idx)

				if j > 0 {
					query += ", "
				}

				query += fmt.Sprintf("($%d, $%d, $%d, $%d)",
					paramIdx, paramIdx+1, paramIdx+2, paramIdx+3)

				values = append(values,
					eventID,
					tagKey,
					tagValue,
					"other",
				)
				paramIdx += 4
			}

			_, err := db.DB.ExecContext(ctx, query, values...)
			require.NoError(t, err, "failed to insert additional tags")

			fmt.Printf("Added %d/%d additional tags\n", endIdx, otherTagsCount)
		}
	}

	var totalEventsCount int64
	err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&totalEventsCount)
	require.NoError(t, err, "failed to count events")

	var deviceRegEventsCount int64
	err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE kind = $1", model.CustomIONKindDeviceRegistration).Scan(&deviceRegEventsCount)
	require.NoError(t, err, "failed to count device registration events")

	fmt.Printf("Verified events in database: total %d, device registrations %d\n", totalEventsCount, deviceRegEventsCount)
	if totalEventsCount == 0 || deviceRegEventsCount == 0 {
		return nil, errors.New("no events were actually inserted into the database")
	}

	var invalidCount int64
	err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_tags WHERE event_tag_key = 'token' AND event_tag_value2 = 'invalid'").Scan(&invalidCount)
	require.NoError(t, err, "failed to count invalid tokens")

	fmt.Printf("Valid device registration events: %d (of %d total)\n", validDeviceRegCount, deviceRegEventsCount)

	return &EventCounts{
		totalEvents:        int(totalEventsCount),
		deviceRegEvents:    int(deviceRegEventsCount),
		invalidTokensCount: int(invalidCount),
		validTokensCount:   validDeviceRegCount,
		otherTagsCount:     otherTagsCount,
	}, nil
}

func helperCountEventsByMethod(methodNum int, count2, count3, count4, count5, count6, count7 int) int {
	switch methodNum {
	case 2:
		return count2
	case 3:
		return count3
	case 4:
		return count4
	case 5:
		return count5
	case 6:
		return count6
	case 7:
		return count7
	default:
		return 0
	}
}

func helperCountValidDeviceRegistrationEvents(t *testing.T, ctx context.Context, db *dbClient) int {
	t.Helper()
	var validDeviceRegCount int64
	err := db.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM events e
		WHERE e.kind = $1
			AND EXISTS (
				SELECT 1
				FROM event_tags et
				WHERE et.event_id = e.id 
					AND et.event_tag_key = 'token' 
					AND et.event_tag_value2 != 'invalid'
			)
	`, model.CustomIONKindDeviceRegistration).Scan(&validDeviceRegCount)
	require.NoError(t, err)
	require.Greater(t, validDeviceRegCount, int64(0), "Must have at least one valid device registration event")
	return int(validDeviceRegCount)
}

func helperCountEventsFromIterator(it EventIterator) int {
	var count int
	it(func(event *model.Event, err error) bool {
		if err != nil {
			return false
		}
		if event != nil {
			count++
		}
		return true
	})
	return count
}

func helperLogMethodPerformance(t *testing.T, methodName string, eventsCount int, duration time.Duration) {
	t.Helper()
	eventsPerSec := 0.0
	if duration.Seconds() > 0 {
		eventsPerSec = float64(eventsCount) / duration.Seconds()
	}

	t.Logf("%s: %d events in %v (%.2f events/sec)",
		methodName, eventsCount, duration, eventsPerSec)
}

func helperClearCache(t *testing.T, db *dbClient) {
	t.Helper()
	_, err := db.DB.ExecContext(t.Context(), "SELECT 1")
	require.NoError(t, err)
}

// helperQueryWithIteratorBase performs common database query and iteration logic
// for all helper functions that collect device registration events
func helperQueryWithIteratorBase(t *testing.T, db *dbClient, sqlQuery string) EventIterator {
	t.Helper()
	const batchSize = 1000

	return func(yield func(*model.Event, error) bool) {
		var lastCreatedAt time.Time
		var lastID string

		for t.Context().Err() == nil {
			params := map[string]any{
				"kind":            model.CustomIONKindDeviceRegistration,
				"last_created_at": lastCreatedAt.UTC(),
				"last_id":         lastID,
				"batch_size":      batchSize,
			}

			var eventsProcessed int
			it := db.newReadEventIterator(t.Context(), sqlQuery, params)
			for event, iterErr := range it {
				if iterErr != nil {
					if !yield(nil, errors.Wrap(iterErr, "failed to iterate device registration events")) {
						return
					}
					break
				}
				if !yield(event, nil) {
					return
				}

				lastCreatedAt = time.Unix(int64(event.CreatedAt), 0)
				lastID = event.ID
				eventsProcessed++
			}
			if eventsProcessed < batchSize {
				break
			}
		}
	}
}

func helperCollectDeviceRegistrationEventsNoJoin(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	t.Helper()
	sqlQuery := `SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM events e
		WHERE e.kind = :kind
			AND e.id IN (SELECT event_id FROM event_tags WHERE event_tag_key = 'token' AND event_tag_value2 = '')
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}

// helperCollectDeviceRegistrationEventsJoin uses INNER JOIN for filtering device registration events
func helperCollectDeviceRegistrationEventsJoin(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	sqlQuery := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM events e
		JOIN event_tags et ON e.id = et.event_id
		WHERE e.kind = :kind
			AND et.event_tag_key = 'token'
			AND et.event_tag_value2 != 'invalid'
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size
	`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}

// helperCollectDeviceRegistrationEventsInnerJoinWithSubquery uses INNER JOIN with subquery
func helperCollectDeviceRegistrationEventsInnerJoinWithSubquery(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	sqlQuery := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM events e
		JOIN (
			SELECT DISTINCT e2.id, e2.created_at
			FROM events e2
			JOIN event_tags et ON e2.id = et.event_id
			WHERE e2.kind = :kind
				AND et.event_tag_key = 'token'
				AND et.event_tag_value2 != 'invalid'
				AND (e2.created_at > :last_created_at OR (e2.created_at = :last_created_at AND e2.id > :last_id))
			ORDER BY e2.created_at ASC, e2.id
			LIMIT :batch_size
		) valid_events ON e.id = valid_events.id
		ORDER BY e.created_at ASC, e.id
	`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}

// helperCollectDeviceRegistrationEventsLateral uses LATERAL JOIN for improved performance
// by more efficiently excluding invalid events
func helperCollectDeviceRegistrationEventsLateral(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	sqlQuery := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM events e
		JOIN LATERAL (
			SELECT 1 as valid_token
			FROM event_tags et 
			WHERE et.event_id = e.id 
				AND et.event_tag_key = 'token' 
				AND et.event_tag_value2 != 'invalid'
			LIMIT 1
		) valid_check ON true
		WHERE e.kind = :kind
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size
	`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}

// helperCollectDeviceRegistrationEventsExists uses EXISTS instead of JOIN
// to select events that have a token tag with valid value
func helperCollectDeviceRegistrationEventsExists(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	sqlQuery := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM events e
		WHERE e.kind = :kind
			AND EXISTS (
				SELECT 1
				FROM event_tags et 
				WHERE et.event_id = e.id 
					AND et.event_tag_key = 'token' 
					AND et.event_tag_value2 != 'invalid'
			)
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size
	`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}

// helperCollectDeviceRegistrationEventsRightJoin uses RIGHT JOIN to filter device registration events
// and returns only events that have a valid token
func helperCollectDeviceRegistrationEventsRightJoin(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	sqlQuery := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM event_tags et
		RIGHT JOIN events e ON et.event_id = e.id AND et.event_tag_key = 'token' AND et.event_tag_value2 != 'invalid'
		WHERE e.kind = :kind
			AND et.event_id IS NOT NULL
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size
	`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}

// helperCollectDeviceRegistrationEventsEmptyValueJoin uses JOIN
// to select valid events where event_tag_value2 is not 'invalid'.
// This method selects only events that have a token tag and its value is not 'invalid'.
func helperCollectDeviceRegistrationEventsEmptyValueJoin(t *testing.T, db *dbClient) EventIterator {
	t.Helper()
	sqlQuery := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags
		FROM event_tags et
		JOIN events e ON et.event_id = e.id
		WHERE e.kind = :kind
			AND et.event_tag_key = 'token'
			AND et.event_tag_value2 = ''
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size
	`
	return helperQueryWithIteratorBase(t, db, sqlQuery)
}
