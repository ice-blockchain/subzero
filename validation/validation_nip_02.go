// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func validateFollowListEvent(e *model.Event) error {
	keys := make(map[string]struct{})
	master := e.GetMasterPublicKey()
	for _, tag := range e.GetTags("p") {
		v := tag.Value()
		switch v {
		case "":
			return errors.Wrap(ErrWrongEventParams, "nip-02: missing public key")
		case master, e.PubKey:
			return errors.Wrapf(ErrWrongEventParams, "tag %q: cannot have the same value as public key or %q", "p", model.CustomIONTagOnBehalfOf)
		}
		keys[v] = struct{}{}
	}

	return nil
}
