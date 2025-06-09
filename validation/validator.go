// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

type Validator interface {
	Validate(ctx context.Context, events ...*model.Event) error
}

type eventValidator struct {
	config *config
}

func (v *eventValidator) Validate(ctx context.Context, events ...*model.Event) error {
	if events == nil {
		return nil
	}

	for _, e := range events {
		if !e.CheckID() {
			return ErrEventInvalidID
		}
		if ok, err := e.CheckSignature(); err != nil {
			return errors.Wrap(err, "signature check failed")
		} else if !ok {
			return ErrEventInvalidSign
		}

		if err := validate(ctx, e, events...); err != nil {
			return errors.Wrap(err, "validation failed")
		}

		if v.config != nil && v.config.NIP13MinLeadingZeroBits > 0 {
			if err := e.CheckNIP13Difficulty(v.config.NIP13MinLeadingZeroBits); err != nil {
				return errors.Wrap(err, "wrong event difficulty")
			}
		}
	}

	return nil
}

func NewEventValidator(cfg *Config) Validator {
	if cfg == nil {
		return &eventValidator{}
	}

	internalConfig := &config{
		MaxWrappedEventExpiration: cfg.MaxWrappedEventExpiration,
		MaxContentSizes:           cfg.MaxContentSizes,
		NIP13MinLeadingZeroBits:   cfg.NIP13MinLeadingZeroBits,
		RelayURL:                  cfg.RelayURL,
	}

	return &eventValidator{
		config: internalConfig,
	}
}
