// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"github.com/ice-blockchain/subzero/model"
)

// parseEvent deconstructs the event into a set of index keys that represent all the possible filter dimensions it matches against.
func parseEvent(ev *model.Event, buf []indexKey) []indexKey {
	// Generic Catch-All: {}.
	buf = append(buf, indexKey{Kind: anyKind, Dimension: dimNone})

	// Kind Catch-All: {"kinds": [x]}.
	buf = append(buf, indexKey{Kind: ev.Kind, Dimension: dimNone})

	// Author Catch-All: {"authors": [x]}.
	buf = append(buf, indexKey{Kind: anyKind, Dimension: dimAuthor, Value: ev.PubKey})
	buf = append(buf, indexKey{Kind: ev.Kind, Dimension: dimAuthor, Value: ev.PubKey})
	if m := ev.GetMasterPublicKey(); m != "" && m != ev.PubKey {
		buf = append(buf, indexKey{Kind: anyKind, Dimension: dimAuthor, Value: m})
		buf = append(buf, indexKey{Kind: ev.Kind, Dimension: dimAuthor, Value: m})
	}

	// Tags.
	for i := range ev.Tags {
		if len(ev.Tags[i]) < 2 {
			continue // Skip empty or malformed tags.
		}

		var dim dimension
		var val string

		switch ev.Tags[i][0] {
		case "p":
			dim = dimTagP
			val = ev.Tags[i][1]

		case "k":
			dim = dimTagK
			val = ev.Tags[i][1]

		case model.CustomIONTagAddressableQ:
			if len(ev.Tags[i]) > 3 {
				val = ev.Tags[i][3]
				dim = dimTagQ
			}
		}

		// If we matched a tracked dimension, emit both the specific-kind key
		// and the any-kind key.
		if dim != dimNone && val != "" {
			buf = append(buf, indexKey{Kind: anyKind, Dimension: dim, Value: val})
			buf = append(buf, indexKey{Kind: ev.Kind, Dimension: dim, Value: val})
		}
	}

	return buf
}
