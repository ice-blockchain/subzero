// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/model"
)

func TestDVM_ConcurrentEvents(t *testing.T) {
	t.Parallel()

	const eventsPerThread = 5
	const threadsCount = 1000
	ctx, cancel := appcontext.NewAppContext(t.Context())
	defer cancel()
	d := mustNewDVM(ctx)
	var wg sync.WaitGroup
	for range threadsCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			userPrivKey, userKey := model.GenerateKeyPair()
			var idx atomic.Int64
			var inWG sync.WaitGroup
			for range eventsPerThread {
				inWG.Add(1)
				go func() {
					defer inWG.Done()
					e := &model.Event{
						Event: nostr.Event{
							CreatedAt: nostr.Now(),
							Content:   strconv.FormatInt(idx.Load(), 10),
							Kind:      model.KindDVMCountResponse,
							Tags: model.Tags{
								{"request", "{}"},
								{"expiration", nostr.Now().Add(model.DVMJobResultExpiration).String()},
								{"p", userKey},
								{model.CustomIONTagOnBehalfOf, d.PublicKey},
							},
						}}
					require.NoError(t, e.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
					require.NoError(t, d.acceptDVMResponseEvent(e))
					idx.Add(1)
				}()
				inWG.Wait()
			}
			queryCtx, cancelQuery := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancelQuery()
			eventsIt := d.searchDVMEvents(queryCtx, model.Filters{{
				Kinds: []int{model.KindDVMCountResponse},
				Tags:  model.TagMap{}.Append("p", &userKey),
			}})
			var events []*model.Event
			for ev, err := range eventsIt {
				require.NoError(t, err)
				events = append(events, ev)
			}
			require.Equal(t, int64(len(events)), idx.Load())
		}()
	}
	t.Logf("Waiting for %d threads to finish...", threadsCount)
	wg.Wait()
}
