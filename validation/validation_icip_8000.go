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
	token := e.GetTag("token").Value()
	if token == "" {
		return errors.Wrapf(ErrWrongEventParams, "missing token tag")
	}

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

	dtagValues := strings.SplitN(e.Tags.GetD(), "_", 2)
	if len(dtagValues) > 1 && dtagValues[1] != "" { // Combination of master public key and device id is used: `master` + '_' + `device-id`.
		dtagMasterKey := dtagValues[0]

		authoritativeForDtagMasterKey, _, err := ev.IsRelayAuthoritativeForUser(ctx, ev.Config.RelayURL, dtagMasterKey, "")
		if err != nil {
			return errors.Wrapf(err, "failed to check relay authoritativeness for user %v", dtagMasterKey)
		}

		if authoritativeForDtagMasterKey && dtagMasterKey != e.GetMasterPublicKey() {
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
		return errors.Wrapf(ErrWrongEventParams, "relay is not authoritative for the master public key in d tag: %v or d tag value is invalid", dtagMasterKey)
	}

	data := model.GetUserDataFromContext(ctx)
	if data.Authoritative && data.MasterPublicKey == e.GetMasterPublicKey() {
		return validateKindDeviceRegistrationAuthoritative(ctx, ev, e)
	}

	return errors.Wrap(ErrWrongEventParams, "relay is not authoritative for the user and/or master public key is different from the one in the event")
}
