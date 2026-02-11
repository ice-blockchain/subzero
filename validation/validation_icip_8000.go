// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindDeviceRegistrationAuthoritative(_ context.Context, ev *eventValidator, e *model.Event) error {
	deviceType := e.GetTag("t").Value()
	if !slices.Contains([]string{model.DeviceTokenOSAndroid, model.DeviceTokenOSIOS, model.DeviceTokenOSWeb}, strings.ToLower(deviceType)) {
		return errors.Wrapf(ErrWrongEventParams, "invalid device type in t tag: %q", deviceType)
	}

	relayTag := e.GetTag("relay").Value()
	if ev.Config != nil && ev.Config.RelayURL != "" && !ev.Config.EqualRelayURL(relayTag) {
		return errors.Wrapf(ErrWrongEventParams, "relay tag value %q does not match configured relay URL %q", relayTag, ev.Config.RelayURL)
	}
	return nil
}

func validateKindDeviceRegistration(ctx context.Context, ev *eventValidator, e *model.Event) error {
	var filters model.Filters

	if err := json.Unmarshal([]byte(e.Content), &filters); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong content JSON value: %v", err)
	}

	token := e.GetTag("token").Value()
	if token != "" {
		return validateKindDeviceRegistrationAuthoritative(ctx, ev, e)
	}

	for _, tag := range e.Tags {
		switch tag.Key() {
		case "relay":
			if v := tag.Value(); v == "" || !nostr.IsValidRelayURL(v) {
				return errors.Wrapf(ErrWrongEventParams, "invalid relay URL in relay tag: %q", v)
			}
		}
	}

	return nil
}
