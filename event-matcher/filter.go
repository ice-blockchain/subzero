// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

var (
	repostKinds = map[int]struct{}{
		model.CustomIONKindRepostOfArticle:                      {},
		model.CustomIONKindRepostOfEditableTextNote:             {},
		model.CustomIONKindRepostOfTokenizedCommunityDefinition: {},
		model.CustomIONKindRepostOfTokenizedCommunityAction:     {},
	}
)

func hasSpecialKinds(kinds []int) bool {
	for _, kind := range kinds {
		if _, isRepost := repostKinds[kind]; isRepost {
			return true
		}
		if kind < 0 {
			return true
		}
	}
	return false
}

func parseFilters(filters model.Filters) []indexKey {
	if len(filters) == 0 {
		return []indexKey{{Kind: anyKind, Dimension: dimNone}}
	}

	uniqueKeys := make(map[indexKey]struct{}, len(filters))
	for i := range filters {
		var targets []indexKey

		for _, author := range filters[i].Authors {
			targets = append(targets, indexKey{Dimension: dimAuthor, Value: author})
		}

		for k, v := range filters[i].Tags {
			for j := range v {
				var dim dimension
				var val string

				switch k {
				case "p", "k":
					if len(v[j]) >= 1 && v[j][0] != nil && *v[j][0] != "" {
						val = *v[j][0]
						if k == "p" {
							dim = dimTagP
						} else {
							dim = dimTagK
						}
					}
				case model.CustomIONTagAddressableQ, "q":
					if len(v[j]) == 3 && v[j][2] != nil && *v[j][2] != "" {
						dim = dimTagUpperQ
						if k == "q" {
							dim = dimTagLowerQ
						}
						val = *v[j][2]
					}
				}

				if dim != dimNone && val != "" {
					targets = append(targets, indexKey{Dimension: dim, Value: val})
				}
			}
		}

		kinds := filters[i].Kinds
		if hasSpecialKinds(filters[i].Kinds) {
			kinds = make([]int, 0, len(filters[i].Kinds))

			var hadRepost bool
			for _, kind := range filters[i].Kinds {
				if kind < 0 {
					// If we had any negative kinds, we need to add the any-kind key to the targets to ensure they get matched against events of any kind.
					uniqueKeys[indexKey{Kind: anyKind, Dimension: dimNone}] = struct{}{}
					continue
				}

				if _, isRepost := repostKinds[kind]; isRepost {
					if hadRepost {
						continue
					}

					hadRepost = true
					kind = nostr.KindGenericRepost
				}

				kinds = append(kinds, kind)
			}
		}

		numKinds := max(1, len(kinds))
		numTargets := max(1, len(targets))
		combinations := numKinds * numTargets

		if combinations <= indexCartesianLimit {
			if len(kinds) == 0 && len(targets) == 0 {
				uniqueKeys[indexKey{Kind: anyKind, Dimension: dimNone}] = struct{}{}
			} else if len(targets) == 0 {
				for _, kind := range kinds {
					uniqueKeys[indexKey{Kind: kind, Dimension: dimNone}] = struct{}{}
				}
			} else if len(kinds) == 0 {
				for _, t := range targets {
					t.Kind = anyKind
					uniqueKeys[t] = struct{}{}
				}
			} else {
				for _, kind := range kinds {
					for _, t := range targets {
						t.Kind = kind
						uniqueKeys[t] = struct{}{}
					}
				}
			}
		} else {
			if len(targets) > 0 {
				for _, t := range targets {
					t.Kind = anyKind
					uniqueKeys[t] = struct{}{}
				}
			} else {
				for _, kind := range kinds {
					uniqueKeys[indexKey{Kind: kind, Dimension: dimNone}] = struct{}{}
				}
			}
		}
	}

	keys := make([]indexKey, 0, len(uniqueKeys))
	for k := range uniqueKeys {
		keys = append(keys, k)
	}

	return keys
}
