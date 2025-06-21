// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"fmt"

	"github.com/jellydator/ttlcache/v3"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func GetStoredEvents(ctx context.Context, filters ...model.Filter) query.EventIterator {
	return globalDVM.searchDVMEvents(ctx, filters)
}

func (d *dvm) searchDVMEvents(ctx context.Context, filters model.Filters) query.EventIterator {
	return func(yield func(*model.Event, error) bool) {
		var doStop bool
		for index, f := range filters {
			if ctx.Err() != nil && !doStop {
				yield(nil, ctx.Err())
				return
			} else if doStop {
				// If we already stopped, we don't need to continue.
				return
			}

			if f.Tags.HasValues("p") {
				for _, e := range d.findByFilterTag(filters, index, "p") {
					if !yield(e, nil) {
						return
					}
				}
				// We are done with this filter, continue to the next one.
				continue
			}

			d.responseCache.Range(func(item *ttlcache.Item[string, *xsync.Map[string, *model.Event]]) bool {
				item.Value().Range(func(key string, value *model.Event) bool {
					if model.FiltersMatch(filters, value, "", "") {
						if !yield(value, nil) {
							doStop = true
							return false // Stop iterating over xsync.Map.
						}
					}
					return ctx.Err() == nil && !doStop
				})
				return ctx.Err() == nil && !doStop
			})
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
		val, _ := d.responseCache.GetOrSet(key, xsync.NewMap[string, *model.Event](),
			ttlcache.WithTTL[string, *xsync.Map[string, *model.Event]](model.DVMJobResultExpiration))
		val.Value().LoadAndStore(event.ID, event)
		d.responseCache.Set(key, val.Value(), model.DVMJobResultExpiration)
	}

	return nil
}
