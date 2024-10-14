package query

import (
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func sqlObehalfIsAllowed(masterJsonTags, onBehalfPubkey, masterPubkey string, kind int, nowUnix int64) (bool, error) {
	if masterJsonTags == "" {
		return false, nil
	}

	var tags model.Tags
	err := json.Unmarshal([]byte(masterJsonTags), &tags)
	if err != nil {
		return false, errors.Wrapf(err, "%v: failed to unmarshal tags", masterPubkey)
	}

	return onBehalfIsAllowed(tags, onBehalfPubkey, kind, nowUnix), nil
}

func sqlEventTagReorderJSON(jsonTag string) (string, error) {
	var tag model.Tag

	if jsonTag == "" {
		return "[]", nil
	}

	if err := json.Unmarshal([]byte(jsonTag), &tag); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal tags")
	}

	data, err := json.Marshal(eventTagsReorder(tag))

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

	return attestationUpdateIsAllowed(oldTags, newTags), nil
}
