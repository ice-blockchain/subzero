// SPDX-License-Identifier: ice License 1.0

package model

import (
	"slices"
	"strconv"

	"github.com/goccy/go-json"
	"github.com/nbd-wtf/go-nostr"
	"github.com/tidwall/gjson"
)

type (
	TagMap    = nostr.TagMap
	TagValues = nostr.TagValues
	Filter    = nostr.Filter
	Filters   = nostr.Filters

	FiltersWithData[T any] struct {
		Filters Filters
		Data    []T
	}

	FiltersWithEvents = FiltersWithData[*Event]
)

func filterMatchAuthors(filter *Filter, event *Event, currentMasterKey, currentDeviceKey string) bool {
	eventMasterKey := event.GetMasterPublicKey()
	for _, author := range filter.Authors {
		switch author {
		case event.PubKey, eventMasterKey:
			// Direct match with the event's public key or master key.
			return true
		case currentDeviceKey, currentMasterKey:
			// If we have a current device or master key in the filter, check them against the event's keys.
			if event.PubKey == currentDeviceKey ||
				eventMasterKey == currentDeviceKey ||
				event.PubKey == currentMasterKey ||
				eventMasterKey == currentMasterKey {
				return true
			}
		}
	}
	return false
}

func filterMatchKind(filter *Filter, ev *Event) bool {
	repostOfKinds := map[int]int{
		CustomIONKindRepostOfArticle:                      nostr.KindArticle,
		CustomIONKindRepostOfEditableTextNote:             CustomIONKindEditableTextNote,
		CustomIONKindRepostOfTokenizedCommunityDefinition: CustomIONKindTokenizedCommunityDefinition,
		CustomIONKindRepostOfTokenizedCommunityAction:     CustomIONKindTokenizedCommunityAction,
	}

	if len(filter.Kinds) == 0 {
		return true // No kind filter means all kinds match.
	}

	for _, k := range filter.Kinds {
		switch k {
		case ev.Kind:
			// Direct match with the given kind.
			return true
		case -ev.Kind:
			// Negative kind match, meaning we want to exclude this kind.
			return false
		default:
			// Check for repost kinds.
			if ev.Kind == nostr.KindGenericRepost {
				if val, err := strconv.Atoi(ev.GetTag("k").Value()); val > 0 && err == nil {
					if repostOriginalKind, ok := repostOfKinds[k]; ok && val == repostOriginalKind {
						return true
					}
				}
			}
		}

	}

	return false
}

func filterMatchAddresses(filter *Filter, event *Event) bool {
	addr := event.Address()
	for i := range filter.Addresses {
		switch filter.Addresses[i] {
		case addr, event.ID:
			return true
		}
	}
	return false
}

func FilterMatch(filter *Filter, event *Event, currentMasterKey, currentDeviceKey string) bool {
	if filter.Since != nil && event.CreatedAt.Before(*filter.Since) {
		return false
	}

	if filter.Until != nil && event.CreatedAt.After(*filter.Until) {
		return false
	}

	if filter.IDs != nil && !slices.Contains(filter.IDs, event.ID) {
		return false
	}

	if filter.Kinds != nil && !filterMatchKind(filter, event) {
		return false
	}

	if filter.Authors != nil && !filterMatchAuthors(filter, event, currentMasterKey, currentDeviceKey) {
		return false
	}

	if filter.Addresses != nil && !filterMatchAddresses(filter, event) {
		return false
	}

	if filter.Tags != nil && !filter.MatchesTags(event.Tags) {
		return false
	}

	return true
}

func FiltersMatch(filters Filters, event *Event, currentMasterKey, currentDeviceKey string) bool {
	if len(filters) == 0 {
		return true // No filters means all events match.
	}
	for idx := range filters {
		if FilterMatch(&filters[idx], event, currentMasterKey, currentDeviceKey) {
			return true
		}
	}
	return false
}

func (f *FiltersWithData[T]) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()

	for _, item := range arr {
		raw := item.Raw
		isFilter := item.Get("ids").Exists() ||
			item.Get("authors").Exists() ||
			item.Get("kinds").Exists() ||
			item.Get("addresses").Exists() ||
			item.Get("since").Exists() ||
			item.Get("until").Exists() ||
			item.Get("limit").Exists() ||
			item.Get("search").Exists()

		// Also check for tag filters (fields starting with #).
		if !isFilter {
			item.ForEach(func(key, value gjson.Result) bool {
				if key.Type == gjson.String && len(key.String()) > 0 && key.String()[0] == '#' {
					isFilter = true
					return false
				}
				return true
			})
		}

		if isFilter {
			var filter Filter
			if err := json.Unmarshal([]byte(raw), &filter); err != nil {
				return err
			}
			f.Filters = append(f.Filters, filter)
		} else {
			var d T
			if err := json.Unmarshal([]byte(raw), &d); err != nil {
				return err
			}
			f.Data = append(f.Data, d)
		}
	}

	return nil
}

func (f FiltersWithData[T]) MarshalJSON() ([]byte, error) {
	items := make([]json.RawMessage, 0, len(f.Filters)+len(f.Data))

	for _, filter := range f.Filters {
		b, err := json.Marshal(filter)
		if err != nil {
			return nil, err
		}
		items = append(items, b)
	}

	for _, d := range f.Data {
		b, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		items = append(items, b)
	}

	return json.Marshal(items)
}

func (f FiltersWithData[T]) String() string {
	b, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(b)
}
