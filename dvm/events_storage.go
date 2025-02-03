// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"fmt"

	"github.com/jellydator/ttlcache/v3"
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) query.EventIterator {
	return globalDVM.searchDVMEvents(ctx, subscription)
}

func (d *dvm) searchDVMEvents(ctx context.Context, subscription *model.Subscription) query.EventIterator {
	var filters model.Filters
	if subscription != nil {
		filters = subscription.Filters
	}
	return func(yield func(*model.Event, error) bool) {
		for index, f := range filters {
			if f.Tags.HasValues("p") {
				for _, e := range d.findByFilterTag(filters, index, "p") {
					if !yield(e, nil) {
						return
					}
				}
			} else {
				d.responseCache.Range(func(item *ttlcache.Item[string, *xsync.MapOf[string, *model.Event]]) bool {
					if ctx.Err() != nil {
						return yield(nil, ctx.Err())
					}
					item.Value().Range(func(key string, value *model.Event) bool {
						if filters.Match(&value.Event) { // <-- filters
							if !yield(value, nil) {
								return false
							}
						}
						return true
					})
					return true
				})
			}
		}
	}
}

func (d *dvm) findByFilterTag(filters model.Filters, current int, tagName string) (results []*model.Event) {
	for _, pk := range filters[current].Tags.All(tagName) {
		eventsByAuthor := d.responseCache.Get(tagCacheKey("p", pk))
		if eventsByAuthor == nil {
			continue
		}

		eventsByAuthor.Value().Range(func(_ string, value *model.Event) bool {
			if filters.Match(&value.Event) {
				results = append(results, value)
			}
			return true
		})

	}
	return results
}

func tagCacheKey(tag, value string) string {
	return fmt.Sprintf("%v%v", tag, value)
}

func (d *dvm) acceptDVMResponseEvent(event *model.Event) error {
	if pTag := event.Tags.GetFirst([]string{"p"}); pTag != nil {
		key := tagCacheKey("p", pTag.Value())
		val, _ := d.responseCache.GetOrSet(key, xsync.NewMapOf[string, *model.Event](),
			ttlcache.WithTTL[string, *xsync.MapOf[string, *model.Event]](model.DVMJobResultExpiration))
		val.Value().LoadAndStore(event.ID, event)
		d.responseCache.Set(key, val.Value(), model.DVMJobResultExpiration)
	}

	return nil
}
