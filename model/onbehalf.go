// SPDX-License-Identifier: ice License 1.0

package model

import (
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

type OnBehalfAccessEntry struct {
	Start   *time.Time
	End     *time.Time
	Revoked *time.Time
	Kinds   []int
}

const (
	TagAttestationName             = "p"
	TagAttestationValueIndexPubkey = 1
	TagAttestationValueIndexRelay  = 2
	TagAttestationValueIndexAction = 3
)

func ParseAttestationString(s string) (action string, ts time.Time, kinds []int, err error) {
	// Format: <action>:<timestamp>[:<kind1>,<kind2>,...]
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

func ParseAttestationTags(tags Tags) (map[string]*OnBehalfAccessEntry, error) {
	// List of onbehalf access entries, pubkey -> entry.
	entries := make(map[string]*OnBehalfAccessEntry)
	for _, tag := range tags {
		if len(tag) < 4 || tag.Key() != TagAttestationName {
			// Attetation tags are just a part of the regular tags, and regular tags may contain other tags, so just log and go on.
			log.Printf("invalid attestation tag: %+v", tag)

			continue
		}

		var entry *OnBehalfAccessEntry
		if e, ok := entries[tag[TagAttestationValueIndexPubkey]]; ok {
			entry = e
		} else {
			entry = new(OnBehalfAccessEntry)
			entries[tag[TagAttestationValueIndexPubkey]] = entry
		}

		action, ts, kinds, err := ParseAttestationString(tag[TagAttestationValueIndexAction])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse attestation string")
		}

		if action == IceAttestationKindRevoked {
			// Revoke access.
			entry.Revoked = &ts
		} else if action == IceAttestationKindActive {
			// Grant access.
			entry.Start = &ts
			entry.End = nil
			entry.Kinds = kinds
		} else if action == IceAttestationKindInactive {
			// Remove access.
			entry.End = &ts
		}
	}

	return entries, nil
}

func OnBehalfIsAccessAllowed(masterTags Tags, onBehalfPubkey string, kind int, nowUnix int64) (bool, error) {
	if kind == IceKindAttestation {
		// Explicitly forbid attestation access for all sub-accounts.
		return false, nil
	}

	entries, err := ParseAttestationTags(masterTags)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse attestation tags")
	}
	entry, ok := entries[onBehalfPubkey]
	if !ok || entry.Revoked != nil {
		return false, nil
	}

	if kind > 0 && len(entry.Kinds) > 0 && !slices.Contains(entry.Kinds, kind) {
		return false, nil
	}

	now := time.Unix(nowUnix, 0)

	return (now.After(*entry.Start)) &&
		(entry.End == nil || now.Before(*entry.End)), nil
}

func AttestationUpdateIsAllowed(old Tags, new Tags) bool {
	if len(new) < len(old) {
		// Deleting tags is not allowed.
		return false
	}

	// At this point, len(new) >= len(old).
	// Check if the new tags contain all the old tags.
	if slices.CompareFunc(old, new[:len(old)], slices.Compare) != 0 {
		return false
	}

	// List of revoked pubkeys.
	revoked := make(map[string]struct{})
	for _, tag := range old {
		if tag.Key() == TagAttestationName && len(tag) >= 4 && strings.HasPrefix(tag[TagAttestationValueIndexAction], IceAttestationKindRevoked) {
			revoked[tag[1]] = struct{}{}
		}
	}

	// Do a basic format check for the new tags.
	for _, tag := range new[len(old):] {
		if tag.Key() != TagAttestationName {
			// Just ignore non-attestation tags.
			continue
		}

		// Malformed attestation tag.
		if len(tag) < 4 {
			log.Printf("failed to parse attestation string: %#v: malformed tag", tag)

			return false
		}

		action, _, _, err := ParseAttestationString(tag[TagAttestationValueIndexAction])
		if err != nil {
			log.Printf("failed to parse attestation string: %#v: %v", tag, err)

			return false
		}

		if _, ok := revoked[tag[TagAttestationValueIndexPubkey]]; ok {
			log.Printf("found attestations for revoked pubkey: %v: %#v", tag[1], tag)

			return false
		}

		if action == IceAttestationKindRevoked {
			revoked[tag[TagAttestationValueIndexPubkey]] = struct{}{}
		}

	}

	return true
}
