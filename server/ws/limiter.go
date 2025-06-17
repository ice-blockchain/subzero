// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"github.com/ice-blockchain/subzero/model"
)

const (
	defaultLimitREQ = 100
)

func applySubscriptionLimit(s *model.Subscription) {
	if len(s.Filters) == 0 {
		s.Filters = []model.Filter{{Limit: defaultLimitREQ}}
	}

	limitPerFilter := defaultLimitREQ / len(s.Filters)
	for i := range s.Filters {
		if s.Filters[i].Limit > limitPerFilter || s.Filters[i].Limit <= 0 {
			s.Filters[i].Limit = limitPerFilter
		}
	}
}
