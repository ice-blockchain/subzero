// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"strconv"

	"github.com/nbd-wtf/go-nostr"
)

func parseTimestamp(value string) (nostr.Timestamp, error) {
	val, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}

	return nostr.Timestamp(val), nil
}
