// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"html"
	"log"
	"regexp"
	"strings"

	quill "github.com/dchenk/go-render-quill"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func extractRichTextContent(ev *model.Event) string {
	richTextTag := ev.GetTag(model.CustomIONTagRichText)
	if richTextTag == nil || len(richTextTag) < 3 {
		return ""
	}
	protocol := richTextTag[1]
	if protocol != "quill_delta" {
		return ""
	}

	return parseQuillDeltaToPlainText(richTextTag[2])
}

func parseQuillDeltaToPlainText(deltaJSON string) string {
	var delta []map[string]interface{}
	if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
		log.Printf("Error unmarshalling Quill delta: %v", err)

		return ""
	}
	customElementsText := extractUsefulCustomElementsContentFromParsed(delta)
	cleanedDelta := removeCustomElementsFromParsedDelta(delta)
	cleanedDelta = ensureDeltaEndsWithNewline(cleanedDelta)
	cleanedJSON, err := json.Marshal(cleanedDelta)
	if err != nil {
		log.Printf("Error marshalling cleaned delta: %v", err)

		return ""
	}
	htmlBytes, err := quill.Render(cleanedJSON)
	if err != nil {
		log.Printf("Error rendering Quill delta: %v", err)

		return ""
	}
	plainText := htmlToPlainText(string(htmlBytes))
	var combinedText []string
	if plainText != "" {
		combinedText = append(combinedText, plainText)
	}
	if customElementsText != "" {
		combinedText = append(combinedText, customElementsText)
	}

	return strings.TrimSpace(strings.Join(combinedText, " "))
}

func extractUsefulCustomElementsContentFromParsed(delta []map[string]interface{}) string {
	var extractedTexts []string
	for _, op := range delta {
		insert, ok := op["insert"]
		if !ok {
			continue
		}
		if insertObj, isObj := insert.(map[string]interface{}); isObj {
			if text := extractUsefulContentFromCustomElement(insertObj); text != "" {
				extractedTexts = append(extractedTexts, text)
			}
		}
	}

	return strings.Join(extractedTexts, " ")
}

func extractUsefulContentFromCustomElement(element map[string]interface{}) string {
	if codeContent, ok := element["text-editor-code"].(string); ok {
		return codeContent
	}
	if profileContent, ok := element["text-editor-profile"].(string); ok {
		return profileContent
	}

	return ""
}

func removeCustomElementsFromParsedDelta(delta []map[string]interface{}) []map[string]interface{} {
	var cleanedDelta []map[string]interface{}
	for _, op := range delta {
		insert, ok := op["insert"]
		if !ok {
			cleanedDelta = append(cleanedDelta, op)
			continue
		}
		if insertObj, isObj := insert.(map[string]interface{}); isObj && isAnyCustomTextEditorElement(insertObj) {
			continue
		}
		cleanedDelta = append(cleanedDelta, op)
	}
	if cleanedDelta == nil {
		cleanedDelta = []map[string]interface{}{}
	}

	return cleanedDelta
}

func ensureDeltaEndsWithNewline(delta []map[string]interface{}) []map[string]interface{} {
	if len(delta) == 0 {
		return delta
	}
	lastOp := delta[len(delta)-1]
	if insert, ok := lastOp["insert"]; ok {
		if insertStr, isStr := insert.(string); isStr {
			if !strings.HasSuffix(insertStr, "\n") {
				newLastOp := make(map[string]interface{})
				for k, v := range lastOp {
					newLastOp[k] = v
				}
				newLastOp["insert"] = insertStr + "\n"
				delta[len(delta)-1] = newLastOp
			}
		}
	}

	return delta
}

func isAnyCustomTextEditorElement(element map[string]interface{}) bool {
	customKeys := []string{
		"text-editor-separator",
		"text-editor-single-image",
		"text-editor-code",
		"text-editor-profile",
	}
	for _, key := range customKeys {
		if _, exists := element[key]; exists {
			return true
		}
	}

	return false
}

func htmlToPlainText(htmlStr string) string {
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	text := htmlTagRegex.ReplaceAllString(htmlStr, " ")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

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
		} else if richTextContent := extractRichTextContent(ev); richTextContent != "" {
			fields = append(fields, richTextContent)
		}
	case nostr.KindFileMetadata:
		fields = append(fields, extractTagValues(ev)...)
	}

	return strings.Join(fields, " ")
}
