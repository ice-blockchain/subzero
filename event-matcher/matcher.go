// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"iter"

	"github.com/RoaringBitmap/roaring/v2/roaring64"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/pool"
)

const (
	indexPoolBitmapCapacity = 1024
	indexMapDefaultCapacity = 100
	indexCartesianLimit     = 100
	anyKind                 = -1
)

type (
	// Value represents a generic value that can be indexed and matched against events. It must provide a Hash method for indexing.
	Value interface {
		// Hash returns a unique hash for the value, used for indexing and matching.
		Hash() uint64
	}

	// dimension identifies the logical type of the indexed value.
	dimension uint8

	// indexKey is the deterministic composite key used for flat map routing.
	indexKey struct {
		Value     string
		Kind      int
		Dimension dimension
	}

	// entry represents a single indexed entry in the matcher.
	entry[V Value] struct {
		Value       V          // The value associated with the filters.
		RoutingKeys []indexKey // The exact keys this entry was indexed under.
	}

	// matcher is responsible for indexing event filters and matching incoming events.
	matcher[V Value] struct {
		Mu          *xsync.RBMutex
		Values      *xsync.Map[uint64, entry[V]]   // V hash -> entry[V].
		Indexes     map[indexKey]*roaring64.Bitmap // indexKey -> Bitmap of V hashes.
		RoaringPool *pool.Pool[*roaring64.Bitmap]
	}
)

const (
	dimNone   dimension = iota // Generic (No specific tags/authors)
	dimAuthor                  // Event PubKey or master pubkey.
	dimTagP                    // 'p' tag.
	dimTagQ                    // 'Q' tag.
	dimTagK                    // 'k' tag.
)

func newEventMatcher[V Value]() *matcher[V] {
	return &matcher[V]{
		Mu:      xsync.NewRBMutex(),
		Values:  xsync.NewMap[uint64, entry[V]](),
		Indexes: make(map[indexKey]*roaring64.Bitmap, indexMapDefaultCapacity),
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

// Index adds the given value to the matcher, associating it with the filters derived from the value. If a value with the same hash already exists, it will be replaced.
func (s *matcher[V]) Index(rawFilters model.Filters, v V) {
	hash := v.Hash()
	keys := parseFilters(rawFilters)

	newEntry := entry[V]{
		Value:       v,
		RoutingKeys: keys,
	}

	oldValue, loaded := s.Values.LoadAndStore(hash, newEntry)
	if loaded {
		// If the value existed, remove old bindings first.
		s.removeEntry(hash, oldValue.RoutingKeys)
	}

	s.Mu.Lock()
	defer s.Mu.Unlock()

	for _, key := range keys {
		bm, exists := s.Indexes[key]
		if !exists {
			bm = s.RoaringPool.Get()
		}
		bm.Add(hash)
		s.Indexes[key] = bm
	}
}

func (s *matcher[V]) removeEntry(hash uint64, keys []indexKey) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	for _, key := range keys {
		bm, exists := s.Indexes[key]
		if !exists {
			continue
		}

		bm.Remove(hash)
		if bm.IsEmpty() {
			delete(s.Indexes, key)
			s.RoaringPool.Put(bm)
		} else {
			s.Indexes[key] = bm
		}
	}
}

// RemoveByHash removes the value associated with the given hash from the matcher. Returns true if the value was found and removed, false otherwise.
func (s *matcher[V]) RemoveByHash(hash uint64) bool {
	oldValue, loaded := s.Values.LoadAndDelete(hash)
	if loaded {
		s.removeEntry(hash, oldValue.RoutingKeys)
	}
	return loaded
}

// Remove removes the given value from the matcher. Returns true if the value was found and removed, false otherwise.
func (s *matcher[V]) Remove(v V) bool {
	return s.RemoveByHash(v.Hash())
}

// Size returns the total number of indexed values in the matcher.
func (s *matcher[V]) Size() int {
	return s.Values.Size()
}

// Get retrieves the value associated with the given event, if it exists.
func (s *matcher[V]) Get(ev *model.Event) (data []V) {
	s.Lookup(ev, func(v V) bool {
		data = append(data, v)
		return true
	})
	return data
}

// Lookup finds all potentially matching values (NOT EXACT MATCH) for the given event by checking against the indexed filters and yields them through the provided callback function.
// If the callback returns false, the lookup will stop early.
func (s *matcher[V]) Lookup(ev *model.Event, cb func(V) bool) {
	result := s.RoaringPool.Get()
	defer s.RoaringPool.Put(result)

	// Stack-allocated buffer prevents heap allocations during event deconstruction.
	var keyBuf [128]indexKey
	keys := parseEvent(ev, keyBuf[:0])

	token := s.Mu.RLock()
	for i := range keys {
		if bm, exists := s.Indexes[keys[i]]; exists && !bm.IsEmpty() {
			result.Or(bm)
		}
	}
	s.Mu.RUnlock(token)

	it := result.Iterator()
	for it.HasNext() {
		hash := it.Next()
		entry, loaded := s.Values.Load(hash)
		if !loaded {
			// Value was removed concurrently after the bitmap was copied. Safe to skip.
			continue
		}

		if !cb(entry.Value) {
			break
		}
	}
}

// Range returns a sequence of all values currently indexed in the matcher.
func (s *matcher[V]) Range() iter.Seq[V] {
	return func(yield func(V) bool) {
		s.Values.Range(func(key uint64, value entry[V]) bool {
			return yield(value.Value)
		})
	}
}
