// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

const (
	DeviceTokenOSAndroid = "android"
	DeviceTokenOSIOS     = "ios"
	DeviceTokenOSWeb     = "web"
)

func GetDeviceRegistrationValidator() kindValidator {
	return newKindValidatorBuilder().
		ContentNotEmpty().
		Required("d", "t", "relay", "token").
		Build()
}

func validateKindDeviceRegistration(e *model.Event) error {
	tTag := e.GetTag("t").Value()
	if tTag != DeviceTokenOSAndroid && tTag != DeviceTokenOSIOS && tTag != DeviceTokenOSWeb {
		return errors.Wrapf(ErrWrongEventParams, "wrong t tag value: %v", tTag)
	}
	relayTag := e.GetTag("relay").Value()
	if globalConfig != nil && relayTag != globalConfig.RelayURL {
		return errors.Wrapf(ErrWrongEventParams, "relay tag value %q does not match configured relay URL %q", relayTag, globalConfig.RelayURL)
	}

	var filters model.Filters
	if err := json.Unmarshal([]byte(e.Content), &filters); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong content JSON value: %v", err)
	}

	return nil
}
