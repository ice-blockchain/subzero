// SPDX-License-Identifier: ice License 1.0

package query

import (
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

type onbehalfAccessEntry struct {
	Start    *time.Time
	End      *time.Time
	Reworked *time.Time
	Kinds    []int
}

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

func parseAttestationString(s string) (action string, ts time.Time, kinds []int, err error) {
	// Format: <action>[:<timestamp>][:<kind1>,<kind2>,...]
	actionEnd := strings.IndexRune(s, ':')
	if actionEnd == -1 {
		// Just action.
		return s, time.Time{}, nil, errors.Errorf("missing timestamp in attestation string: %v", s)
	}

	tsStr := s[actionEnd+1:]
	action = s[:actionEnd]
	tsStrEnd := strings.IndexRune(tsStr, ':')
	if tsStrEnd == -1 {
		tsStrEnd = len(tsStr)
	}
	unix, err := strconv.ParseInt(tsStr[:tsStrEnd], 10, 64)
	if err != nil {
		return "", time.Time{}, nil, errors.Wrapf(err, "failed to parse timestamp in attestation string: %v", tsStr[:tsStrEnd])
	}
	ts = time.Unix(unix, 0)
	if tsStrEnd == len(tsStr) {
		return action, ts, nil, nil
	}
	kindsTokens := strings.Split(tsStr[tsStrEnd+1:], ",")
	if len(kindsTokens) > 0 {
		kinds = make([]int, len(kindsTokens))
		for i, kindStr := range kindsTokens {
			kinds[i], err = strconv.Atoi(kindStr)
			if err != nil {
				return "", time.Time{}, nil, errors.Wrapf(err, "failed to parse kind %q in attestation string: %v", kindStr, s)
			}
		}
	}

	return action, ts, kinds, nil
}

func parseAttestationTags(tags model.Tags) map[string]*onbehalfAccessEntry {
	const (
		tagIdxPubkey = 1
		tagIdxAttest = 3
	)

	// List of onbehalf access entries, pubkey -> entry.
	entries := make(map[string]*onbehalfAccessEntry)
	for _, tag := range tags {
		if len(tag) < 4 || tag.Key() != model.IceTagAttestation {
			log.Printf("invalid attestation tag: %+v", tag)

			continue
		}

		var entry *onbehalfAccessEntry
		if e, ok := entries[tag[tagIdxPubkey]]; ok {
			entry = e
		} else {
			entry = new(onbehalfAccessEntry)
			entries[tag[tagIdxPubkey]] = entry
		}

		action, ts, kinds, err := parseAttestationString(tag[tagIdxAttest])
		if err != nil {
			log.Printf("failed to parse attestation string: %v", err)

			continue
		}

		if action == model.IceAttestationKindRevoked {
			// Revoke access.
			entry.End = &ts
		} else if action == model.IceAttestationKindActive {
			// Grant access.
			entry.Start = &ts
			entry.End = nil
			entry.Kinds = kinds
		} else if action == model.IceAttestationKindInactive {
			// Remove access.
			entry.End = &ts
		} else {
			log.Printf("unknown attestation action: %v: %v", action, tag)
		}
	}

	return entries
}

func onBehalfIsAllowed(tags model.Tags, onBehalfPubkey string, kind int, nowUnix int64) bool {
	entries := parseAttestationTags(tags)
	entry, ok := entries[onBehalfPubkey]
	if !ok || entry.Reworked != nil {
		return false
	}

	if len(entry.Kinds) > 0 && !slices.Contains(entry.Kinds, kind) {
		return false
	}

	now := time.Unix(nowUnix, 0)

	return (now.After(*entry.Start)) &&
		(entry.End == nil || now.Before(*entry.End))
}
