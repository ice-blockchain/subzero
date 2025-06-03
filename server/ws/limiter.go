// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

const (
	defaultLimitREQ = 100
)

var (
	errLimitExceeded = errors.New("limit-exceeded: request exceeds the allowed limit")
	errNoFilter      = errors.New("no-filter: request does not contain a filter")
)

func applySubscriptionLimit(s *model.Subscription) error {
	var total int

	if len(s.Filters) == 0 {
		return errNoFilter
	}

	used, filtersWithoutLimit := 0, 0
	for i := range s.Filters {
		if s.Filters[i].Limit > 0 {
			used += s.Filters[i].Limit
		} else {
			filtersWithoutLimit++
		}
	}

	// Calculate limit per filter without limit.
	remainingLimit := defaultLimitREQ - used
	fairSharePerFilter := 0
	if filtersWithoutLimit > 0 {
		fairSharePerFilter = remainingLimit / filtersWithoutLimit
	}

	// Apply limits and calculate total.
	total = 0
	for i := range s.Filters {
		if s.Filters[i].Limit <= 0 {
			if fairSharePerFilter == 0 {
				return errors.Wrapf(errLimitExceeded, "no fair share limit available for filters without limit")
			}
			s.Filters[i].Limit = fairSharePerFilter
		}
		total += s.Filters[i].Limit
	}

	if total > defaultLimitREQ {
		return errors.Wrapf(errLimitExceeded, "total limit %d exceeds the maximum allowed %d", total, defaultLimitREQ)
	}

	return nil
}
