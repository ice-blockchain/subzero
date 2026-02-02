// SPDX-License-Identifier: ice License 1.0

package model

import (
	"net/url"
	"strings"
)

// CompareRelaysURLs compares two relay URLs for equality, ignoring case and port differences.
func CompareRelaysURLs(relay1, relay2 string) bool {
	if strings.EqualFold(relay1, relay2) {
		return true
	}

	parsed1, err1 := url.Parse(relay1)
	parsed2, err2 := url.Parse(relay2)
	if err1 != nil || err2 != nil {
		return false
	}

	return strings.EqualFold(parsed1.Scheme, parsed2.Scheme) &&
		strings.EqualFold(parsed1.Path, parsed2.Path) &&
		strings.EqualFold(parsed1.Hostname(), parsed2.Hostname()) // Ignore port differences.
}
