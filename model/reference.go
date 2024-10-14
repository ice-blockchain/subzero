// SPDX-License-Identifier: ice License 1.0

package model

import (
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

func ParseEventReference(tags Tags) (refs []EventReference, err error) {
	plainEvents := make([]string, 0, len(tags))
	for _, tag := range tags {
		switch tag.Key() {
		case "e":
			if v := tag.Value(); v != "" {
				plainEvents = append(plainEvents, v)
			}
		case "a":
			vals := strings.Split(tag.Value(), ":")
			if len(vals) != 3 {
				return nil, errors.Errorf("failed to parse replaceable event reference, len != 3: %v", tag.Value())
			}

			kind, err := strconv.ParseInt(vals[0], 10, 64)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse replaceable event kind %v", tag.Value())
			}

			refs = append(refs, &ReplaceableEventReference{
				Kind:   int(kind),
				PubKey: vals[1],
				DTag:   vals[2],
			})
		}
	}
	if len(plainEvents) > 0 {
		refs = append(refs, &PlainEventReference{EventIDs: plainEvents})
	}

	return refs, nil
}

func (e *PlainEventReference) Filter() (f Filter) {
	f.IDs = e.EventIDs

	return f
}

func (e *ReplaceableEventReference) Filter() (f Filter) {
	f.Kinds = []int{e.Kind}
	f.Authors = []string{e.PubKey}

	if e.DTag != "" {
		f.Tags = TagMap{"d": {e.DTag}}
	}

	return f
}
