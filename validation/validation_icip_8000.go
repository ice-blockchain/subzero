// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindDeviceRegistration(e *model.Event) error {
	tTag := e.GetTag("t").Value()
	if tTag != model.DeviceTokenOSAndroid && tTag != model.DeviceTokenOSIOS && tTag != model.DeviceTokenOSWeb {
		return errors.Wrapf(ErrWrongEventParams, "wrong t tag value: %v", tTag)
	}
	var filters model.Filters
	if err := json.Unmarshal([]byte(e.Content), &filters); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong content JSON value: %v", err)
	}

	return nil
}
