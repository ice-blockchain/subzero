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
	Rule   func(*ruleSet)

	eventValidator struct {
		Config    *Config
		QueryFunc func(context.Context, ...model.Filter) query.EventIterator
	}
	ruleSet struct {
		SkipKindProfileProofEventsVerify bool
		BroadcastMode                    bool
	}
)

func (r *ruleSet) Configure(rules ...Rule) *ruleSet {
	for _, rule := range rules {
		rule(r)
	}
	return r
}

func (v *eventValidator) Validate(ctx context.Context, batch model.Events, rules ...Rule) error {
	var ruleSet ruleSet

	ruleSet.Configure(rules...)

	for _, e := range batch {
		if err := v.validate(ctx, &ruleSet, batch, e); err != nil {
			return errors.Wrap(err, "validation failed")
		}
	}

	return nil
}

func WithQueryFunc(f func(context.Context, ...model.Filter) query.EventIterator) Option {
	return func(v *eventValidator) {
		v.QueryFunc = f
	}
}

func RuleWithBroadcastMode() Rule {
	return func(v *ruleSet) {
		v.BroadcastMode = true
	}
}

func RuleWithSkipProfileMetadataProofEventsVerify() Rule {
	return func(v *ruleSet) {
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
