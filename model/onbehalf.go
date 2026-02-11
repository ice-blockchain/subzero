// SPDX-License-Identifier: ice License 1.0

package model

import (
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"
)

type (
	OnBehalfAccessEntry struct {
		Start   *Timestamp
		End     *Timestamp
		Revoked *Timestamp
		Kinds   []int
	}
	OnBehalfAccessEntries struct {
		Records map[string]*OnBehalfAccessEntry
	}
)

const (
	TagAttestationName             = "p"
	TagAttestationValueIndexPubkey = 1
	TagAttestationValueIndexRelay  = 2
	TagAttestationValueIndexAction = 3
)

var (
	ErrAttestationRecordNotFound    = errors.New("attestation record not found")
	ErrAttestationRecordExpired     = errors.New("attestation record is expired")
	ErrAttestationRecordRevoked     = errors.New("attestation record is revoked")
	ErrAttestationRecordIsNotActive = errors.New("attestation record is not active yet")
)

func ParseAttestationString(s string) (action string, ts Timestamp, kinds []int, err error) {
	// Format: <action>:<timestamp>[:<kind1>,<kind2>,...]
	actionEnd := strings.IndexRune(s, ':')
	if actionEnd == -1 {
		// Just action.
		return s, 0, nil, errors.Errorf("missing timestamp in attestation string: %v", s)
	}

	tsStr := s[actionEnd+1:]
	action = s[:actionEnd]
	tsStrEnd := strings.IndexRune(tsStr, ':')
	if tsStrEnd == -1 {
		tsStrEnd = len(tsStr)
	}
	unix, err := strconv.ParseInt(tsStr[:tsStrEnd], 10, 64)
	if err != nil {
		return "", 0, nil, errors.Wrapf(err, "failed to parse timestamp in attestation string: %v", tsStr[:tsStrEnd])
	}
	ts = Timestamp(unix)
	if tsStrEnd == len(tsStr) {
		return action, ts, nil, nil
	}
	kindsTokens := strings.Split(tsStr[tsStrEnd+1:], ",")
	if len(kindsTokens) > 0 {
		kinds = make([]int, len(kindsTokens))
		for i, kindStr := range kindsTokens {
			kinds[i], err = strconv.Atoi(kindStr)
			if err != nil {
				return "", 0, nil, errors.Wrapf(err, "failed to parse kind %q in attestation string: %v", kindStr, s)
			}
		}
	}

	return action, ts, kinds, nil
}

func ParseAttestationTags(tags Tags) (*OnBehalfAccessEntries, error) {
	// List of onbehalf access entries, pubkey -> entry.
	entries := &OnBehalfAccessEntries{Records: make(map[string]*OnBehalfAccessEntry)}
	for _, tag := range tags {
		if len(tag) < 4 || tag.Key() != TagAttestationName {
			// Attetation tags are just a part of the regular tags, and regular tags may contain other tags, so just log and go on.
			log.Trace().
				Str("context", "MODEL").
				Strs("tag", tag).
				Msg("invalid attestation tag")

			continue
		}

		var entry *OnBehalfAccessEntry
		if e, ok := entries.Records[tag[TagAttestationValueIndexPubkey]]; ok {
			entry = e
		} else {
			entry = new(OnBehalfAccessEntry)
			entries.Records[tag[TagAttestationValueIndexPubkey]] = entry
		}

		action, ts, kinds, err := ParseAttestationString(tag[TagAttestationValueIndexAction])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse attestation string")
		}

		switch action {
		case CustomIONAttestationKindRevoked:
			// Revoke access.
			entry.Revoked = &ts
		case CustomIONAttestationKindActive:
			// Grant access.
			entry.Start = &ts
			entry.End = nil
			entry.Kinds = kinds
		case CustomIONAttestationKindInactive:
			// Remove access.
			entry.End = &ts
		}
	}

	return entries, nil
}

// AllowedKinds returns the allowed kinds for the given device key, or nil if the device key is not found.
// It does NOT check the validity of the attestation record (e.g. whether it is expired or revoked).
func (r *OnBehalfAccessEntries) AllowedKinds(deviceKey string) []int {
	entry, ok := r.Records[deviceKey]
	if !ok {
		return nil
	} else if len(entry.Kinds) == 0 {
		// All kinds are allowed.
		return nil
	}
	return slices.Clone(entry.Kinds)
}

// IsAccessAllowed checks if the given device key is allowed to act on behalf of the master public key for the given kind at the given time.
func (r *OnBehalfAccessEntries) IsAccessAllowed(deviceKey string, kind int, now Timestamp) (bool, error) {
	if kind == CustomIONKindAttestation {
		return false, nil
	}

	record, ok := r.Records[deviceKey]
	if !ok {
		return false, errors.Wrap(ErrAttestationRecordNotFound, deviceKey)
	}

	switch {
	case record.Revoked != nil && !now.Before(*record.Revoked):
		return false, errors.Wrap(ErrAttestationRecordRevoked, deviceKey)

	case record.End != nil && now.After(*record.End):
		return false, errors.Wrap(ErrAttestationRecordExpired, deviceKey)

	case record.Start == nil:
		return false, errors.Wrapf(ErrAttestationRecordNotFound, "start time is not set for device key %q", deviceKey)

	case now.Before(*record.Start):
		return false, errors.Wrap(ErrAttestationRecordIsNotActive, deviceKey)
	}

	if kind >= 0 && len(record.Kinds) > 0 && !slices.Contains(record.Kinds, kind) {
		return false, nil
	}

	return true, nil
}

func OnBehalfIsAccessAllowed(masterTags Tags, onBehalfPubkey string, kind int, now Timestamp) (bool, error) {
	entries, err := ParseAttestationTags(masterTags)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse attestation tags")
	}

	return entries.IsAccessAllowed(onBehalfPubkey, kind, now)
}
