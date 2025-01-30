// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"fmt"

	"github.com/jellydator/ttlcache/v3"
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/model"
)

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) ([]*model.Event, error) {
	var filters model.Filters
	if subscription != nil {
		filters = subscription.Filters
	}
	return globalDVM.searchDVMEvents(ctx, filters)
}

func (d *dvm) searchDVMEvents(ctx context.Context, filters model.Filters) ([]*model.Event, error) {
	events := []*model.Event{}
	for _, f := range filters {
		if f.Tags.HasValues("p") {
			events = append(events, d.findByFilterTag(f, "p")...)
		} else {
			d.dvmResponses.Range(func(item *ttlcache.Item[string, *xsync.MapOf[string, *model.Event]]) bool {
				if ctx.Err() != nil {
					return false
				}
				item.Value().Range(func(key string, value *model.Event) bool {
					events = append(events, value)
					return true
				})
				return true
			})
		}
	}
	return events, ctx.Err()
}

func (d *dvm) findByFilterTag(
	f model.Filter,
	tagName string,
) []*model.Event {
	resultEvents := []*model.Event{}
	if tag, hasTag := f.Tags[tagName]; hasTag {
		for _, t := range tag {
			for _, entry := range t {
				if entry == nil {
					continue
				}
				matchingEvents := d.dvmResponses.Get(fmt.Sprintf("%v%v", tagName, *entry))
				if matchingEvents != nil {
					matchingEvents.Value().Range(func(key string, value *model.Event) bool {
						resultEvents = append(resultEvents, value)
						return true
					})
				}
			}
		}
	}
	return resultEvents
}

func (d *dvm) acceptDVMResponseEvent(event *model.Event) error {
	if pTag := event.Tags.GetFirst([]string{"p"}); pTag != nil {
		key := fmt.Sprintf("p%v", pTag.Value())
		val, _ := d.dvmResponses.GetOrSet(key, xsync.NewMapOf[string, *model.Event](),
			ttlcache.WithTTL[string, *xsync.MapOf[string, *model.Event]](ttlcache.DefaultTTL))
		val.Value().LoadAndStore(event.ID, event)
		d.dvmResponses.Set(key, val.Value(), ttlcache.DefaultTTL)
	}

	return nil
}
