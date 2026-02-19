// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"iter"
	"runtime"

	"github.com/ice-blockchain/subzero/model"
)

type (
	// Storage is the main index storage structure.
	Storage[V Value] struct {
		shards []*matcher[V] // Shards of matchers to reduce lock contention.
	}
)

// NewMatcherStorage initializes a new Storage with the specified number of shards for concurrency.
func NewMatcherStorage[V Value](numShards uint32) *Storage[V] {
	var is Storage[V]

	if numShards == 0 {
		numShards = min(uint32(runtime.NumCPU()), 4)
	}

	for range numShards {
		is.shards = append(is.shards, newEventMatcher[V]())
	}

	return &is
}

func (is *Storage[V]) getMatcherFor(hash uint64) *matcher[V] {
	return is.shards[hash%uint64(len(is.shards))]
}

// Index adds a value with its associated filters to the storage.
func (is *Storage[V]) Index(rawFilters model.Filters, v V) {
	is.getMatcherFor(v.Hash()).Index(rawFilters, v)
}

// Remove deletes a value from the storage based on its hash.
func (is *Storage[V]) Remove(v V) bool {
	return is.getMatcherFor(v.Hash()).Remove(v)
}

// RemoveByHash deletes a value from the storage based on its hash and returns whether it was found and removed.
func (is *Storage[V]) RemoveByHash(hash uint64) bool {
	return is.getMatcherFor(hash).RemoveByHash(hash)
}

// Size returns the total number of indexed values across all shards.
func (is *Storage[V]) Size() (size int) {
	for i := range is.shards {
		size += is.shards[i].Size()
	}
	return size
}

// Lookup finds all possible candidates (NOT EXACT MATCH) for the given event by checking against the indexed filters.
func (is *Storage[V]) Lookup(ev *model.Event) iter.Seq[V] {
	return func(yield func(V) bool) {
		for i := range is.shards {
			// Initialize it with true, so if current shard has no matches, we continue to the next shard.
			shouldContinue := true
			is.shards[i].Lookup(ev, func(v V) bool {
				shouldContinue = yield(v)
				return shouldContinue
			})
			// Stop early if requested.
			if !shouldContinue {
				return
			}
		}
	}
}

// Range returns a sequence of all values currently indexed in the storage.
func (is *Storage[V]) Range() iter.Seq[V] {
	return func(yield func(V) bool) {
		for i := range is.shards {
			for v := range is.shards[i].Range() {
				if !yield(v) {
					return
				}
			}
		}
	}
}
