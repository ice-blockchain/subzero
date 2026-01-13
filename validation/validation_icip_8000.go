// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (ev *eventValidator) validateKindDeviceRegistration(_ context.Context, rules *ruleSet, _ model.Events, e *model.Event) error {
	tTag := e.GetTag("t").Value()
	if tTag != model.DeviceTokenOSAndroid && tTag != model.DeviceTokenOSIOS && tTag != model.DeviceTokenOSWeb {
		return errors.Wrapf(ErrWrongEventParams, "wrong t tag value: %v", tTag)
	}

	var filters model.Filters
	if err := json.Unmarshal([]byte(e.Content), &filters); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong content JSON value: %v", err)
	}

	if rules.BroadcastMode {
		return nil
	}

	relayTag := e.GetTag("relay").Value()
	if ev.Config != nil && ev.Config.RelayURL != "" && !ev.Config.EqualRelayURL(relayTag) {
		return errors.Wrapf(ErrWrongEventParams, "relay tag value %q does not match configured relay URL %q", relayTag, ev.Config.RelayURL)
	}
	return nil
}
