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
		for _, f := range filters {
			if f.Tags.HasValues("p") {
				for _, e := range eventMatcher(ctx, subscription, d.findByFilterTag(f, "p")...) {
					if !yield(e, nil) {
						return
					}
				}
			} else {
				events := []*model.Event{}
				d.responseCache.Range(func(item *ttlcache.Item[string, *xsync.MapOf[string, *model.Event]]) bool {
					if ctx.Err() != nil {
						return yield(nil, ctx.Err())
					}
					item.Value().Range(func(key string, value *model.Event) bool {
						events = append(events, value)
						return true
					})
					return true
				})
				for _, event := range eventMatcher(ctx, subscription, events...) {
					if !yield(event, nil) {
						return
					}
				}
			}
		}
	}
}

func (d *dvm) findByFilterTag(
	f model.Filter,
	tagName string,
) []*model.Event {
	resultEvents := []*model.Event{}
	for _, pk := range f.Tags.All(tagName) {
		matchingEvents := d.responseCache.Get(tagCacheKey("p", pk))
		if matchingEvents != nil {
			matchingEvents.Value().Range(func(key string, value *model.Event) bool {
				if f.Matches(&value.Event) {
					resultEvents = append(resultEvents, value)
				}
				return true
			})
		}
	}
	return resultEvents
}

func tagCacheKey(tag, value string) string {
	return fmt.Sprintf("%v%v", tag, value)
}

func (d *dvm) acceptDVMResponseEvent(event *model.Event) error {
	if pTag := event.Tags.GetFirst([]string{"p"}); pTag != nil {
		key := tagCacheKey("p", pTag.Value())
		val, _ := d.responseCache.GetOrSet(key, xsync.NewMapOf[string, *model.Event](),
			ttlcache.WithTTL[string, *xsync.MapOf[string, *model.Event]](ttlcache.DefaultTTL))
		val.Value().LoadAndStore(event.ID, event)
		d.responseCache.Set(key, val.Value(), ttlcache.DefaultTTL)
	}

	return nil
}
