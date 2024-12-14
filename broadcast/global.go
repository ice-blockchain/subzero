// SPDX-License-Identifier: ice License 1.0

package broadcast

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var (
	kindsToBroadcast = map[int]struct{}{
		nostr.KindGiftWrap: {},
	}

	globalBroadcaster struct {
		Instance *broadcaster
		Once     sync.Once
	}
	globalConfig *config
)

func MustInit(ctx context.Context) {
	globalBroadcaster.Once.Do(func() {
		globalConfig = cfg.MustGet[config]()
		bcast, err := newBroadcaster(ctx, globalConfig)
		if err != nil {
			log.Panicf("failed to initialize broadcaster: %v", err)
		}
		globalBroadcaster.Instance = bcast

		go func() {
			<-ctx.Done()
			globalBroadcaster.Instance.Close()
			globalBroadcaster.Once = sync.Once{}
		}()
	})
}

func AcceptEvents(ctx context.Context, events ...*model.Event) (err error) {
	author, authenticated := model.GetUserDataFromContext(ctx)
	if !authenticated {
		return nil
	}

	for _, event := range events {
		if _, ok := kindsToBroadcast[event.Kind]; !ok {
			continue
		}

		err = errors.Join(err, globalBroadcaster.Instance.Broadcast(ctx, author, event))
	}

	return err
}
