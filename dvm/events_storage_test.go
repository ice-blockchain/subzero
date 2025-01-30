// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"github.com/jellydator/ttlcache/v3"
	"github.com/puzpuzpuz/xsync/v3"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

func TestDVM_ConcurrentEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	globalConfig = cfg.MustGet[config]()
	d := dvm{
		dvmResponses: ttlcache.New[string, *xsync.MapOf[string, *model.Event]](),
	}
	relayKey, err := model.GetPublicKey(globalConfig.PrivateKey)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for _ = range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			userPrivKey, userKey := model.GenerateKeyPair()
			var idx atomic.Int64
			for ctx.Err() == nil {
				var inWG sync.WaitGroup
				for _ = range 5 {
					inWG.Add(1)
					go func() {
						defer inWG.Done()
						incomingEventID := uuid.NewString()
						e := &model.Event{
							Event: nostr.Event{
								CreatedAt: nostr.Timestamp(time.Now().Unix()),
								Content:   strconv.FormatInt(idx.Load(), 10),
								Kind:      model.KindDVMCountResponse,
								Tags: model.Tags{
									{"request", "{}"},
									{"e", incomingEventID, globalConfig.RelayURL},
									{"expiration", strconv.FormatInt(time.Now().Add(model.DVMJobResultExpiration).Unix(), 10)},
									{"p", userKey},
									{model.CustomIONTagOnBehalfOf, relayKey},
								},
							}}
						require.NoError(t, e.Sign(userPrivKey))
						require.NoError(t, d.acceptDVMResponseEvent(e))
						idx.Add(1)
					}()
				}
				inWG.Wait()
			}
			queryCtx, cancelQuery := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancelQuery()
			events, err := d.searchDVMEvents(queryCtx, model.Filters{{Kinds: []int{model.KindDVMCountResponse}, Tags: model.TagMap{}.
				Append("p", &userKey),
			}})
			require.NoError(t, err)
			require.Equal(t, int64(len(events)), idx.Load())
		}()
	}
	wg.Wait()
}
