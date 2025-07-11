// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"sync"

	"github.com/ice-blockchain/subzero/model"
)

var (
	globalDVM struct {
		*dvm
		sync.Once
	}
)

func MustInit(ctx context.Context, opts ...Option) {
	globalDVM.Do(func() {
		globalDVM.dvm = mustNewDVM(ctx, opts...)
	})
}

func PublicKey() string {
	return globalDVM.PublicKey
}

func AcceptJob(ctx context.Context, event *model.Event) (<-chan *model.Event, error) {
	return globalDVM.AcceptJob(ctx, event)
}
