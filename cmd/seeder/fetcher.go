// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alitto/pond"
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

type (
	Fetcher interface {
		StartFetching(ctx context.Context)
	}
	fetcher struct {
		relays       [][]*nostr.Relay
		outputRelays []*nostr.Relay
		outputLBIdx  uint64
		profiles     int
		perUser      int
		threads      int
	}
)

const concurrentReqs = 10

func NewFetcher(ctx context.Context, relayUrls []string, threads, profiles, perUser int, output string) Fetcher {
	f := &fetcher{
		relays:       make([][]*nostr.Relay, 0, len(relayUrls)),
		outputRelays: make([]*nostr.Relay, 0, threads),
		profiles:     profiles,
		perUser:      perUser,
		threads:      threads,
	}
	for _ = range threads {
		relay, err := connectToRelay(ctx, output)
		if err != nil {
			panic(err)
		}
		f.outputRelays = append(f.outputRelays, relay)
	}
	log.Println(fmt.Sprintf("Established %v conns to %v", threads, output))

	for _, relayUrl := range relayUrls {
		rels := make([]*nostr.Relay, 0, threads/concurrentReqs)
		if threads > concurrentReqs {
			for i := 0; i < threads/concurrentReqs; i++ {
				relay, err := connectToRelay(ctx, relayUrl)
				if err != nil {
					panic(err)
				}
				rels = append(rels, relay)
			}
		} else {
			relay, err := connectToRelay(ctx, relayUrl)
			if err != nil {
				panic(err)
			}
			rels = append(rels, relay)
		}
		log.Printf("Established %v conns to %v", len(rels), relayUrl)
		f.relays = append(f.relays, rels)
	}
	return f
}

func (f *fetcher) StartFetching(ctx context.Context) {
	var wg sync.WaitGroup
	for _, r := range f.relays {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.mustFetchUsers(ctx, f.profiles/len(f.relays), r)
		}()
	}
	wg.Wait()
}

func connectToRelay(ctx context.Context, url string) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url)
	err := relay.Connect(ctx)
	return relay, err
}

func (f *fetcher) mustFetchUsers(ctx context.Context, profiles int, relayConns []*nostr.Relay) {
	profilesProcessed := uint64(0)
	lastTS := int64(nostr.Now())
	eventAuthors := map[string]bool{}
	pool := pond.New(f.threads, profiles, pond.MinWorkers(f.threads))
	for len(eventAuthors) < profiles {
		ts := nostr.Timestamp(atomic.LoadInt64(&lastTS))
		latestEvents, err := queryEvents(ctx, relayConns[0], nostr.Filter{
			Kinds: []int{nostr.KindArticle, nostr.KindTextNote},
			Limit: threads,
			Until: &ts,
		})
		if err != nil {
			log.Fatal(err)
		}
		var lbIdx uint64
		for ev := range latestEvents {
			if int64(ev.CreatedAt) < atomic.LoadInt64(&lastTS) {
				atomic.StoreInt64(&lastTS, int64(ev.CreatedAt))
			}
			if _, processed := eventAuthors[ev.PubKey]; processed {
				continue
			}
			eventAuthors[ev.PubKey] = true
			pool.Submit(func() {
				idx := atomic.AddUint64(&lbIdx, 1) % uint64(len(relayConns))
				eventAuthor, err := queryEvents(ctx, relayConns[idx], nostr.Filter{
					Kinds:   []int{nostr.KindProfileMetadata},
					Authors: []string{ev.PubKey},
					Limit:   1,
				})
				if err != nil {
					log.Fatal(err)
				}
				if profile, ok := <-eventAuthor; ok {
					e := &model.Event{Event: *profile}
					if e.Validate() == nil {
						if aErr := f.AcceptEvent(ctx, e); aErr != nil {
							log.Fatal(aErr)
						}
						f.fetchUserContent(ctx, ev.PubKey, relayConns[idx])
						atomic.AddUint64(&profilesProcessed, 1)
					}
				}
			})
		}
	}
	pool.StopAndWait()
}
func (f *fetcher) fetchUserContent(ctx context.Context, userKey string, relay *nostr.Relay) {
	events, err := relay.QueryEvents(ctx, nostr.Filter{
		Kinds:   []int{nostr.KindTextNote, nostr.KindArticle, nostr.KindReaction, nostr.KindRepost},
		Authors: []string{userKey},
		Limit:   f.perUser,
	})
	if err != nil {
		log.Fatal(err)
	}
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	eventsCount := f.fetchLinkedAndProcessEvents(ctx, evList, relay)
	fmt.Println(relay.URL, userKey, eventsCount)
}

func (f *fetcher) fetchLinkedAndProcessEvents(ctx context.Context, events []*nostr.Event, relay *nostr.Relay) int {
	repliesAndReactionsGroupedByRelay := map[string][]string{}
	repostedEventsGroupedByRelay := map[string][]string{}
	eventsAndReplies := map[string]*nostr.Event{}
	for _, ev := range events {
		if ev.Kind == nostr.KindTextNote || ev.Kind == nostr.KindReaction {
			extractEventRef(ev, "e", relay.URL, repliesAndReactionsGroupedByRelay)
			extractEventRef(ev, "q", relay.URL, repostedEventsGroupedByRelay)
		}
		if ev.Kind == nostr.KindRepost {
			if ev.Content != "" {
				var respostedEvent nostr.Event
				if err := json.Unmarshal([]byte(ev.Content), &respostedEvent); err != nil {
					log.Fatal(err)
				}
				eventsAndReplies[respostedEvent.ID] = &respostedEvent
			} else {
				extractEventRef(ev, "e", relay.URL, repostedEventsGroupedByRelay)
			}
		}
		eventsAndReplies[ev.ID] = ev
	}
	evCount := 0
	if len(repliesAndReactionsGroupedByRelay) > 0 {
		evCount += f.fetchLinkedEvents(ctx, relay, repliesAndReactionsGroupedByRelay, func(evID string) {
			delete(eventsAndReplies, evID)
		})
	}
	if len(repostedEventsGroupedByRelay) > 0 {
		evCount += f.fetchLinkedEvents(ctx, relay, repostedEventsGroupedByRelay, func(evID string) {
			delete(eventsAndReplies, evID)
		})
	}
	for _, ev := range eventsAndReplies {
		e := &model.Event{Event: *ev}
		if e.Validate() == nil {
			if aErr := f.AcceptEvent(ctx, e); aErr != nil {
				log.Fatal(aErr)
			}
			evCount += 1
		}
	}
	return evCount
}

func (f *fetcher) fetchLinkedEvents(ctx context.Context, currentRelay *nostr.Relay, groupedByRelay map[string][]string, onFailure func(evID string)) int {
	evCount := 0
	for relayUrl, eventsFromRelay := range groupedByRelay {
		origEvents, oErr := f.fetchPost(ctx, eventsFromRelay, relayUrl, currentRelay)
		if oErr != nil {
			for _, ev := range eventsFromRelay {
				onFailure(ev)
			}
		}
		// original events can be intermediate replies and we need to fetch further
		evList := make([]*nostr.Event, 0)
		for ev := range origEvents {
			evList = append(evList, ev)
		}
		if len(evList) > 0 {
			evCount += f.fetchLinkedAndProcessEvents(ctx, evList, currentRelay)
		}
	}
	return evCount
}

func extractEventRef(ev *nostr.Event, tag, currentRelayUrl string, res map[string][]string) bool {
	if eOrQTag := ev.Tags.GetFirst([]string{tag}); eOrQTag != nil {
		relayAddr := ""
		if len(*eOrQTag) >= 3 {
			relayAddr = (*eOrQTag)[2]
		}
		if relayAddr == "" {
			relayAddr = currentRelayUrl
		}
		res[relayAddr] = append(res[relayAddr], (*eOrQTag)[1])
		return true
	}
	return false
}

func (f *fetcher) fetchPost(ctx context.Context, events []string, relayUrl string, r *nostr.Relay) (chan *nostr.Event, error) {
	var err error
	if r == nil || relayUrl != r.URL {
		connCtx, connCancel := context.WithTimeout(ctx, 15*time.Second)
		defer func() {
			connCancel()
			r.Close()
		}()
		r, err = connectToRelay(connCtx, relayUrl)
		if err != nil {
			return nil, errorx.Withf(err, "failed to connect to %v", relayUrl)
		}
	}
	fetchedEvents, err := queryEvents(ctx, r, nostr.Filter{
		IDs: events,
	})
	if err != nil {
		return nil, errorx.Withf(err, "failed to fetch original posts for replies %#v from %v", events, relayUrl)
	}
	return fetchedEvents, err
}

func queryEvents(ctx context.Context, r *nostr.Relay, filter nostr.Filter) (chan *nostr.Event, error) {
	fetchedEvents, err := r.QueryEvents(ctx, filter)
	if err != nil {
		return fetchedEvents, errorx.Withf(err, "failed to query events from %v", r.URL)
	}
	return fetchedEvents, nil
}