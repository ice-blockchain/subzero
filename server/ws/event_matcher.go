// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"iter"

	"github.com/RoaringBitmap/roaring/v2/roaring64"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/zeebo/xxh3"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal/pool"
)

const (
	indexPoolBitmapCapacity = 1024
	indexMapDefaultCapacity = 100
)

type (
	// matcherBitmap wraps a roaring.Bitmap with lazy initialization.
	matcherBitmap struct {
		*roaring64.Bitmap
	}
	// parsedFilter represents a parsed filter with extracted tag values.
	parsedFilter struct {
		*model.Filter                            // Embedded filter.
		MasterKeysByKind map[model.Kind][]string // Extracted p/Q tag values per kind.
		MasterKeys       []string                // All p/Q tag values from the filter.
	}
	parsedFilters []parsedFilter
	eventMatcher  struct {
		Mu                *xsync.RBMutex
		Subscriptions     *xsync.Map[uint64, subscription]        // Subscription hash -> subscription.
		Generic           matcherBitmap                           // Subscription bitmap for those without kind/p/Q tags.
		ByKind            map[model.Kind]matcherBitmap            // Kind -> subscription bitmap, for those without p/Q tags.
		ByDestination     map[string]matcherBitmap                // Master pubkey from Q or p tag -> subscription bitmap, for those without kinds.
		ByKindDestination map[model.Kind]map[string]matcherBitmap // Kind -> Master pubkey from Q or p tag -> subscription bitmap.
		RoaringPool       *pool.Pool[*roaring64.Bitmap]           // Pool of bitmaps for lookups.
	}
	// eventMatcherStorage is the main index storage structure.
	eventMatcherStorage struct {
		Shards []*eventMatcher
	}
)

func (bm *matcherBitmap) IsEmpty() bool {
	return bm.Bitmap == nil || bm.Bitmap.IsEmpty()
}

func (bm *matcherBitmap) GetCardinality() uint64 {
	if bm.Bitmap == nil {
		return 0
	}
	return bm.Bitmap.GetCardinality()
}

func (bm *matcherBitmap) Add(x uint64) *matcherBitmap {
	if bm.Bitmap == nil {
		bm.Bitmap = roaring64.New()
	}
	bm.Bitmap.Add(x)
	return bm
}

func (bm *matcherBitmap) Remove(x uint64) *matcherBitmap {
	if bm.Bitmap != nil {
		bm.Bitmap.Remove(x)
	}
	return bm
}

func (bm matcherBitmap) String() string {
	if bm.Bitmap == nil {
		return "{nil}"
	}
	return bm.Bitmap.String()
}

func newEventMatcher() *eventMatcher {
	return &eventMatcher{
		Mu:                xsync.NewRBMutex(),
		Subscriptions:     xsync.NewMap[uint64, subscription](),
		ByDestination:     make(map[string]matcherBitmap, indexMapDefaultCapacity),
		ByKind:            make(map[model.Kind]matcherBitmap, indexMapDefaultCapacity),
		ByKindDestination: make(map[model.Kind]map[string]matcherBitmap, indexMapDefaultCapacity),
		RoaringPool: pool.New(
			roaring64.New,
			pool.WithDesiredNumberOfItems[*roaring64.Bitmap](indexPoolBitmapCapacity),
			pool.WithPreFill[*roaring64.Bitmap](true),
			pool.WithBeforeGet(func(bm *roaring64.Bitmap) *roaring64.Bitmap {
				bm.Clear()
				return bm
			}),
		),
	}
}

func hashSubscriptionID(ID string) uint64 {
	return xxh3.HashString(ID)
}

func (*eventMatcher) ParseFilters(filters model.Filters) (parsedFilters parsedFilters) {
	for i := range filters {
		parsed := parsedFilter{
			Filter:           &filters[i],
			MasterKeysByKind: make(map[model.Kind][]string),
		}

	tagsLoop:
		for k, v := range filters[i].Tags {
			if len(v) != 1 {
				// Expected formats:
				// "p": [[currentUserMasterPubkey]].
				// "p": [[currentUserMasterPubkey, "", currentDevicePubkey]].
				// "Q": [[null, null, currentUserMasterPubkey]].
				continue
			}

			var targetKey string
			switch k {
			case "p":
				if len(v[0]) < 1 || v[0][0] == nil || *v[0][0] == "" {
					continue tagsLoop
				}
				targetKey = k + *v[0][0] // Prefix to avoid collision with Q tags.

			case model.CustomIONTagAddressableQ:
				if len(v[0]) != 3 || v[0][0] != nil || v[0][1] != nil || v[0][2] == nil || *v[0][2] == "" {
					continue tagsLoop
				}
				targetKey = k + *v[0][2]

			default:
				continue tagsLoop
			}

			if len(filters[i].Kinds) > 0 {
				for _, kind := range filters[i].Kinds {
					parsed.MasterKeysByKind[kind] = append(parsed.MasterKeysByKind[kind], targetKey)
				}
			}
			parsed.MasterKeys = append(parsed.MasterKeys, targetKey)
		}
		parsedFilters = append(parsedFilters, parsed)
	}
	return parsedFilters
}

func (p parsedFilters) Meta() (keys []string, kinds []model.Kind) {
	for i := range p {
		if len(p[i].Kinds) > 0 {
			kinds = append(kinds, p[i].Kinds...)
		}
		if len(p[i].MasterKeys) > 0 {
			keys = append(keys, p[i].MasterKeys...)
		}
	}

	return model.DeduplicateStringSlice(keys), model.DeduplicateIntSlice(kinds)
}

func (s *eventMatcher) Index(conn Writer, sub *model.Subscription) bool {
	if sub.OneShot {
		// OneShot subscriptions are not stored, they are processed immediately.
		return false
	}

	hash := hashSubscriptionID(sub.ID)
	filters := s.ParseFilters(sub.Filters)
	keys, kinds := filters.Meta()
	newSub := subscription{
		Source:     sub,
		Writer:     conn,
		MasterKeys: keys,
		Kinds:      kinds,
	}

	value, exist := s.Subscriptions.Compute(hash, func(oldValue subscription, loaded bool) (subscription, xsync.ComputeOp) {
		if !loaded || oldValue.Writer == conn {
			return newSub, xsync.UpdateOp
		}
		return oldValue, xsync.CancelOp
	})
	if !exist || value.Writer != conn {
		// This subscription belongs to a different connection, do not index it.
		log.Warn().
			Str("context", "index").
			Str("subscription_id", sub.ID).
			Msg("subscription exists with different writer, not updating")
		return false
	}

	s.Mu.Lock()
	if len(filters) == 0 {
		// Empty filters means all events, so add to generic.
		s.Generic.Add(hash)
	}
	for i := range filters {
		switch {
		// If there are no kinds or p/Q tags, add to generic.
		case len(filters[i].Kinds) == 0 && len(filters[i].MasterKeys) == 0:
			s.Generic.Add(hash)

		// If there are kinds AND p/Q tags, add to ByKindDestination.
		case len(filters[i].Kinds) > 0 && len(filters[i].MasterKeysByKind) > 0:
			for _, kind := range filters[i].Kinds {
				bmByDestination, exists := s.ByKindDestination[kind]
				if !exists {
					bmByDestination = make(map[string]matcherBitmap, indexMapDefaultCapacity)
					s.ByKindDestination[kind] = bmByDestination
				}
				for _, masterKey := range filters[i].MasterKeysByKind[kind] {
					v := bmByDestination[masterKey]
					v.Add(hash)
					bmByDestination[masterKey] = v
				}
			}

		// If there are only kinds, add to ByKind.
		case len(filters[i].Kinds) > 0 && len(filters[i].MasterKeys) == 0:
			for _, kind := range filters[i].Kinds {
				v := s.ByKind[kind]
				v.Add(hash)
				s.ByKind[kind] = v
			}

		// If there are only p/Q tags, add to ByDestination.
		case len(filters[i].Kinds) == 0 && len(filters[i].MasterKeys) > 0:
			for _, masterKey := range filters[i].MasterKeys {
				v := s.ByDestination[masterKey]
				v.Add(hash)
				s.ByDestination[masterKey] = v
			}

		default:
			log.Warn().Str("context", "index").
				Str("filter", sub.Filters[i].String()).
				Msg("unhandled filter indexing case")
			s.Generic.Add(hash)
		}
	}
	s.Mu.Unlock()

	return true
}

func (s *eventMatcher) Size() int {
	return s.Subscriptions.Size()
}

func (s *eventMatcher) Remove(conn Writer, subID string) (*model.Subscription, bool) {
	hash := hashSubscriptionID(subID)

	entry, stillExist := s.Subscriptions.Compute(hash, func(value subscription, loaded bool) (subscription, xsync.ComputeOp) {
		if !loaded {
			return value, xsync.CancelOp
		}

		if value.Writer != nil && value.Writer != conn {
			// Different writer, do not delete.
			return value, xsync.CancelOp
		}
		return value, xsync.DeleteOp
	})
	if stillExist || entry.Source == nil {
		// Either caller's writer did not match, or subscription did not exist.
		return nil, false
	}

	s.Mu.Lock()
	defer s.Mu.Unlock()

	for _, kind := range entry.Kinds {
		if bm, exists := s.ByKind[kind]; exists {
			bm.Remove(hash)
			// We do not delete empty bitmaps from ByKind to avoid additional reallocations later.
		}

		if bmByDestination, exists := s.ByKindDestination[kind]; exists {
			for _, masterKey := range entry.MasterKeys {
				bm, exists := bmByDestination[masterKey]
				if !exists {
					continue
				}
				if bm.Remove(hash).IsEmpty() {
					delete(bmByDestination, masterKey)
				}
			}
			// Keep `kind` in ByKindDestination even if empty to avoid reallocations.
		}
	}

	for _, masterKey := range entry.MasterKeys {
		bm, exists := s.ByDestination[masterKey]
		if !exists {
			continue
		}
		if bm.Remove(hash).IsEmpty() {
			delete(s.ByDestination, masterKey)
		}
	}

	s.Generic.Remove(hash)

	return entry.Source, true
}

func (s *eventMatcher) Get(ev *model.Event) (data []subscription) {
	s.Lookup(ev, func(sub subscription) bool {
		data = append(data, sub)
		return true
	})
	return data
}

// Lookup finds all subscriptions matching the given event and calls the provided
// callback function for each matching subscription.
// The callback function should return true to continue iterating over subscriptions
// or false to stop the iteration.
func (s *eventMatcher) Lookup(ev *model.Event, cb func(subscription) bool) {
	result := s.RoaringPool.Get()
	defer s.RoaringPool.Put(result)

	token := s.Mu.RLock()

	// Start with generic subscriptions.
	if !s.Generic.IsEmpty() {
		result.Or(s.Generic.Bitmap)
	}

	// Add kind-based subscriptions.
	bm, exists := s.ByKind[ev.Kind]
	if exists && !bm.IsEmpty() {
		result.Or(bm.Bitmap)
	}

	for i := range ev.Tags {
		var targetKey string

		switch ev.Tags[i].Key() {
		case "p":
			targetKey = ev.Tags[i].Key() + ev.Tags[i].Value()

		case model.CustomIONTagAddressableQ:
			if len(ev.Tags[i]) > 3 {
				targetKey = ev.Tags[i].Key() + ev.Tags[i][3]
			}
		}

		if targetKey == "" {
			continue
		}

		// By general author (p/Q) subscriptions.
		bm, exists := s.ByDestination[targetKey]
		if exists && !bm.IsEmpty() {
			result.Or(bm.Bitmap)
		}

		// By kind+author (p/Q) subscriptions.
		m, exists := s.ByKindDestination[ev.Kind]
		if exists {
			bm, exists := m[targetKey]
			if exists && !bm.IsEmpty() {
				result.Or(bm.Bitmap)
			}
		}
	}
	s.Mu.RUnlock(token)

	it := result.Iterator()
	for it.HasNext() {
		x := it.Next()
		sub, loaded := s.Subscriptions.Load(x)
		if !loaded {
			// Nothing we can do here, just skip.
			continue
		}
		if !cb(sub) {
			break
		}
	}
}

func newEventMatcherStorage(numShards uint32) *eventMatcherStorage {
	var is eventMatcherStorage

	for range numShards {
		is.Shards = append(is.Shards, newEventMatcher())
	}

	return &is
}

func (is *eventMatcherStorage) getMatcherFor(subID string) *eventMatcher {
	hash := hashSubscriptionID(subID)

	return is.Shards[hash%uint64(len(is.Shards))]
}

func (is *eventMatcherStorage) Index(conn Writer, sub *model.Subscription) bool {
	return is.getMatcherFor(sub.ID).Index(conn, sub)
}

func (is *eventMatcherStorage) Remove(conn Writer, subID string) (*model.Subscription, bool) {
	return is.getMatcherFor(subID).Remove(conn, subID)
}

func (is *eventMatcherStorage) Size() (size int) {
	for i := range is.Shards {
		size += is.Shards[i].Size()
	}
	return size
}

func (is *eventMatcherStorage) Lookup(ev *model.Event) iter.Seq2[Writer, *model.Subscription] {
	return func(yield func(Writer, *model.Subscription) bool) {
		for i := range is.Shards {
			// Initialize it with true, so if current shard has no matches, we continue to the next shard.
			shouldContinue := true
			is.Shards[i].Lookup(ev, func(sub subscription) bool {
				shouldContinue = yield(sub.Writer, sub.Source)
				return shouldContinue
			})
			// Stop early if requested.
			if !shouldContinue {
				return
			}
		}
	}
}
