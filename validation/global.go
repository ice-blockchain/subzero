// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"sync"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var (
	global struct {
		Validator *eventValidator
		sync.Once
	}
)

func MustInit(ctx context.Context, opts ...Option) {
	global.Do(func() {
		global.Validator = newEventValidator(ctx, cfg.MustGet[Config](), opts...)
	})
}

func Validate(ctx context.Context, events model.Events, rules ...Rule) error {
	return global.Validator.Validate(ctx, events, rules...)
}

func GetCommunityDefinition(ctx context.Context, hTag string) (*model.Event, error) {
	return global.Validator.getCommunityDefinition(ctx, hTag)
}

func IsUserBanned(ctx context.Context, pubkey, communityID string) error {
	return global.Validator.IsUserBanned(ctx, pubkey, communityID)
}

func IsUserPartOfCommunity(ctx context.Context, communityDefinitionEvent *model.Event, masterPubkey string) error {
	return global.Validator.IsUserPartOfCommunity(ctx, communityDefinitionEvent, masterPubkey)
}
