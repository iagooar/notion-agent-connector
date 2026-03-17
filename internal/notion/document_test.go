package notion

import (
	"path/filepath"
	"testing"
)

func TestParseMarkdown(t *testing.T) {
	doc, err := ParseMarkdown("# Project\n\nIntro\n\n- A\n- B\n\n```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if doc.Title != "Project" {
		t.Fatalf("Title = %q", doc.Title)
	}
	if len(doc.Blocks) != 4 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	if doc.Blocks[3].CodeLanguage != "go" {
		t.Fatalf("CodeLanguage = %q", doc.Blocks[3].CodeLanguage)
	}
}

func TestParseMarkdownCapturesInlineCode(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\nUse `support_ticket` here.")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	got := doc.Blocks[0].Inlines
	if len(got) != 3 {
		t.Fatalf("len(Inlines) = %d", len(got))
	}
	if got[1].Text != "support_ticket" || !got[1].Code {
		t.Fatalf("expected inline code span, got %#v", got[1])
	}
}

func TestParseMarkdownCapturesRichInlineFormatting(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\nUse **bold**, *italic*, ~~gone~~, <u>underlined</u>, and [docs](https://example.com).")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	var (
		sawBold      bool
		sawItalic    bool
		sawStrike    bool
		sawUnderline bool
		sawLink      bool
	)
	for _, inline := range doc.Blocks[0].Inlines {
		sawBold = sawBold || inline.Bold
		sawItalic = sawItalic || inline.Italic
		sawStrike = sawStrike || inline.Strikethrough
		sawUnderline = sawUnderline || inline.Underline
		sawLink = sawLink || inline.URL == "https://example.com"
	}
	if !sawBold || !sawItalic || !sawStrike || !sawUnderline || !sawLink {
		t.Fatalf("unexpected inline parse = %#v", doc.Blocks[0].Inlines)
	}
}

func TestRewriteMarkdownTitle(t *testing.T) {
	got, err := RewriteMarkdownTitle("# Manual\n\nBody", "Live page")
	if err != nil {
		t.Fatalf("RewriteMarkdownTitle() error = %v", err)
	}
	if got != "# Live page\n\nBody" {
		t.Fatalf("RewriteMarkdownTitle() = %q", got)
	}
}

func TestReplaceLocalImageRefs(t *testing.T) {
	source := "# Manual\n\n![Setup](../assets/setup.png)"
	current := "# Manual\n\n![Setup](https://signed.example/setup.png)"
	got, err := ReplaceLocalImageRefs(source, current)
	if err != nil {
		t.Fatalf("ReplaceLocalImageRefs() error = %v", err)
	}
	if got != "# Manual\n\n![Setup](https://signed.example/setup.png)" {
		t.Fatalf("ReplaceLocalImageRefs() = %q", got)
	}
}

func TestParseMarkdownCapturesTaggedAssetBlocks(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\n<pdf src=\"docs/spec.pdf\">Spec</pdf>\n\n<file src=\"https://example.com/archive.zip\">Archive</file>\n\n<audio src=\"clips/intro.mp3\">Intro</audio>\n\n<video src=\"https://example.com/demo.mp4\">Demo</video>")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if len(doc.Blocks) != 4 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	if doc.Blocks[0].Kind != "pdf" || doc.Blocks[0].AssetPath != "docs/spec.pdf" || doc.Blocks[0].Caption != "Spec" {
		t.Fatalf("unexpected pdf block = %#v", doc.Blocks[0])
	}
	if doc.Blocks[1].Kind != "file" || doc.Blocks[1].AssetPath != "https://example.com/archive.zip" {
		t.Fatalf("unexpected file block = %#v", doc.Blocks[1])
	}
	if doc.Blocks[2].Kind != "audio" || doc.Blocks[2].AssetPath != "clips/intro.mp3" {
		t.Fatalf("unexpected audio block = %#v", doc.Blocks[2])
	}
	if doc.Blocks[3].Kind != "video" || doc.Blocks[3].AssetPath != "https://example.com/demo.mp4" {
		t.Fatalf("unexpected video block = %#v", doc.Blocks[3])
	}
}

func TestParseMarkdownCapturesToDosCalloutsDividerAndEquation(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\n- [ ] Open task\n- [x] Closed task\n\n> [!WARNING] Watch this\n\n---\n\n$$\na^2 + b^2 = c^2\n$$")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if len(doc.Blocks) != 5 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	if doc.Blocks[0].Kind != "to_do" || doc.Blocks[0].Checked {
		t.Fatalf("unexpected first block = %#v", doc.Blocks[0])
	}
	if doc.Blocks[1].Kind != "to_do" || !doc.Blocks[1].Checked {
		t.Fatalf("unexpected second block = %#v", doc.Blocks[1])
	}
	if doc.Blocks[2].Kind != "callout" || doc.Blocks[2].Icon == "" {
		t.Fatalf("unexpected callout block = %#v", doc.Blocks[2])
	}
	if doc.Blocks[3].Kind != "divider" {
		t.Fatalf("unexpected divider block = %#v", doc.Blocks[3])
	}
	if doc.Blocks[4].Kind != "equation" || doc.Blocks[4].Text != "a^2 + b^2 = c^2" {
		t.Fatalf("unexpected equation block = %#v", doc.Blocks[4])
	}
}

func TestParseMarkdownCapturesNativeCalloutTableAndMentions(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\n<callout icon=\"💡\" color=\"yellow_background\">\n\tUse <mention-page url=\"https://www.notion.so/324c89e1ec8980f38b44e4b064fff562\">Notion Go Connector</mention-page> carefully.\n\t- [ ] Keep structure\n</callout>\n\n<table header-row=\"true\" header-column=\"false\">\n<tr>\n<td>Primitive</td>\n<td>Status</td>\n</tr>\n<tr>\n<td>Tables</td>\n<td>Working</td>\n</tr>\n</table>")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	callout := doc.Blocks[0]
	if callout.Kind != "callout" || callout.Icon != "💡" || callout.Color != "yellow_background" {
		t.Fatalf("unexpected callout = %#v", callout)
	}
	if len(callout.Inlines) == 0 || callout.Inlines[1].MentionType != "page" {
		t.Fatalf("expected page mention in callout, got %#v", callout.Inlines)
	}
	if len(callout.Children) != 1 || callout.Children[0].Kind != "to_do" {
		t.Fatalf("unexpected callout children = %#v", callout.Children)
	}
	table := doc.Blocks[1]
	if table.Kind != "table" || !table.TableHeader || table.TableRowHead {
		t.Fatalf("unexpected table metadata = %#v", table)
	}
	if len(table.TableRows) != 2 || len(table.TableRows[0]) != 2 {
		t.Fatalf("unexpected table rows = %#v", table.TableRows)
	}
}

func TestParseMarkdownCapturesDateAndTemplateMentions(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\nRelease <mention-date start=\"2026-03-17\" timezone=\"Europe/Warsaw\">March 17</mention-date> for <mention-template type=\"template_mention_date\" value=\"today\">today</mention-template>.")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(Blocks) = %d", len(doc.Blocks))
	}
	inlines := doc.Blocks[0].Inlines
	var sawDate bool
	var sawTemplate bool
	for _, inline := range inlines {
		if inline.MentionType == "date" && inline.MentionStart == "2026-03-17" && inline.MentionTimeZone == "Europe/Warsaw" {
			sawDate = true
		}
		if inline.MentionType == "template_mention" && inline.MentionTemplateType == "template_mention_date" && inline.MentionTemplateValue == "today" {
			sawTemplate = true
		}
	}
	if !sawDate || !sawTemplate {
		t.Fatalf("unexpected inlines = %#v", inlines)
	}
}

func TestResolveRelativeAssetPathsRewritesTaggedAssets(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\n<pdf src=\"docs/spec.pdf\">Spec</pdf>\n\n![Setup](assets/setup.png)")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	ResolveRelativeAssetPaths(&doc, "/workspace/project")

	if got := doc.Blocks[0].AssetPath; got != filepath.Clean("/workspace/project/docs/spec.pdf") {
		t.Fatalf("pdf AssetPath = %q", got)
	}
	if got := doc.Blocks[1].AssetPath; got != filepath.Clean("/workspace/project/assets/setup.png") {
		t.Fatalf("image AssetPath = %q", got)
	}
	if got := doc.Markdown; got != "# Manual\n\n<pdf src=\"/workspace/project/docs/spec.pdf\">Spec</pdf>\n\n![Setup](/workspace/project/assets/setup.png)" {
		t.Fatalf("Markdown = %q", got)
	}
}

func TestReplaceLocalAssetRefs(t *testing.T) {
	source := "# Manual\n\n<pdf src=\"../assets/spec.pdf\">Spec</pdf>"
	current := "# Manual\n\n<pdf src=\"https://signed.example/spec.pdf\">Spec</pdf>"
	got, err := ReplaceLocalAssetRefs(source, current)
	if err != nil {
		t.Fatalf("ReplaceLocalAssetRefs() error = %v", err)
	}
	if got != "# Manual\n\n<pdf src=\"https://signed.example/spec.pdf\">Spec</pdf>" {
		t.Fatalf("ReplaceLocalAssetRefs() = %q", got)
	}
}

func TestExtractSection(t *testing.T) {
	markdown := "# Manual\n\nIntro\n\n## Operator setup\n\nStep 1\n\n### Details\n\nExtra\n\n## Local config\n\nDone"
	section, err := ExtractSection(markdown, "Operator setup", 2)
	if err != nil {
		t.Fatalf("ExtractSection() error = %v", err)
	}
	want := "## Operator setup\n\nStep 1\n\n### Details\n\nExtra"
	if section.Body != want {
		t.Fatalf("section.Body = %q", section.Body)
	}
}

func TestExtractBlockSection(t *testing.T) {
	doc, err := ParseMarkdown("# Manual\n\nIntro\n\n## Operator setup\n\nStep 1\n\n### Details\n\nExtra\n\n## Local config\n\nDone")
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	section, err := ExtractBlockSection(doc.Blocks, "Operator setup", 2)
	if err != nil {
		t.Fatalf("ExtractBlockSection() error = %v", err)
	}
	if section.Start != 1 || section.End != 5 {
		t.Fatalf("section range = %d..%d", section.Start, section.End)
	}
	if len(section.Blocks) != 4 {
		t.Fatalf("len(section.Blocks) = %d", len(section.Blocks))
	}
	if section.Blocks[0].Kind != "heading_2" || section.Blocks[2].Kind != "heading_3" {
		t.Fatalf("unexpected section blocks = %#v", section.Blocks)
	}
}
