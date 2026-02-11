// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindGiftWrapEvent(_ context.Context, v *eventValidator, e *model.Event, _ *ruleSet) error {
	subkindNoExpiration := map[int]struct{}{
		model.CustomIONKindUserBlock:           {},
		model.CustomIONKindFundReceive:         {},
		model.CustomIONKindFundSendNotify:      {},
		model.CustomIONKindArchiveConversation: {},
		model.CustomIONKindMute:                {},
	}
	kTag := e.GetTag("k").Value()
	subkind, err := strconv.ParseInt(kTag, 10, 64)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "gift wrap: invalid k tag value: %q: %v", kTag, err)
	}
	expiresAt := e.GetTag("expiration").Value()
	if expiresAt != "" {
		// Always check expiresAt if it's provided.
		ts, err := nostr.ParseTimestamp(expiresAt)
		if err != nil {
			return errors.Wrapf(ErrWrongEventParams, "gift wrap: invalid expiration value: %q: %v", expiresAt, err)
		}
		if v.Config != nil && v.Config.MaxWrappedEventExpiration > 0 && ts.Time().After(time.Now().Add(v.Config.MaxWrappedEventExpiration)) {
			return errors.Wrapf(ErrWrongEventParams, "gift wrap: expiration is too far in the future, max is %s", v.Config.MaxWrappedEventExpiration)
		}
	} else if _, ok := subkindNoExpiration[int(subkind)]; !ok {
		// If expiresAt is empty and subkind is not in the exception list, return an error.
		return errors.Wrapf(ErrWrongEventParams, "gift wrap: expiration is required for subkind %d", subkind)
	}
	return nil
}
