package model

import (
	"strconv"
	"strings"

	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
)

func ParseEventReference(tags nostr.Tags) ([]EventReference, error) {
	plainEvents := make([]string, 0, len(tags))
	refs := []EventReference{}
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == "e" {
			plainEvents = append(plainEvents, tag.Value())
		} else if len(tag) >= 2 && tag[0] == "a" {
			val := strings.Split(tag.Value(), ":")
			if len(val) != 3 {
				return nil, errorx.Errorf("failed to parse replaceable event reference, len != 3: %v", val)
			}
			kind, err := strconv.ParseInt(val[0], 10, 64)
			if err != nil {
				return nil, errorx.Withf(err, "failed to parse replaceable event reference %v", val)
			}
			refs = append(refs, &ReplaceableEventReference{
				Kind:   int(kind),
				PubKey: val[1],
				DTag:   val[2],
			})
		}
	}
	if len(plainEvents) > 0 {
		refs = append(refs, &PlainEventReference{EventIDs: plainEvents})
	}

	return refs, nil
}

func (e *PlainEventReference) Filter() nostr.Filter {
	return nostr.Filter{
		IDs: e.EventIDs,
	}
}

func (e *ReplaceableEventReference) Filter() nostr.Filter {
	f := nostr.Filter{
		Kinds:   []int{e.Kind},
		Authors: []string{e.PubKey},
	}
	if e.DTag != "" {
		f.Tags = nostr.TagMap{"d": {e.DTag}}
	}

	return f
}
