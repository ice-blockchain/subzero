// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func helperUnixNano(t *testing.T, val int64) time.Time {
	t.Helper()

	return time.Unix(val/1e9, val%1e9)
}

func TestParseTimestamp(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Value    string
		Expected time.Time
	}{
		{"1748448074", time.Unix(1748448074, 0)},                        // Seconds.
		{"1748448074001", time.UnixMilli(1748448074001)},                // Milliseconds.
		{"1748448074000002", time.UnixMicro(1748448074000002)},          // Microseconds.
		{"1748448074000000003", helperUnixNano(t, 1748448074000000003)}, // Nanoseconds.
	}

	for _, c := range cases {
		t.Run(c.Value, func(t *testing.T) {
			parsed, err := parseTimestamp(c.Value)
			require.NoError(t, err)
			require.Truef(t, c.Expected.Equal(parsed.Time()), "Expected %v, got %v", c.Expected, parsed.Time())
		})
	}
}
