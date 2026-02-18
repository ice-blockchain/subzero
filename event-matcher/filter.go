// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"github.com/ice-blockchain/subzero/model"
)

type (
	// parsedFilter represents a parsed filter with extracted tag values.
	parsedFilter struct {
		*model.Filter                            // Embedded filter.
		MasterKeysByKind map[model.Kind][]string // Extracted p/Q tag values per kind.
		MasterKeys       []string                // All p/Q tag values from the filter.
	}

	// parsedFilters is a slice of parsedFilter, representing multiple filters.
	parsedFilters []parsedFilter
)

func parseFilters(filters model.Filters) (parsedFilters parsedFilters) {
	for i := range filters {
		parsed := parsedFilter{
			Filter:           &filters[i],
			MasterKeysByKind: make(map[model.Kind][]string),
		}

	tagsLoop:
		for k, v := range filters[i].Tags {
			if len(v) != 1 {
				// Expected formats:
				// "p": [[currentUserMasterPubkey]].
				// "p": [[currentUserMasterPubkey, "", currentDevicePubkey]].
				// "Q": [[null, null, currentUserMasterPubkey]].
				continue
			}

			var targetKey string
			switch k {
			case "p":
				if len(v[0]) < 1 || v[0][0] == nil || *v[0][0] == "" {
					continue tagsLoop
				}
				targetKey = k + *v[0][0] // Prefix to avoid collision with Q tags.

			case model.CustomIONTagAddressableQ:
				if len(v[0]) != 3 || v[0][0] != nil || v[0][1] != nil || v[0][2] == nil || *v[0][2] == "" {
					continue tagsLoop
				}
				targetKey = k + *v[0][2]

			default:
				continue tagsLoop
			}

			if len(filters[i].Kinds) > 0 {
				for _, kind := range filters[i].Kinds {
					parsed.MasterKeysByKind[kind] = append(parsed.MasterKeysByKind[kind], targetKey)
				}
			}
			parsed.MasterKeys = append(parsed.MasterKeys, targetKey)
		}
		parsedFilters = append(parsedFilters, parsed)
	}
	return parsedFilters
}

// Meta extracts all unique master keys, authors and kinds from the parsed filters for removal purposes.
func (p parsedFilters) Meta() (keys []string, kinds []model.Kind) {
	for i := range p {
		if len(p[i].Kinds) > 0 {
			kinds = append(kinds, p[i].Kinds...)
		}
		if len(p[i].MasterKeys) > 0 {
			keys = append(keys, p[i].MasterKeys...)
		}
		if len(p[i].Authors) > 0 {
			keys = append(keys, p[i].Authors...)
		}
	}

	return model.DeduplicateStringSlice(keys), model.DeduplicateIntSlice(kinds)
}
