// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindDeviceRegistration(_ context.Context, ev *eventValidator, e *model.Event) error {
	var filters model.Filters

	if err := json.Unmarshal([]byte(e.Content), &filters); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong content JSON value: %v", err)
	}

	for _, tag := range e.Tags {
		switch tag.Key() {
		case "t":
			if !slices.Contains([]string{model.DeviceTokenOSAndroid, model.DeviceTokenOSIOS, model.DeviceTokenOSWeb}, strings.ToLower(tag.Value())) {
				return errors.Wrapf(ErrWrongEventParams, "invalid device type in t tag: %q", tag.Value())
			}
		case "relay":
			_, err := url.Parse(tag.Value())
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "invalid relay URL in relay tag: %q: %v", tag.Value(), err)
			}
		}
	}

	return nil
}
