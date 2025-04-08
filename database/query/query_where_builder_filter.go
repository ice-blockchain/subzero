// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"
	"strings"

	"github.com/ice-blockchain/subzero/model"
)

func parseNostrFilterFlags(f *databaseFilterSearch) *databaseFilterSearch {
	flags := []struct {
		Name string
		Flag **bool
	}{
		{"expiration", &f.Expiration},
		{"videos", &f.Videos},
		{"images", &f.Images},
		{"quotes", &f.Quotes},
		{"references", &f.References},
	}

	for idx := range flags {
		flagStart := strings.Index(strings.ToLower(f.Search), flags[idx].Name+":")
		if flagStart == -1 {
			continue
		}

		flagEnd := strings.Index(f.Search[flagStart:], " ")
		if flagEnd == -1 {
			flagEnd = len(f.Search)
		} else {
			flagEnd += flagStart
		}

		value := strings.ToLower(f.Search[flagStart+len(flags[idx].Name)+1 : flagEnd])
		if value == "true" || value == "1" || value == "on" || value == "yes" {
			on := true
			*flags[idx].Flag = &on
		} else if value == "false" || value == "0" || value == "off" || value == "no" {
			off := false
			*flags[idx].Flag = &off
		} else {
			// Do not now how to parse the value.
			continue
		}

		// Remove flag:value from the search string.
		f.Search = strings.TrimSpace(f.Search[:flagStart] + f.Search[flagEnd:])
	}

	return f
}

func parseNostrFilterTagMarkers(f *databaseFilterSearch) *databaseFilterSearch {
	// Parse tag markers.
	// - <tag>marker:<value>
	// - !<tag>marker:<value>
	const tagMarker = `marker:`
	var tagMarkerOffset int

	for strings.Contains(f.Search[tagMarkerOffset:], tagMarker) {
		tagMarkerStart := strings.Index(f.Search[tagMarkerOffset:], tagMarker)
		if tagMarkerStart < 1 {
			// Should be on the 1st position, not zero.
			break
		}
		tagMarkerEnd := strings.Index(f.Search[tagMarkerOffset+tagMarkerStart+len(tagMarker):], " ")
		if tagMarkerEnd == -1 {
			tagMarkerEnd = len(f.Search[tagMarkerOffset:])
		} else if tagMarkerEnd > 0 {
			tagMarkerEnd += tagMarkerStart + len(tagMarker)
		}
		if x := strings.TrimSpace(f.Search[tagMarkerOffset+tagMarkerStart-1 : tagMarkerOffset+tagMarkerStart]); x != "" {
			var excludeOffset int
			if (tagMarkerOffset+tagMarkerStart-1 > 0) && (f.Search[tagMarkerOffset+tagMarkerStart-2] == '!') {
				excludeOffset = 1
			}
			f.TagMarkers = append(f.TagMarkers, databaseFilterMarker{
				Tag:     x,
				Marker:  f.Search[tagMarkerOffset+tagMarkerStart+len(tagMarker) : tagMarkerOffset+tagMarkerEnd],
				Exclude: excludeOffset > 0,
			})
			if tagMarkerEnd < len(f.Search) {
				tagMarkerEnd++
			}
			f.Search = strings.TrimSpace(f.Search[:tagMarkerStart-1-excludeOffset] + f.Search[tagMarkerEnd:])
		} else {
			tagMarkerOffset += tagMarkerStart + len(tagMarker)
		}
	}
	return f
}

func parseNostrFilterDependencies(f *databaseFilterSearch) (*databaseFilterSearch, error) {
	const dependenciesPrefix = "include:dependencies:"
	for strings.Contains(f.Search, dependenciesPrefix) {
		depStrStart := strings.Index(f.Search, dependenciesPrefix)
		if depStrStart == -1 {
			break
		}
		depStrEnd := strings.Index(f.Search[depStrStart:], " ")
		if depStrEnd == -1 {
			depStrEnd = len(f.Search)
		} else {
			depStrEnd += depStrStart
		}
		dep, err := parseDepRequest(f.Search[depStrStart+len(dependenciesPrefix) : depStrEnd])
		if err != nil {
			return nil, err
		}
		f.Dependencies = append(f.Dependencies, dep)
		f.Search = strings.TrimSpace(f.Search[:depStrStart] + f.Search[depStrEnd:])
	}
	return f, nil
}

func parseNostrFilterText(f *databaseFilterSearch) *databaseFilterSearch {
	quoteStart := strings.Index(f.Search, "\"")
	quoteEnd := strings.LastIndex(f.Search, "\"")

	if quoteStart != -1 && quoteEnd != -1 && quoteEnd > quoteStart {
		s, err := strconv.Unquote(f.Search[quoteStart+1 : quoteEnd])
		if err != nil {
			s = f.Search[quoteStart+1 : quoteEnd]
		}
		f.SearchText = strings.TrimSpace(s)
		f.Search = strings.TrimSpace(f.Search[:quoteStart] + f.Search[quoteEnd+1:])
	}

	return f
}

func parseRank(f *databaseFilterSearch) *databaseFilterSearch {
	f.Rank = rankUndef
	for _, r := range []string{"top", "trending"} {
		start := strings.Index(f.Search, r)
		if start == -1 {
			continue
		}
		switch r {
		case "top":
			f.Rank = rankTOP
		case "trending":
			f.Rank = rankTrending
		}
		f.Search = strings.TrimSpace(f.Search[:start] + f.Search[start+len(r):])
	}
	return f
}

func parseNostrFilter(filter model.Filter) (*databaseFilterSearch, error) {
	f := parseNostrFilterText(&databaseFilterSearch{
		Filter: filter,
	})

	f = parseNostrFilterFlags(f)
	f = parseNostrFilterTagMarkers(f)
	f, err := parseNostrFilterDependencies(f)
	if err != nil {
		return nil, err
	}
	f = parseRank(f)

	f.Search = strings.TrimSpace(f.Search)

	return f, nil
}
