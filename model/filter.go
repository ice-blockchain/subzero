// SPDX-License-Identifier: ice License 1.0

package model

import (
	"slices"
	"strconv"

	"github.com/nbd-wtf/go-nostr"
)

type (
	TagMap    = nostr.TagMap
	TagValues = nostr.TagValues
	Filter    = nostr.Filter
	Filters   = nostr.Filters
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
		CustomIONKindRepostOfTokenizedCommunityDefination: CustomIONKindTokenizedCommunityDefinition,
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

	if filter.Addresses != nil && !slices.Contains(filter.Addresses, event.Address()) {
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
