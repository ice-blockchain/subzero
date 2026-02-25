// SPDX-License-Identifier: ice License 1.0

package model

import (
	"encoding/json"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	quill "github.com/dchenk/go-render-quill"
	"github.com/microcosm-cc/bluemonday"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/rs/zerolog/log"
)

type (
	deltaOperation struct {
		Attributes map[string]json.RawMessage `json:"attributes,omitempty"`
		Insert     json.RawMessage            `json:"insert"`
	}
	deltaInsertObject struct {
		TextEditorProfile     string `json:"text-editor-profile,omitempty"`
		TextEditorCode        string `json:"text-editor-code,omitempty"`
		TextEditorSeparator   string `json:"text-editor-separator,omitempty"`
		TextEditorSingleImage string `json:"text-editor-single-image,omitempty"`
	}
)

var (
	nprofileRegex = regexp.MustCompile(`(?:nostr:|ion:)?nprofile1[a-z0-9]+`)
)

func ExtractRichTextContent(ev *Event) string {
	richTextTag := ev.GetTag(CustomIONTagRichText)
	if len(richTextTag) < 3 {
		return ""
	}
	protocol := richTextTag[1]
	if protocol != QuillDeltaProtocol {
		return ""
	}

	return parseQuillDeltaToPlainText(richTextTag[2])
}

func ExtractMentionedPubkeys(e *Event) ([]string, error) {
	if e.Event.Content != "" {
		return extractPubkeysFromContent(e.Content), nil
	}
	richTextPubkeys, err := extractPubkeysFromRichText(e)
	if err != nil {
		return nil, err
	}

	return richTextPubkeys, nil
}

func parseQuillDeltaToPlainText(deltaJSON string) string {
	var delta []deltaOperation
	if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
		log.Error().Str("context", "MODEL").Err(err).Msg("error unmarshalling Quill delta")

		return ""
	}
	customElementsText := extractUsefulCustomElementsContentFromParsed(delta)
	cleanedDelta := removeCustomElementsFromParsedDelta(delta)
	cleanedDelta = ensureDeltaEndsWithNewline(cleanedDelta)
	cleanedJSON, err := json.Marshal(cleanedDelta)
	if err != nil {
		log.Error().Str("context", "MODEL").Err(err).Msg("error marshalling cleaned delta")

		return ""
	}
	htmlBytes, err := quill.Render(cleanedJSON)
	if err != nil {
		log.Error().Str("context", "MODEL").Err(err).Msg("error rendering Quill delta")

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

func extractUsefulCustomElementsContentFromParsed(delta []deltaOperation) string {
	var extractedTexts []string
	for _, op := range delta {
		var insertObj deltaInsertObject
		if err := json.Unmarshal(op.Insert, &insertObj); err == nil {
			if text := extractUsefulContentFromCustomElement(&insertObj); text != "" {
				extractedTexts = append(extractedTexts, text)
			}
		}
	}

	return strings.Join(extractedTexts, " ")
}

func extractUsefulContentFromCustomElement(element *deltaInsertObject) string {
	if element.TextEditorCode != "" {
		return element.TextEditorCode
	}
	if element.TextEditorProfile != "" {
		return element.TextEditorProfile
	}

	return ""
}

func removeCustomElementsFromParsedDelta(delta []deltaOperation) []deltaOperation {
	var cleanedDelta []deltaOperation
	for _, op := range delta {
		var insertObj deltaInsertObject
		if err := json.Unmarshal(op.Insert, &insertObj); err == nil && isAnyCustomTextEditorElement(&insertObj) {
			continue
		}
		cleanedDelta = append(cleanedDelta, op)
	}
	if cleanedDelta == nil {
		cleanedDelta = []deltaOperation{}
	}

	return cleanedDelta
}

func ensureDeltaEndsWithNewline(delta []deltaOperation) []deltaOperation {
	if len(delta) == 0 {
		return delta
	}
	lastOp := delta[len(delta)-1]
	var insertStr string
	if err := json.Unmarshal(lastOp.Insert, &insertStr); err == nil {
		if !strings.HasSuffix(insertStr, "\n") {
			newInsertStr := insertStr + "\n"
			newInsertJSON, _ := json.Marshal(newInsertStr)
			delta[len(delta)-1].Insert = newInsertJSON
		}
	}

	return delta
}

func isAnyCustomTextEditorElement(element *deltaInsertObject) bool {
	return element.TextEditorSeparator != "" ||
		element.TextEditorSingleImage != "" ||
		element.TextEditorCode != "" ||
		element.TextEditorProfile != ""
}

func htmlToPlainText(htmlStr string) string {
	policy := bluemonday.StrictPolicy()
	policy.AddSpaceWhenStrippingTag(true)
	text := html.UnescapeString(policy.Sanitize(htmlStr))

	text = strings.ReplaceAll(text, "\n", " ")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

func extractPubkeysFromContent(content string) []string {
	var pubkeys []string
	matches := nprofileRegex.FindAllString(content, -1)
	for _, match := range matches {
		if pubkey := decodePubkeyFromNprofile(match); pubkey != "" {
			pubkeys = append(pubkeys, pubkey)
		}
	}

	return pubkeys
}

func extractPubkeysFromRichText(e *Event) ([]string, error) {
	var pubkeys []string
	richTextTag := e.GetTag(CustomIONTagRichText)
	if len(richTextTag) < 3 || richTextTag.Value() != QuillDeltaProtocol {
		return pubkeys, nil
	}
	var delta []deltaOperation
	if err := json.Unmarshal([]byte(richTextTag[2]), &delta); err != nil {
		return pubkeys, nil
	}
	for _, op := range delta {
		if op.Attributes != nil {
			if mentionData, exists := op.Attributes["mention"]; exists {
				var mentionStr string
				if err := json.Unmarshal(mentionData, &mentionStr); err == nil {
					if pubkey := decodePubkeyFromNprofile(mentionStr); pubkey != "" {
						pubkeys = append(pubkeys, pubkey)
					}
				}
			}
		}
	}

	return pubkeys, nil
}

func decodePubkeyFromNprofile(nprofileMatch string) string {
	nprofileStr, ok := strings.CutPrefix(nprofileMatch, "nostr:")
	if !ok {
		nprofileStr = strings.TrimPrefix(nprofileMatch, "ion:")
	}
	prefix, data, err := nip19.Decode(nprofileStr)
	if err != nil || prefix != "nprofile" {
		return ""
	}
	profile, ok := data.(nostr.ProfilePointer)
	if !ok {
		return ""
	}

	return profile.PublicKey
}

func ReplacePMO(ev *Event, replace func(orig, replace string) (bool, string)) (string, error) {
	content := ev.Content
	pmos := ev.Tags.GetAll([]string{"pmo"})
	if len(pmos) == 0 {
		return content, nil
	}

	type override struct {
		text  string
		start int
		end   int
	}

	overrides := make([]override, 0, len(pmos))
	for _, pmo := range pmos {
		if len(pmo) < 3 {
			return "", errors.Errorf("invalid PMO tag, less than 3 elements: %v", len(pmo))
		}

		parts := strings.Split(pmo.Value(), ":")
		if len(parts) != 2 {
			return "", errors.Errorf("invalid PMO tag, not start:end separated %v", pmo.Value())
		}

		var start, end int
		var err error
		if start, err = strconv.Atoi(parts[0]); err != nil {
			return "", errors.Wrapf(err, "invalid PMO tag, start index not a number: %v", parts[0])
		}
		if end, err = strconv.Atoi(parts[1]); err != nil {
			return "", errors.Wrapf(err, "invalid PMO tag, end index not a number: %v", parts[1])
		}
		if start < 0 || end < start || start > len(content) || end > len(content) {
			return "", errors.Errorf("invalid PMO tag, invalid indexes %v %v (len %v)", parts[0], parts[1], len(content))
		}
		overrides = append(overrides, override{start: start, end: end, text: pmo[2]})
	}
	// start from the end
	sort.Slice(overrides, func(i, j int) bool {
		return overrides[i].start > overrides[j].start
	})

	result := content
	for _, o := range overrides {
		ok, newReplacement := replace(result[o.start:o.end], o.text)
		if !ok {
			continue
		}
		if newReplacement != "" {
			o.text = newReplacement
		}
		result = result[:o.start] + o.text + result[o.end:]
	}

	return result, nil
}
