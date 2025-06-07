// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/ice-blockchain/subzero/model"
)

type Validator interface {
	Validate(ctx context.Context, e *model.Event, events ...*model.Event) error
}

type eventValidator struct {
	config *config
}

func (v *eventValidator) Validate(ctx context.Context, e *model.Event, events ...*model.Event) error {
	if e == nil {
		return nil
	}

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

	return nil
}

// newEventValidator creates a new eventValidator instance
func newEventValidator(cfg *config) *eventValidator {
	return &eventValidator{
		config: cfg,
	}
}

// NewEventValidator creates a new eventValidator instance with public Config
func NewEventValidator(cfg *Config) Validator {
	if cfg == nil {
		return newEventValidator(nil)
	}

	internalConfig := &config{
		MaxWrappedEventExpiration: cfg.MaxWrappedEventExpiration,
		MaxContentSizes:           cfg.MaxContentSizes,
		NIP13MinLeadingZeroBits:   cfg.NIP13MinLeadingZeroBits,
		RelayURL:                  cfg.RelayURL,
	}

	return newEventValidator(internalConfig)
}
