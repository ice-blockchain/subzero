// SPDX-License-Identifier: ice License 1.0

package query

import (
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
			f.TagMarkers = append(f.TagMarkers, databaseFilterMarker{
				Tag:    x,
				Marker: f.Search[tagMarkerOffset+tagMarkerStart+len(tagMarker) : tagMarkerOffset+tagMarkerEnd],
			})
			if tagMarkerEnd < len(f.Search) {
				tagMarkerEnd++
			}
			f.Search = strings.TrimSpace(f.Search[:tagMarkerStart-1] + f.Search[tagMarkerEnd:])
		} else {
			tagMarkerOffset += tagMarkerStart + len(tagMarker)
		}
	}
	return f
}

func parseNostrFilterDependencies(f *databaseFilterSearch) (*databaseFilterSearch, error) {
	const dependenciesPrefix = "include:dependencies:"
	if depStrStart := strings.Index(f.Search, dependenciesPrefix); depStrStart != -1 {
		depStrEnd := strings.Index(f.Search[depStrStart:], " ")
		if depStrEnd == -1 {
			depStrEnd = len(f.Search)
		}
		dep, err := parseDepRequest(f.Search[depStrStart+len(dependenciesPrefix) : depStrEnd])
		if err != nil {
			return nil, err
		}
		f.Dependencies = dep
		f.Search = f.Search[:depStrStart] + f.Search[depStrEnd:]
	}
	return f, nil
}

func parseNostrFilter(filter model.Filter) (*databaseFilterSearch, error) {
	f := parseNostrFilterFlags(&databaseFilterSearch{
		Filter: filter,
	})
	f = parseNostrFilterTagMarkers(f)
	f, err := parseNostrFilterDependencies(f)
	if err != nil {
		return nil, err
	}

	f.Search = strings.TrimSpace(f.Search)

	return f, nil
}
