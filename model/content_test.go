// SPDX-License-Identifier: ice License 1.0

package model

import (
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestExtractMentionedPubkeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    *Event
		expected []string
	}{
		{
			name: "extract from content with nprofile",
			event: &Event{
				Event: nostr.Event{
					Content: "Mention nostr:nprofile1qqsgy2xak5fc8jrf5e2qydnheup4amwtca4k96c3evkj7t2wy4d7z8q400gfm and nostr:nprofile1qqs86zuljkandqe73upkma2gc3fpme0rwkqtjvhmqz044vdkhyhc4ysysh7dj here",
				},
			},
			expected: []string{
				"8228ddb51383c869a654023677cf035eedcbc76b62eb11cb2d2f2d4e255be11c",
				"7d0b9f95bb36833e8f036df548c4521de5e37580b932fb009f5ab1b6b92f8a92",
			},
		},
		{
			name: "extract from rich_text tag",
			event: &Event{
				Event: nostr.Event{
					Content: "",
					Tags: Tags{
						Tag{CustomIONTagRichText, QuillDeltaProtocol, `[{"insert":"Only "},{"insert":"@user","attributes":{"mention":"nostr:nprofile1qqsgy2xak5fc8jrf5e2qydnheup4amwtca4k96c3evkj7t2wy4d7z8q400gfm"}},{"insert":" can reply"}]`},
					},
				},
			},
			expected: []string{
				"8228ddb51383c869a654023677cf035eedcbc76b62eb11cb2d2f2d4e255be11c",
			},
		},
		{
			name: "no mentions found",
			event: &Event{
				Event: nostr.Event{
					Content: "No mentions here",
				},
			},
			expected: []string{},
		},
		{
			name: "empty content and no rich_text tag",
			event: &Event{
				Event: nostr.Event{
					Content: "",
				},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pubkeys, err := ExtractMentionedPubkeys(tt.event)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.expected, pubkeys)
		})
	}
}

func TestExtractRichTextContent(t *testing.T) {
	t.Parallel()
	t.Run("Valid Quill Delta with text content", func(t *testing.T) {
		deltaJSON := `[{"insert":"Header 1"},{"insert":"\n","attributes":{"header":1}},{"insert":"Regular text "},{"insert":"Bold","attributes":{"bold":true}},{"insert":" "},{"insert":"Italic","attributes":{"italic":true}},{"insert":"\n"}]`
		var ev Event
		ev.Tags = Tags{
			{CustomIONTagRichText, QuillDeltaProtocol, deltaJSON},
		}
		result := ExtractRichTextContent(&ev)
		require.Equal(t, "Header 1 Regular text Bold Italic", result)
	})
	t.Run("Valid Quill Delta with simple text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Hello World!\n"}]`
		var ev Event
		ev.Tags = Tags{
			{CustomIONTagRichText, QuillDeltaProtocol, deltaJSON},
		}
		result := ExtractRichTextContent(&ev)
		require.Equal(t, "Hello World!", result)
	})
	t.Run("Valid Quill Delta with embeds", func(t *testing.T) {
		deltaJSON := `[{"insert":"Text before image "},{"insert":{"image":"https://example.com/img.jpg"}},{"insert":" text after image\n"}]`
		var ev Event
		ev.Tags = Tags{
			{CustomIONTagRichText, QuillDeltaProtocol, deltaJSON},
		}
		result := ExtractRichTextContent(&ev)
		require.Equal(t, "Text before image text after image", result)
	})
	t.Run("Invalid JSON", func(t *testing.T) {
		invalidJSON := `[{"insert":"Header 1"}`
		var ev Event
		ev.Tags = Tags{
			{CustomIONTagRichText, QuillDeltaProtocol, invalidJSON},
		}
		result := ExtractRichTextContent(&ev)
		require.Empty(t, result)
	})
	t.Run("Unsupported protocol", func(t *testing.T) {
		var ev Event
		ev.Tags = Tags{
			{CustomIONTagRichText, "unsupported_protocol", "some content"},
		}
		result := ExtractRichTextContent(&ev)
		require.Empty(t, result)
	})
	t.Run("No rich_text tag", func(t *testing.T) {
		var ev Event
		ev.Tags = Tags{}
		result := ExtractRichTextContent(&ev)
		require.Empty(t, result)
	})
	t.Run("Malformed rich_text tag", func(t *testing.T) {
		var ev Event
		ev.Tags = Tags{
			{CustomIONTagRichText, QuillDeltaProtocol},
		}
		result := ExtractRichTextContent(&ev)
		require.Empty(t, result)
	})
}

func TestParseQuillDeltaToPlainText(t *testing.T) {
	t.Parallel()
	t.Run("Simple text operations", func(t *testing.T) {
		deltaJSON := `[{"insert":"Hello "},{"insert":"World","attributes":{"bold":true}},{"insert":"!\n"}]`
		require.Equal(t, "Hello World !", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Text with headers", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Some content\n"}]`
		require.Equal(t, "Main Title Some content", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Text with formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Start "},{"insert":"bold text","attributes":{"bold":true}},{"insert":" and "},{"insert":"italic text","attributes":{"italic":true}},{"insert":" end\n"}]`
		require.Equal(t, "Start bold text and italic text end", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Empty operations", func(t *testing.T) {
		deltaJSON := `[]`
		require.Empty(t, parseQuillDeltaToPlainText(deltaJSON))
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		invalidJSON := `[{"insert":"test"`
		require.Empty(t, parseQuillDeltaToPlainText(invalidJSON))
	})
}

func TestHtmlToPlainText(t *testing.T) {
	t.Parallel()
	t.Run("Simple HTML", func(t *testing.T) {
		html := `<p>Hello <strong>World</strong>!</p>`
		require.Equal(t, "Hello World !", htmlToPlainText(html))
	})
	t.Run("HTML with multiple tags", func(t *testing.T) {
		html := `<h1>Title</h1><p>Paragraph with <em>italic</em> and <strong>bold</strong> text.</p>`
		require.Equal(t, "Title Paragraph with italic and bold text.", htmlToPlainText(html))
	})
	t.Run("HTML with entities", func(t *testing.T) {
		html := `<p>&lt;script&gt;alert(&quot;test&quot;)&lt;/script&gt;</p>`
		require.Equal(t, `<script>alert("test")</script>`, htmlToPlainText(html))
	})
	t.Run("HTML with newlines", func(t *testing.T) {
		html := "<p>Line 1</p>\n<p>Line 2</p>"
		require.Equal(t, "Line 1 Line 2", htmlToPlainText(html))
	})
	t.Run("Empty HTML", func(t *testing.T) {
		html := ``
		require.Empty(t, htmlToPlainText(html))
	})
}

func TestQuillDeltaFormatVariants(t *testing.T) {
	t.Parallel()
	t.Run("Document - Basic text with formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Gandalf","attributes":{"bold":true}},{"insert":" the "},{"insert":"Grey","attributes":{"color":"#cccccc"}},{"insert":"\n"}]`
		require.Equal(t, "Gandalf the Grey", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Document - Complex text with multiple formats", func(t *testing.T) {
		deltaJSON := `[{"insert":"Bold text","attributes":{"bold":true}},{"insert":" and "},{"insert":"italic text","attributes":{"italic":true}},{"insert":" and "},{"insert":"underlined","attributes":{"underline":true}},{"insert":" text.\n"}]`
		require.Equal(t, "Bold text and italic text and underlined text.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Embeds - Image embed", func(t *testing.T) {
		deltaJSON := `[{"insert":{"image":"https://quilljs.com/assets/images/icon.png"},"attributes":{"link":"https://quilljs.com"}}]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Embeds - Text with image embed", func(t *testing.T) {
		deltaJSON := `[{"insert":"Check out this image: "},{"insert":{"image":"https://example.com/image.png"}},{"insert":" Amazing!\n"}]`
		require.Equal(t, "Check out this image: Amazing!", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Embeds - Multiple embed types", func(t *testing.T) {
		deltaJSON := `[{"insert":"Video: unsupported embed"},{"insert":" and formula: "},{"insert":"e=mc^2"},{"insert":" end.\n"}]`
		require.Equal(t, "Video: unsupported embed and formula: e=mc^2 end.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Headers", func(t *testing.T) {
		deltaJSON := `[{"insert":"The Two Towers"},{"insert":"\n","attributes":{"header":1}},{"insert":"Aragorn sped on up the hill.\n"}]`
		require.Equal(t, "The Two Towers Aragorn sped on up the hill.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Multiple header levels", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Subtitle"},{"insert":"\n","attributes":{"header":2}},{"insert":"Sub-subtitle"},{"insert":"\n","attributes":{"header":3}},{"insert":"Regular text\n"}]`
		require.Equal(t, "Main Title Subtitle Sub-subtitle Regular text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Bullet lists", func(t *testing.T) {
		deltaJSON := `[{"insert":"First item"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Second item"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Third item"},{"insert":"\n","attributes":{"list":"bullet"}}]`
		require.Equal(t, "First item Second item Third item", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Ordered lists", func(t *testing.T) {
		deltaJSON := `[{"insert":"First numbered item"},{"insert":"\n","attributes":{"list":"ordered"}},{"insert":"Second numbered item"},{"insert":"\n","attributes":{"list":"ordered"}},{"insert":"Third numbered item"},{"insert":"\n","attributes":{"list":"ordered"}}]`
		require.Equal(t, "First numbered item Second numbered item Third numbered item", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Blockquotes", func(t *testing.T) {
		deltaJSON := `[{"insert":"This is a quote"},{"insert":"\n","attributes":{"blockquote":true}},{"insert":"This is another quote"},{"insert":"\n","attributes":{"blockquote":true}},{"insert":"Regular text\n"}]`
		require.Equal(t, "This is a quote This is another quote Regular text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Code blocks", func(t *testing.T) {
		deltaJSON := `[{"insert":"function hello() {"},{"insert":"\n","attributes":{"code-block":true}},{"insert":"  console.log('Hello');"},{"insert":"\n","attributes":{"code-block":true}},{"insert":"}"},{"insert":"\n","attributes":{"code-block":true}}]`
		require.Equal(t, "function hello() { console.log('Hello'); }", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Text alignment", func(t *testing.T) {
		deltaJSON := `[{"insert":"Left aligned text"},{"insert":"\n"},{"insert":"Center aligned text"},{"insert":"\n","attributes":{"align":"center"}},{"insert":"Right aligned text"},{"insert":"\n","attributes":{"align":"right"}},{"insert":"Justify aligned text"},{"insert":"\n","attributes":{"align":"justify"}}]`
		require.Equal(t, "Left aligned text Center aligned text Right aligned text Justify aligned text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Mixed formatting - Complex document", func(t *testing.T) {
		deltaJSON := `[{"insert":"Document Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"This is "},{"insert":"bold","attributes":{"bold":true}},{"insert":" and "},{"insert":"italic","attributes":{"italic":true}},{"insert":" text.\n"},{"insert":"List item 1"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"List item 2 with "},{"insert":"link","attributes":{"link":"https://example.com"}},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Image: "},{"insert":{"image":"https://example.com/img.jpg"}},{"insert":"\n"},{"insert":"Quote text"},{"insert":"\n","attributes":{"blockquote":true}}]`
		require.Equal(t, "Document Title This is bold and italic text. List item 1 List item 2 with link Image: Quote text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Special characters and entities", func(t *testing.T) {
		deltaJSON := `[{"insert":"Special chars: <>&\"'"},{"insert":"\n","attributes":{"header":2}},{"insert":"Math: α + β = γ"},{"insert":"\n"},{"insert":"Code: "},{"insert":"console.log(\"Hello\");","attributes":{"code":true}},{"insert":"\n"}]`
		require.Equal(t, "Special chars: <>&\"' Math: α + β = γ Code: console.log(\"Hello\");", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Links and formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Visit "},{"insert":"our website","attributes":{"link":"https://example.com","bold":true}},{"insert":" for more info. Also check "},{"insert":"this link","attributes":{"link":"https://other.com","italic":true}},{"insert":".\n"}]`
		require.Equal(t, "Visit our website for more info. Also check this link .", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Color and background formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Red text","attributes":{"color":"#ff0000"}},{"insert":" and "},{"insert":"blue background","attributes":{"background":"#0000ff"}},{"insert":" and "},{"insert":"both","attributes":{"color":"#00ff00","background":"#ffff00"}},{"insert":".\n"}]`
		require.Equal(t, "Red text and blue background and both .", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Font styling", func(t *testing.T) {
		deltaJSON := `[{"insert":"Arial text","attributes":{"font":"arial"}},{"insert":" and "},{"insert":"serif text","attributes":{"font":"serif"}},{"insert":" and "},{"insert":"monospace","attributes":{"font":"monospace"}},{"insert":".\n"}]`
		require.Equal(t, "Arial text and serif text and monospace.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Size formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Small","attributes":{"size":"small"}},{"insert":" "},{"insert":"Large","attributes":{"size":"large"}},{"insert":" "},{"insert":"Huge","attributes":{"size":"huge"}},{"insert":" text.\n"}]`
		require.Equal(t, "Small Large Huge text.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Superscript and subscript", func(t *testing.T) {
		deltaJSON := `[{"insert":"E=mc"},{"insert":"2","attributes":{"script":"super"}},{"insert":" and H"},{"insert":"2","attributes":{"script":"sub"}},{"insert":"O.\n"}]`
		require.Equal(t, "E=mc 2 and H 2 O.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Empty operations", func(t *testing.T) {
		deltaJSON := `[]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Only newlines", func(t *testing.T) {
		deltaJSON := `[{"insert":"\n"},{"insert":"\n"},{"insert":"\n"}]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Whitespace handling", func(t *testing.T) {
		deltaJSON := `[{"insert":"   Multiple   "},{"insert":"   spaces   "},{"insert":"   here   "},{"insert":"\n"}]`
		require.Equal(t, "Multiple spaces here", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Nested formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"This is "},{"insert":"bold and italic","attributes":{"bold":true,"italic":true}},{"insert":" and "},{"insert":"underlined bold","attributes":{"bold":true,"underline":true}},{"insert":" text.\n"}]`
		require.Equal(t, "This is bold and italic and underlined bold text.", parseQuillDeltaToPlainText(deltaJSON))
	})
}

func TestParseQuillDeltaToPlainText_WithCustomElements(t *testing.T) {
	t.Parallel()
	t.Run("Text with custom elements", func(t *testing.T) {
		deltaJSON := `[
			{"insert":"Header"},
			{"insert":"\n","attributes":{"header":1}},
			{"insert":"Text before image "},
			{"insert":{"text-editor-single-image":"img123"}},
			{"insert":" text after image.\n"},
			{"insert":"Separator below:\n"},
			{"insert":{"text-editor-separator":"---"}},
			{"insert":"Code block:\n"},
			{"insert":{"text-editor-code":"console.log('hello world')"}},
			{"insert":"Profile mention: "},
			{"insert":"@alice123","attributes":{"mention":"nostr:npub1alice123"}},
			{"insert":"\n"}
		]`
		require.Equal(t, "Header Text before image text after image. Separator below: Code block: Profile mention: @alice123 console.log('hello world')", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Only custom elements with useful content", func(t *testing.T) {
		deltaJSON := `[
			{"insert":{"text-editor-single-image":"img1"}},
			{"insert":{"text-editor-code":"function test() { return 42; }"}},
			{"insert":"@user789","attributes":{"mention":"nostr:user789"}}
		]`
		require.Equal(t, "@user789 function test() { return 42; }", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Only useless custom elements", func(t *testing.T) {
		deltaJSON := `[
			{"insert":{"text-editor-single-image":"img1"}},
			{"insert":{"text-editor-separator":"---"}}
		]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Invalid JSON", func(t *testing.T) {
		deltaJSON := `[{"insert":{"text-editor-single-image"`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Example from ICIP-7000 spec", func(t *testing.T) {
		deltaJSON := `[
			{"insert":"Header 1"},
			{"insert":"\n","attributes":{"header":1}},
			{"insert":"Header 2"},
			{"insert":"\n","attributes":{"header":2}},
			{"insert":"Header 3"},
			{"insert":"\n","attributes":{"header":3}},
			{"insert":"Regular "},
			{"insert":"Bold","attributes":{"bold":true}},
			{"insert":" "},
			{"insert":"Italic","attributes":{"italic":true}},
			{"insert":" "},
			{"insert":"Underline","attributes":{"underline":true}},
			{"insert":" "},
			{"insert":"Link wrapped","attributes":{"link":"http://ice.io"}},
			{"insert":" "},
			{"insert":"https://ice.io","attributes":{"link":"https://ice.io"}},
			{"insert":" Image "},
			{"insert":{"text-editor-single-image":"64489600-DB30-4725-A178-A9DDE09061E4/L0/001"}},
			{"insert":" List One"},
			{"insert":"\n","attributes":{"list":"bullet"}},
			{"insert":"Two"},
			{"insert":"\n","attributes":{"list":"bullet"}},
			{"insert":" Quote Some quote"},
			{"insert":"\n","attributes":{"blockquote":true}},
			{"insert":" Mentions: "},
			{"insert":"@ckreioosss","attributes":{"mention":"@ckreioosss"}},
			{"insert":" Hashtags: "},
			{"insert":"#Habits","attributes":{"hashtag":"#Habits"}},
			{"insert":" Separator: "},
			{"insert":{"text-editor-separator":"---"}},
			{"insert":" Code block "},
			{"insert":{"text-editor-code":"8361e203-09ba-4eff-aab8-9c9f06df92d3"}},
			{"insert":"\n"}
		]`
		require.Equal(t, "Header 1 Header 2 Header 3 Regular Bold Italic Underline Link wrapped https://ice.io Image List One Two Quote Some quote Mentions: @ckreioosss Hashtags: #Habits Separator: Code block 8361e203-09ba-4eff-aab8-9c9f06df92d3", parseQuillDeltaToPlainText(deltaJSON))
	})
}

func TestReplacePMO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		content          string
		tags             nostr.Tags
		expected         string
		expectError      bool
		replaceCondition func(string, string) (bool, string)
	}{
		{
			name:    "ICIP-7001 example",
			content: "Breaking NEWS! Aliens have landed on the moon! Read all about it on https://bogus.blog.com!",
			tags: nostr.Tags{
				{"pmo", "15:21", "*Aliens*"},
				{"pmo", "68:90", "[Bogus Blog](https://bogus.blog.com)"},
			},
			replaceCondition: func(old, new string) (bool, string) { return true, "" },
			expected:         "Breaking NEWS! *Aliens* have landed on the moon! Read all about it on [Bogus Blog](https://bogus.blog.com)!",
		},
		{
			name:    "single replacement at the start",
			content: "Hello world",
			tags: nostr.Tags{
				{"pmo", "0:5", "Hi"},
			},
			replaceCondition: func(old, new string) (bool, string) { return true, "" },
			expected:         "Hi world",
		},
		{
			name:    "invalid index format",
			content: "No change here",
			tags: nostr.Tags{
				{"pmo", "invalid", "new"},
				{"pmo", "1:2:3", "new"},
			},
			replaceCondition: func(old, new string) (bool, string) { return true, "" },
			expectError:      true,
			expected:         "No change here",
		},
		{
			name:    "out of bounds indices",
			content: "Short",
			tags: nostr.Tags{
				{"pmo", "0:10", "too long"},
				{"pmo", "10:12", "after end"},
				{"pmo", "-1:2", "negative"},
			},
			expectError:      true,
			replaceCondition: func(old, new string) (bool, string) { return true, "" },

			expected: "Short",
		},
		{
			name:             "no pmo tags",
			content:          "Just plain text",
			tags:             nostr.Tags{{"t", "nostr"}},
			replaceCondition: func(old, new string) (bool, string) { return true, "" },
			expected:         "Just plain text",
		},
		{
			name:    "process only mention",
			content: "nprofile1234567789 Hello world",
			tags: nostr.Tags{
				{"pmo", "0:18", "@team"},
				{"pmo", "20:25", "Hi"},
			},
			replaceCondition: func(old, new string) (bool, string) { return strings.HasPrefix(new, "@"), "" },
			expected:         "@team Hello world",
		},
		{
			name:    "process only mention, but replace only part",
			content: "nprofile1234567789 Hello world",
			tags: nostr.Tags{
				{"pmo", "0:18", "[@team](link)"},
				{"pmo", "20:25", "Hi"},
			},
			replaceCondition: func(old, new string) (bool, string) {
				if !strings.HasPrefix(old, "nprofile") {
					return false, ""
				}
				return true, new[1:6]
			},
			expected: "@team Hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &Event{nil, nostr.Event{
				Content: tt.content,
				Tags:    tt.tags,
			}}
			result, err := ReplacePMO(ev, tt.replaceCondition)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
