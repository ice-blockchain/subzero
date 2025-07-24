// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Option func(*eventValidator)

	eventValidator struct {
		Config                           *Config
		QueryFunc                        func(context.Context, ...model.Filter) query.EventIterator
		SkipKindProfileProofEventsVerify bool
	}
)

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

		if err := v.validate(ctx, e, events...); err != nil {
			return errors.Wrap(err, "validation failed")
		}

		if v.Config != nil && v.Config.NIP13MinLeadingZeroBits > 0 {
			if err := e.CheckNIP13Difficulty(v.Config.NIP13MinLeadingZeroBits); err != nil {
				return errors.Wrap(err, "wrong event difficulty")
			}
		}
	}

	return nil
}

func WithQueryFunc(f func(context.Context, ...model.Filter) query.EventIterator) Option {
	return func(v *eventValidator) {
		v.QueryFunc = f
	}
}

func WithSkipProfileMetadataProofEventsVerify() Option {
	return func(v *eventValidator) {
		v.SkipKindProfileProofEventsVerify = true
	}
}

func newEventValidator(cfg *Config, opts ...Option) *eventValidator {
	validator := eventValidator{
		Config:    cfg,
		QueryFunc: query.GetStoredEvents,
	}

	for _, opt := range opts {
		opt(&validator)
	}

	return &validator
}
