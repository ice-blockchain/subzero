// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func sqlObehalfIsAllowed(masterJsonTags, onBehalfPubkey, masterPubkey string, kind int, nowUnix int64) (bool, error) {
	if masterJsonTags == "" || masterJsonTags == "[]" {
		return false, nil
	}

	var tags model.Tags
	err := json.Unmarshal([]byte(masterJsonTags), &tags)
	if err != nil {
		return false, errors.Wrapf(err, "%v: failed to unmarshal tags", masterPubkey)
	}

	return model.OnBehalfIsAccessAllowed(tags, onBehalfPubkey, kind, nowUnix)
}

func sqlEventTagsReorderJSON(jsonTags string) (string, error) {
	const (
		replyMarkerIndex = 3 // event_tag_value3.
		patchMarkerIndex = 5 // event_tag_value5.
	)
	var tags model.Tags

	if jsonTags == "" {
		return "[]", nil
	}

	if err := tags.Scan(jsonTags); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal tags")
	}

	hasReply := false
	for i := range tags {
		tags[i] = eventTagsReorder(tags[i])
		hasReply = hasReply || (tags[i].Key() == "e" && len(tags[i]) > replyMarkerIndex && strings.EqualFold(tags[i][replyMarkerIndex], "reply"))
	}

	if hasReply {
		for i := range tags {
			if tags[i].Key() == "e" && len(tags[i]) > replyMarkerIndex && strings.EqualFold(tags[i][replyMarkerIndex], "root") {
				for len(tags[i]) < (patchMarkerIndex + 1) {
					// Fill the missing indexes with empty strings.
					tags[i] = append(tags[i], "")
				}
				tags[i][patchMarkerIndex] = "reply_of_root"
			}
		}
	}

	data, err := json.Marshal(tags)

	return string(data), errors.Wrap(err, "failed to marshal tags")
}

func sqlAttestationUpdateIsAllowed(oldTagsJSON, newTagsJSON string) (bool, error) {
	var oldTags, newTags model.Tags

	if err := json.Unmarshal([]byte(oldTagsJSON), &oldTags); err != nil {
		return false, errors.Wrap(err, "failed to unmarshal old tags")
	}
	if err := json.Unmarshal([]byte(newTagsJSON), &newTags); err != nil {
		return false, errors.Wrap(err, "failed to unmarshal new tags")
	}

	return model.AttestationUpdateIsAllowed(oldTags, newTags), nil
}

func sqlTagAGetAt(pos int) func(string) string {
	return func(tag string) string {
		fields := strings.Split(tag, ":")
		if len(fields) <= pos {
			return ""
		}

		return fields[pos]
	}
}

func sqlGetEventAddress(eventID string, kind int, masterPubkey, dTag string) string {
	if nostr.IsAddressableKind(kind) {
		return strconv.Itoa(kind) + ":" + masterPubkey + ":" + dTag
	} else if nostr.IsReplaceableKind(kind) {
		return strconv.Itoa(kind) + ":" + masterPubkey + ":"
	}
	return eventID
}
