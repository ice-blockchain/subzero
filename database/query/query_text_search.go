// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func extractIMetaTagValues(ev *model.Event) (data []string) {
	valuesToExtract := map[string]struct{}{
		"alt":     {},
		"summary": {},
	}
	for _, tag := range ev.Tags {
		if tag.Key() != "imeta" {
			continue
		}
		for _, val := range tag {
			fields := strings.Fields(val)
			if len(fields) == 0 {
				continue
			} else if _, ok := valuesToExtract[fields[0]]; !ok {
				continue
			}

			if len(fields) > 1 {
				data = append(data, fields[1:]...)
			}
		}
	}
	return data
}

func extractProfileContentMetadata(contentJSON string) []string {
	var content struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}

	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return []string{}
	}

	return []string{content.Name, content.DisplayName}
}

func prepareSearchContent(ev *model.Event) string {
	var fields []string

	switch ev.Kind {
	case nostr.KindProfileMetadata:
		fields = append(fields, extractProfileContentMetadata(ev.Content)...)

	case nostr.KindTextNote, nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		if ev.Content != "" {
			fields = append(fields, ev.Content)
		} else if richTextContent := model.ExtractRichTextContent(ev); richTextContent != "" {
			fields = append(fields, richTextContent)
		}
	}

	return strings.Join(fields, " ")
}
