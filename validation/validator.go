// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	identitypubkeys "github.com/ice-blockchain/subzero/validation/internal/ion-identity-pubkeys"
)

type (
	Option func(*eventValidator)
	Rule   func(*ruleSet)

	Validator interface {
		Validate(ctx context.Context, batch model.Events, rules ...Rule) error
	}

	eventValidator struct {
		Config                *Config
		QueryFunc             func(context.Context, ...model.Filter) query.EventIterator
		IONIdentityPublicKeys func() []string
	}
	ruleSet struct {
		SkipKindProfileProofEventsVerify        bool
		SkipKindAttestationProofDevicesVerify   bool
		SkipRootContentNFTCollectionsValidation bool
		SkipRootContentReplyValidation          bool
		BroadcastMode                           bool
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

	if true { // TODO: remove once FE implemented
		ruleSet.SkipKindAttestationProofDevicesVerify = true
	}

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
func WithIONIdentityPublicKeys(f func() []string) Option {
	return func(v *eventValidator) {
		v.IONIdentityPublicKeys = f
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

func RuleWithSkipDeviceIdentificationProofEventsVerify() Rule {
	return func(v *ruleSet) {
		v.SkipKindAttestationProofDevicesVerify = true
	}
}

func RuleWithSkipRootContentNFTCollectionsValidation() Rule {
	return func(v *ruleSet) {
		v.SkipRootContentNFTCollectionsValidation = true
	}
}

func RuleWithSkipRootContentReplyValidation() Rule {
	return func(v *ruleSet) {
		v.SkipRootContentReplyValidation = true
	}
}

func New(ctx context.Context, opts ...Option) Validator {
	return newEventValidator(ctx, cfg.MustGet[Config](), opts...)
}

func newEventValidator(ctx context.Context, cfg *Config, opts ...Option) *eventValidator {
	validator := eventValidator{
		Config:    cfg,
		QueryFunc: query.GetStoredEvents,
	}

	for _, opt := range opts {
		opt(&validator)
	}
	if validator.IONIdentityPublicKeys == nil {
		validator.IONIdentityPublicKeys = identitypubkeys.
			MustNewIONIdentityPublicKeys(ctx, cfg.IONIdentityBaseURL).
			PublicKeys
	}
	return &validator
}
