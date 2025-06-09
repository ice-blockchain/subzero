// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindRelayListMetadataEvent(e *model.Event) error {
	rTags := e.Tags.GetAll([]string{"r"})
	if len(rTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-65, no required r tags: %+v", e)
	}
	if e.Content != "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-65, content is not used: %+v", e)
	}
	for _, tag := range rTags {
		if len(tag) < 2 || (len(tag) > 2 && (tag[2] != "" && tag[2] != model.RelayListReadMarker && tag[2] != model.RelayListWriteMarker)) {
			return errors.Wrapf(ErrWrongEventParams, "nip-65, wrong read/write marker for r tag: %+v", e)
		}
	}

	return nil
}
