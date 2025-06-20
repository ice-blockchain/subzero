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

func extractTagValues(ev *model.Event) (data []string) {
	hasImeta := false
	for _, tag := range ev.Tags {
		switch tag.Key() {
		case "alt", "summary":
			data = append(data, strings.TrimSpace(tag.Value()))
		case "imeta":
			hasImeta = true
		}
	}

	if hasImeta {
		data = append(data, extractIMetaTagValues(ev)...)
	}

	return data
}

func prepareSearchContent(ev *model.Event) string {
	var fields []string

	switch ev.Kind {
	case nostr.KindProfileMetadata:
		fields = append(fields, extractProfileContentMetadata(ev.Content)...)
		fields = append(fields, extractTagValues(ev)...)

	case nostr.KindTextNote, nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		fields = append(fields, extractTagValues(ev)...)
		if ev.Content != "" {
			fields = append(fields, ev.Content)
		} else if richTextContent := model.ExtractRichTextContent(ev); richTextContent != "" {
			fields = append(fields, richTextContent)
		}
	case nostr.KindFileMetadata:
		fields = append(fields, extractTagValues(ev)...)
	}

	return strings.Join(fields, " ")
}
