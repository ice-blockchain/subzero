// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/ice-blockchain/subzero/model"
)

func validateKindGiftWrapEvent(e *model.Event) error {
	subkindNoExpiration := map[int]struct{}{
		model.CustomIONKindUserBlock:      {},
		model.CustomIONKindFundReceive:    {},
		model.CustomIONKindFundSendNotify: {},
	}
	kTag := e.GetTag("k").Value()
	subkind, err := strconv.ParseInt(kTag, 10, 64)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "gift wrap: invalid k tag value: %q: %v", kTag, err)
	}
	expiresAt := e.GetTag("expiration").Value()
	if expiresAt != "" {
		// Always check expiresAt if it's provided.
		ts, err := strconv.ParseInt(expiresAt, 10, 64)
		if err != nil {
			return errors.Wrapf(ErrWrongEventParams, "gift wrap: invalid expiration value: %q: %v", expiresAt, err)
		}
		if globalConfig != nil && globalConfig.MaxWrappedEventExpiration > 0 && time.Unix(ts, 0).After(time.Now().Add(globalConfig.MaxWrappedEventExpiration)) {
			return errors.Wrapf(ErrWrongEventParams, "gift wrap: expiration is too far in the future, max is %s", globalConfig.MaxWrappedEventExpiration)
		}
	} else if _, ok := subkindNoExpiration[int(subkind)]; !ok {
		// If expiresAt is empty and subkind is not in the exception list, return an error.
		return errors.Wrapf(ErrWrongEventParams, "gift wrap: expiration is required for subkind %d", subkind)
	}
	return nil
}
