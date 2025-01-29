// SPDX-License-Identifier: ice License 1.0

package query

import (
	"fmt"
	"sync"

	"github.com/jellydator/ttlcache/v3"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

type (
	group struct {
		m map[string][]*databaseEvent
	}
)

func (g *group) GetDVMEvent(key string) []*databaseEvent {
	return g.m[key]
}

func (db *dbClient) searchDVMEvents(filters model.Filters) func() []*databaseEvent {
	return func() []*databaseEvent {
		filteredDVMEvents := []*databaseEvent{}
		for _, f := range filters {
			for _, kind := range f.Kinds {
				if kind >= 6000 && kind <= nostr.KindJobFeedback {
					matches := db.filterDVMResponseByFilterTag(db, f, kind, "p")
					if len(matches) > 0 && !f.Tags.HasValues("e") {
						filteredDVMEvents = append(filteredDVMEvents, matches...)
						continue
					}
					var searchE interface {
						GetDVMEvent(key string) []*databaseEvent
					} = db
					if len(matches) > 0 {
						searchE = groupByTag(matches, "e")
					}

					filteredDVMEvents = append(filteredDVMEvents, db.filterDVMResponseByFilterTag(searchE, f, kind, "e")...)
				}
			}
		}
		return filteredDVMEvents
	}
}

func (db *dbClient) filterDVMResponseByFilterTag(
	getter interface {
		GetDVMEvent(key string) []*databaseEvent
	},
	f model.Filter,
	kind int,
	tagName string,
) []*databaseEvent {
	resultEvents := []*databaseEvent{}
	if tag, hasTag := f.Tags[tagName]; hasTag {
		for _, t := range tag {
			for _, entry := range t {
				if entry == nil {
					continue
				}
				matchingEvent := getter.GetDVMEvent(fmt.Sprintf("%v%v%v", kind, tagName, *entry))
				if len(matchingEvent) > 0 {
					resultEvents = append(resultEvents, matchingEvent...)
				}
			}
		}
	}
	return resultEvents
}

func (db *dbClient) acceptDVMResponseEvent(event *model.Event) error {
	dbEvent, err := toDatabaseEvent(event)
	if err != nil {
		return err
	}
	if eTag := event.Tags.GetFirst([]string{"e"}); eTag != nil {
		db.dvmResponses.Set(fmt.Sprintf("%ve%v", event.Kind, eTag.Value()), []*databaseEvent{dbEvent}, ttlcache.DefaultTTL)
	}
	if pTag := event.Tags.GetFirst([]string{"p"}); pTag != nil {
		key := fmt.Sprintf("%vp%v", event.Kind, pTag.Value())
		profileDvmResponses := []*databaseEvent{dbEvent}
		var mx sync.Mutex
		mx.Lock()
		val := db.dvmResponses.Get(key)
		if val != nil {
			profileDvmResponses = val.Value()
			profileDvmResponses = append(profileDvmResponses, dbEvent)
		}
		db.dvmResponses.Set(key, profileDvmResponses, ttlcache.DefaultTTL)
		mx.Unlock()
	}
	if bTag := event.Tags.GetFirst([]string{"b"}); bTag != nil {
		key := fmt.Sprintf("%vb%v", event.Kind, bTag.Value())
		profileDvmResponses := []*databaseEvent{dbEvent}
		var mx sync.Mutex
		mx.Lock()
		val := db.dvmResponses.Get(key)
		if val != nil {
			profileDvmResponses = val.Value()
			profileDvmResponses = append(profileDvmResponses, dbEvent)
		}
		db.dvmResponses.Set(key, profileDvmResponses, ttlcache.DefaultTTL)
		mx.Unlock()
	}
	return nil
}

func groupByTag(prevMatches []*databaseEvent, tagName string) interface {
	GetDVMEvent(key string) []*databaseEvent
} {
	g := map[string][]*databaseEvent{}
	for _, m := range prevMatches {
		if tag := m.GetTag(tagName); tag != nil {
			key := fmt.Sprintf("%v%v%v", m.Kind, tagName, tag.Value())
			g[key] = append(g[key], m)
		}
	}
	return &group{m: g}
}

func (db *dbClient) GetDVMEvent(key string) []*databaseEvent {
	v := db.dvmResponses.Get(key, ttlcache.WithDisableTouchOnHit[string, []*databaseEvent]())
	if v != nil {
		return v.Value()
	}
	return nil
}
