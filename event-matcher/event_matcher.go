// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"github.com/RoaringBitmap/roaring/v2/roaring64"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/pool"
)

const (
	indexPoolBitmapCapacity = 1024
	indexMapDefaultCapacity = 100
)

type (
	// Value represents a generic value that can be indexed and matched against events. It must provide a Hash method for indexing.
	Value interface {
		// Hash returns a unique hash for the value, used for indexing and matching.
		Hash() uint64
	}

	// entry represents a single indexed entry in the matcher, containing the value and its associated parsed filters.
	entry[V Value] struct {
		Value   V             // The value associated with the filters, e.g. a subscription.
		Filters parsedFilters // The parsed filters associated with this value.
	}

	// matcher is responsible for indexing event filters and matching incoming events.
	matcher[V Value] struct {
		Mu                *xsync.RBMutex
		Values            *xsync.Map[uint64, entry[V]]            // V hash -> V.
		Generic           matcherBitmap                           // Filter bitmap for those without kind/p/Q tags.
		ByKind            map[model.Kind]matcherBitmap            // Kind -> bitmap, for those without p/Q tags.
		ByDestination     map[string]matcherBitmap                // Master pubkey from Q or p tag -> bitmap, for those without kinds.
		ByKindDestination map[model.Kind]map[string]matcherBitmap // Kind -> Master pubkey from Q or p tag -> bitmap.
		ByKindAuthor      map[model.Kind]map[string]matcherBitmap // Kind -> Author pubkey -> bitmap.
		RoaringPool       *pool.Pool[*roaring64.Bitmap]           // Pool of bitmaps for lookups.
	}
)

func newEventMatcher[V Value]() *matcher[V] {
	return &matcher[V]{
		Mu:                xsync.NewRBMutex(),
		Values:            xsync.NewMap[uint64, entry[V]](),
		ByDestination:     make(map[string]matcherBitmap, indexMapDefaultCapacity),
		ByKind:            make(map[model.Kind]matcherBitmap, indexMapDefaultCapacity),
		ByKindAuthor:      make(map[model.Kind]map[string]matcherBitmap, indexMapDefaultCapacity),
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

func (s *matcher[V]) Index(rawFilters model.Filters, v V) {
	hash := v.Hash()
	filters := parseFilters(rawFilters)
	entry := entry[V]{
		Value:   v,
		Filters: filters,
	}

	oldValue, loaded := s.Values.LoadAndStore(hash, entry)
	if loaded {
		// If the value already exists, we need to remove the old filter from the index before adding the new one.
		s.removeEntry(oldValue)
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
		case len(filters[i].Kinds) > 0 && len(filters[i].MasterKeys) == 0 && len(filters[i].Authors) == 0:
			for _, kind := range filters[i].Kinds {
				v := s.ByKind[kind]
				v.Add(hash)
				s.ByKind[kind] = v
			}

		// If there are kinds and authors, but no p/Q tags, add to ByKindAuthor.
		case len(filters[i].Kinds) > 0 && len(filters[i].MasterKeys) == 0 && len(filters[i].Authors) > 0:
			for _, kind := range filters[i].Kinds {
				bmByAuthor, exists := s.ByKindAuthor[kind]
				if !exists {
					bmByAuthor = make(map[string]matcherBitmap, indexMapDefaultCapacity)
					s.ByKindAuthor[kind] = bmByAuthor
				}
				for _, author := range filters[i].Authors {
					v := bmByAuthor[author]
					v.Add(hash)
					bmByAuthor[author] = v
				}
			}

		// If there are only p/Q tags, add to ByDestination.
		case len(filters[i].Kinds) == 0 && len(filters[i].MasterKeys) > 0:
			for _, masterKey := range filters[i].MasterKeys {
				v := s.ByDestination[masterKey]
				v.Add(hash)
				s.ByDestination[masterKey] = v
			}

		default:
			s.Generic.Add(hash)
		}
	}
	s.Mu.Unlock()
}

func (s *matcher[V]) Size() int {
	return s.Values.Size()
}

func (s *matcher[V]) removeEntry(entry entry[V]) {
	keys, kinds := entry.Filters.Meta()
	hash := entry.Value.Hash()

	s.Mu.Lock()
	defer s.Mu.Unlock()

	for _, kind := range kinds {
		if bm, exists := s.ByKind[kind]; exists {
			bm.Remove(hash)
			// We do not delete empty bitmaps from ByKind to avoid additional reallocations later.
		}

		if bmByDestination, exists := s.ByKindDestination[kind]; exists {
			for _, masterKey := range keys {
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

		if bmByAuthor, exists := s.ByKindAuthor[kind]; exists {
			for _, author := range keys {
				bm, exists := bmByAuthor[author]
				if !exists {
					continue
				}
				if bm.Remove(hash).IsEmpty() {
					delete(bmByAuthor, author)
				}
			}
			// Keep `kind` in ByKindAuthor even if empty to avoid reallocations.
		}
	}

	for _, masterKey := range keys {
		bm, exists := s.ByDestination[masterKey]
		if !exists {
			continue
		}
		if bm.Remove(hash).IsEmpty() {
			delete(s.ByDestination, masterKey)
		}
	}

	s.Generic.Remove(hash)
}

func (s *matcher[V]) RemoveByHash(hash uint64) bool {
	oldValue, loaded := s.Values.LoadAndDelete(hash)
	if loaded {
		s.removeEntry(oldValue)
	}
	return loaded
}

func (s *matcher[V]) Remove(v V) bool {
	return s.RemoveByHash(v.Hash())
}

// Get retrieves the value associated with the given event, if it exists.
func (s *matcher[V]) Get(ev *model.Event) (data []V) {
	s.Lookup(ev, func(v V) bool {
		data = append(data, v)
		return true
	})
	return data
}

// Lookup finds all values matching the given event and calls the callback for each match. It stops if the callback returns false.
func (s *matcher[V]) Lookup(ev *model.Event, cb func(V) bool) {
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

	if bmByAuthor, exists := s.ByKindAuthor[ev.Kind]; exists {
		bm, exists := bmByAuthor[ev.PubKey]
		if exists && !bm.IsEmpty() {
			result.Or(bm.Bitmap)
		}
		bm, exists = bmByAuthor[ev.GetMasterPublicKey()]
		if exists && !bm.IsEmpty() {
			result.Or(bm.Bitmap)
		}
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
		v, loaded := s.Values.Load(x)
		if !loaded {
			// Nothing we can do here, just skip.
			continue
		}
		if !cb(v.Value) {
			break
		}
	}
}
