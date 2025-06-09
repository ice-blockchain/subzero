// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func GetFundReceiveValidator() kindValidator {
	return newKindValidatorBuilderEmpty().
		ContentNotEmpty().
		Optional("asset_address").
		OneOf("p", "l").
		Required(model.CustomIONTagOnBehalfOf, "network", "asset_class").
		RequiredWith("l", "L").
		Build()
}

func GetFundSendNotifyValidator() kindValidator {
	return newKindValidatorBuilderEmpty().
		ContentNotEmpty().
		Optional("request", "asset_address").
		OneOf("p", "l").
		Required(model.CustomIONTagOnBehalfOf, "network", "asset_class").
		RequiredWith("l", "L").
		Build()
}

func validateKindFundReceive(e *model.Event) error {
	if e.GetTag("p").Value() != "" {
		return nil
	}

	var address struct {
		From string `json:"from"`
	}
	if label := e.GetTag("L").Value(); label != "wallet.address" {
		return errors.Errorf("fund receive: invalid L tag value: %q", label)
	}

	if err := json.Unmarshal([]byte(e.Content), &address); err != nil {
		return errors.Errorf("fund receive: invalid content: %v", err)
	} else if address.From == "" {
		return errors.Errorf("fund receive: empty address in content")
	} else if addr := e.GetTag("l"); address.From != addr.Value() {
		return errors.Errorf("fund receive: address in content %q does not match tag l %q", address.From, addr.Value())
	} else if len(addr) < 3 || addr[2] != "wallet.address" {
		return errors.Errorf("fund send notify: invalid alias in tag l %q", addr.Value())
	}

	return nil
}

func validateKindFundSendNotify(e *model.Event) error {
	if e.GetTag("p").Value() != "" {
		return nil
	}

	var address struct {
		To string `json:"to"`
	}
	if label := e.GetTag("L").Value(); label != "wallet.address" {
		return errors.Errorf("fund send notify: invalid L tag value: %q", label)
	}

	if err := json.Unmarshal([]byte(e.Content), &address); err != nil {
		return errors.Errorf("fund send notify: invalid content: %v", err)
	} else if address.To == "" {
		return errors.Errorf("fund send notify: empty address in content")
	} else if addr := e.GetTag("l"); address.To != addr.Value() {
		return errors.Errorf("fund send notify: address in content %q does not match tag l %q", address.To, addr.Value())
	} else if len(addr) < 3 || addr[2] != "wallet.address" {
		return errors.Errorf("fund send notify: invalid alias in tag l %q", addr.Value())
	}

	return nil
}
