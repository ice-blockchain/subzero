// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"iter"

	"github.com/RoaringBitmap/roaring/v2"
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
	// indexBitmap wraps a roaring.Bitmap with lazy initialization.
	indexBitmap struct {
		*roaring.Bitmap
	}
	// indexFilter represents a parsed filter with extracted tag values.
	indexFilter struct {
		*model.Filter                            // Embedded filter.
		MasterKeysByKind map[model.Kind][]string // Extracted p/Q tag values per kind.
		MasterKeys       []string                // All p/Q tag values from the filter.
	}
	// indexShard represents a shard of the index storage.
	indexShard struct {
		Mu            *xsync.RBMutex
		Subscriptions *xsync.Map[uint32, subscription]      // Subscription hash -> subscription.
		Generic       indexBitmap                           // Subscription bitmap for those without kind/p/Q tags.
		ByKind        map[model.Kind]indexBitmap            // Kind -> subscription bitmap, for those without p/Q tags.
		ByAuthor      map[string]indexBitmap                // Master pubkey from Q or p tag -> subscription bitmap, for those without kinds.
		ByKindAuthor  map[model.Kind]map[string]indexBitmap // Kind -> Master pubkey from Q or p tag -> subscription bitmap.
		RoaringPool   *pool.Pool[*roaring.Bitmap]           // Pool of bitmaps for lookups.
	}
	// indexStorage is the main index storage structure.
	indexStorage struct {
		Shards    []*indexShard
		NumShards uint32
	}
)

func (bm *indexBitmap) IsEmpty() bool {
	return bm.Bitmap == nil || bm.Bitmap.IsEmpty()
}

func (bm *indexBitmap) GetCardinality() uint64 {
	if bm.Bitmap == nil {
		return 0
	}
	return bm.Bitmap.GetCardinality()
}

func (bm *indexBitmap) Add(x uint32) *indexBitmap {
	if bm.Bitmap == nil {
		bm.Bitmap = roaring.New()
	}
	bm.Bitmap.Add(x)
	return bm
}

func (bm *indexBitmap) Remove(x uint32) *indexBitmap {
	if bm.Bitmap != nil {
		bm.Bitmap.Remove(x)
	}
	return bm
}

func (bm indexBitmap) String() string {
	if bm.Bitmap == nil {
		return "{nil}"
	}
	return bm.Bitmap.String()
}

func NewIndexShard() *indexShard {
	return &indexShard{
		Mu:            xsync.NewRBMutex(),
		Subscriptions: xsync.NewMap[uint32, subscription](),
		ByAuthor:      make(map[string]indexBitmap, indexMapDefaultCapacity),
		ByKind:        make(map[model.Kind]indexBitmap, indexMapDefaultCapacity),
		ByKindAuthor:  make(map[model.Kind]map[string]indexBitmap, indexMapDefaultCapacity),
		RoaringPool: pool.New(
			roaring.New,
			pool.WithDesiredNumberOfItems[*roaring.Bitmap](indexPoolBitmapCapacity),
			pool.WithPreFill[*roaring.Bitmap](true),
			pool.WithBeforeGet(func(bm *roaring.Bitmap) *roaring.Bitmap {
				bm.Clear()
				return bm
			}),
		),
	}
}

func hashSubscriptionID(ID string) uint32 {
	return uint32((xxh3.HashString(ID) & 0xFFFFFFFF))
}

func (*indexShard) ParseFilters(filters model.Filters) (parsedFilters []indexFilter) {
	for i := range filters {
		parsed := indexFilter{
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

func (s *indexShard) Index(conn Writer, sub *model.Subscription) {
	var masterKeys []string
	var kinds []model.Kind

	if sub.OneShot {
		// OneShot subscriptions are not stored, they are processed immediately.
		return
	}

	hash := hashSubscriptionID(sub.ID)
	filters := s.ParseFilters(sub.Filters)

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

		// If there are kinds AND p/Q tags, add to ByKindAuthor.
		case len(filters[i].Kinds) > 0 && len(filters[i].MasterKeysByKind) > 0:
			for _, kind := range filters[i].Kinds {
				bmByAuthor, exists := s.ByKindAuthor[kind]
				if !exists {
					bmByAuthor = make(map[string]indexBitmap, indexMapDefaultCapacity)
					s.ByKindAuthor[kind] = bmByAuthor
				}
				for _, masterKey := range filters[i].MasterKeysByKind[kind] {
					v := bmByAuthor[masterKey]
					v.Add(hash)
					bmByAuthor[masterKey] = v
				}
			}
			kinds = append(kinds, filters[i].Kinds...)
			masterKeys = append(masterKeys, filters[i].MasterKeys...)

		// If there are only kinds, add to ByKind.
		case len(filters[i].Kinds) > 0 && len(filters[i].MasterKeys) == 0:
			for _, kind := range filters[i].Kinds {
				v := s.ByKind[kind]
				v.Add(hash)
				s.ByKind[kind] = v
			}
			kinds = append(kinds, filters[i].Kinds...)

		// If there are only p/Q tags, add to ByAuthor.
		case len(filters[i].Kinds) == 0 && len(filters[i].MasterKeys) > 0:
			for _, masterKey := range filters[i].MasterKeys {
				v := s.ByAuthor[masterKey]
				v.Add(hash)
				s.ByAuthor[masterKey] = v
			}
			masterKeys = append(masterKeys, filters[i].MasterKeys...)

		default:
			log.Warn().Str("context", "index").
				Str("filter", sub.Filters[i].String()).
				Msg("unhandled filter indexing case")
			s.Generic.Add(hash)
		}
	}
	s.Mu.Unlock()

	_, loaded := s.Subscriptions.LoadAndStore(hash, subscription{
		Source:     sub,
		Writer:     conn,
		MasterKeys: model.DeduplicateStringSlice(masterKeys),
		Kinds:      model.DeduplicateIntSlice(kinds),
	})
	if loaded {
		log.Warn().Str("context", "index").Str("subscription_id", sub.ID).Msg("subscription already exists, overwriting it")
	}
}

func (s *indexShard) Size() int {
	return s.Subscriptions.Size()
}

func (s *indexShard) Remove(conn Writer, subID string) (*model.Subscription, bool) {
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

		if bmByAuthor, exists := s.ByKindAuthor[kind]; exists {
			for _, masterKey := range entry.MasterKeys {
				bm, exists := bmByAuthor[masterKey]
				if !exists {
					continue
				}
				if bm.Remove(hash).IsEmpty() {
					delete(bmByAuthor, masterKey)
				}
			}
			// Keep `kind` in ByKindAuthor even if empty to avoid reallocations.
		}
	}

	for _, masterKey := range entry.MasterKeys {
		bm, exists := s.ByAuthor[masterKey]
		if !exists {
			continue
		}
		if bm.Remove(hash).IsEmpty() {
			delete(s.ByAuthor, masterKey)
		}
	}

	s.Generic.Remove(hash)

	return entry.Source, true
}

func (s *indexShard) Get(ev *model.Event) (data []subscription) {
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
func (s *indexShard) Lookup(ev *model.Event, cb func(subscription) bool) {
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
		bm, exists := s.ByAuthor[targetKey]
		if exists && !bm.IsEmpty() {
			result.Or(bm.Bitmap)
		}

		// By kind+author (p/Q) subscriptions.
		m, exists := s.ByKindAuthor[ev.Kind]
		if exists {
			bm, exists := m[targetKey]
			if exists && !bm.IsEmpty() {
				result.Or(bm.Bitmap)
			}
		}
	}
	s.Mu.RUnlock(token)

	result.Iterate(func(x uint32) bool {
		sub, loaded := s.Subscriptions.Load(x)
		if !loaded {
			// Nothing we can do here, just skip.
			return true
		}
		return cb(sub)
	})
}

func newIndexStorage(numShards uint32) *indexStorage {
	is := &indexStorage{
		NumShards: numShards,
	}

	for range numShards {
		is.Shards = append(is.Shards, NewIndexShard())
	}

	return is
}

func (is *indexStorage) Index(conn Writer, sub *model.Subscription) {
	hash := hashSubscriptionID(sub.ID)
	shard := is.Shards[hash%is.NumShards]
	shard.Index(conn, sub)
}

func (is *indexStorage) Remove(conn Writer, subID string) (*model.Subscription, bool) {
	hash := hashSubscriptionID(subID)
	shard := is.Shards[hash%is.NumShards]
	return shard.Remove(conn, subID)
}

func (is *indexStorage) Size() (size int) {
	for i := range is.Shards {
		size += is.Shards[i].Size()
	}
	return size
}

func (is *indexStorage) Lookup(ev *model.Event) iter.Seq2[Writer, *model.Subscription] {
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
