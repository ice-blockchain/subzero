// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"strings"

	"github.com/forPelevin/gomoji"
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
	if strings.EqualFold(content.Name, content.DisplayName) {
		return []string{strings.ToLower(content.Name)}
	}

	return []string{strings.ToLower(content.Name), strings.ToLower(content.DisplayName)}
}

func prepareSearchContent(ev *model.Event) string {
	var fields []string

	switch ev.Kind {
	case nostr.KindProfileMetadata:
		fields = append(fields, extractProfileContentMetadata(ev.Content)...)

	case nostr.KindTextNote, nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		var data string
		if ev.Content != "" {
			data = ev.Content
		} else if richTextContent := model.ExtractRichTextContent(ev); richTextContent != "" {
			data = richTextContent
		}
		data = strings.TrimSpace(gomoji.RemoveEmojis(data))
		fields = append(fields, data)
	}

	return strings.Join(fields, " ")
}
