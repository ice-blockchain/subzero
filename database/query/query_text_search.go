// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

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
