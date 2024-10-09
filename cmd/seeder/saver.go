// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/ice-blockchain/subzero/model"
)

func (f *fetcher) AcceptEvent(ctx context.Context, event *model.Event) error {
	idx := atomic.AddUint64(&f.outputLBIdx, 1) % uint64(len(f.outputRelays))
	if err := f.outputRelays[idx].Publish(ctx, event.Event); err != nil {
		log.Println("WARN on accept event: ", err)
	}
	return nil
}
