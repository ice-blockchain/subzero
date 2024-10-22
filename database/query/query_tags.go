// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strings"

	"github.com/ice-blockchain/subzero/model"
)

const (
	// Tags/values for `imeta`.
	tagValueURL      = `event_tag_value1` // `url <actual url>`
	tagValueMimeType = `event_tag_value2` // `m <actual mime type>`

	// Tags/values for `expiration`.
	tagValueExpiration = `event_tag_value1`
)

// tagsMarshalIndexes is a map of tag names to their indexes in the marshaled tags.
// Key - tag name.
// Value - tag index in the marshaled tags.
var tagsMarshalIndexes = map[string]int{
	"url": 0,
	"m":   1,
}

func eventTagsReorder(tag model.Tag) model.Tag {
	// Format is: <key>, <key value>, <key value>, ...
	start := 0
	if strings.EqualFold(tag.Key(), "imeta") {
		start = 1
	}

	for pairIndex := start; pairIndex < len(tag); pairIndex++ {
		key, _, found := strings.Cut(tag[pairIndex], " ")
		if !found {
			continue
		}

		newIndex, ok := tagsMarshalIndexes[strings.ToLower(key)]
		if !ok || newIndex+start == pairIndex {
			continue
		}

		for newIndex+start >= len(tag) {
			tag = append(tag, "")
		}

		tag[pairIndex], tag[newIndex+start] = tag[newIndex+start], tag[pairIndex]
	}

	return tag
}
