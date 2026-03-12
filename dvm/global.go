// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"sync"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

var (
	globalDVM struct {
		D             *dvm
		Once          sync.Once
		EventListener func(context.Context, ...*model.Event) error
	}
)

func MustInit(ctx context.Context, opts ...Option) {
	globalDVM.Once.Do(func() {
		globalDVM.D = mustNewDVM(ctx, opts...)
	})
}

func PublicKey() string {
	return globalDVM.D.PublicKey
}

// AcceptEvents accepts events to be processed by the DVM in the background.
// All results from processing the events will be sent to the registered event listener, if any.
func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	_, err := globalDVM.D.AcceptEvents(ctx, false, events...)
	return err
}

// AcceptJob accepts a single event as a job to be processed by the DVM and returns a channel to receive the result.
func AcceptJob(ctx context.Context, event *model.Event) (result <-chan *model.Event, err error) {
	return globalDVM.D.AcceptEvents(ctx, true, event)
}

func GetStoredEvents(ctx context.Context, filters ...model.Filter) query.EventIterator {
	return globalDVM.D.searchDVMEvents(ctx, filters)
}

func RegisterEventListener(listener func(context.Context, ...*model.Event) error) {
	globalDVM.EventListener = listener
}
