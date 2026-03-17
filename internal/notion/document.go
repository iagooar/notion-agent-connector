package notion

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type Document struct {
	Title    string
	Markdown string
	Blocks   []Block
}

type Block struct {
	Kind         string
	Text         string
	Inlines      []Inline
	AssetPath    string
	Caption      string
	CodeLanguage string
	Checked      bool
	Icon         string
	Color        string
	Children     []Block
	TableRows    [][][]Inline
	TableHeader  bool
	TableRowHead bool
	RefID        string
}

type Inline struct {
	Text                 string
	Code                 bool
	Bold                 bool
	Italic               bool
	Strikethrough        bool
	Underline            bool
	URL                  string
	Color                string
	MentionType          string
	MentionID            string
	MentionURL           string
	MentionStart         string
	MentionEnd           string
	MentionTimeZone      string
	MentionTemplateType  string
	MentionTemplateValue string
}

type Section struct {
	Heading string
	Level   int
	Start   int
	End     int
	Body    string
}

type BlockSection struct {
	Heading string
	Level   int
	Start   int
	End     int
	Blocks  []Block
}

var orderedListRE = regexp.MustCompile(`^\d+\.\s+`)
var toDoRE = regexp.MustCompile(`^- \[( |x|X)\]\s+(.+)$`)
var imageRE = regexp.MustCompile(`^!\[(.*?)\]\((.+?)\)$`)
var dividerRE = regexp.MustCompile(`^---+$`)
var calloutRE = regexp.MustCompile(`^> \[!([A-Z]+)\]\s*(.*)$`)
var assetSrcAttrRE = regexp.MustCompile(`\bsrc="([^"]+)"`)
var urlAttrRE = regexp.MustCompile(`\burl="([^"]+)"`)
var idAttrRE = regexp.MustCompile(`\bid="([^"]+)"`)
var iconAttrRE = regexp.MustCompile(`\bicon="([^"]+)"`)
var colorAttrRE = regexp.MustCompile(`\bcolor="([^"]+)"`)
var headerRowAttrRE = regexp.MustCompile(`\bheader-row="([^"]+)"`)
var headerColumnAttrRE = regexp.MustCompile(`\bheader-column="([^"]+)"`)
var colorOpenRE = regexp.MustCompile(`(?i)^<span\s+([^>]*\bcolor="([^"]+)"[^>]*)>`)
var underlineOpenRE = regexp.MustCompile(`(?i)^<span\s+([^>]*\bunderline="true"[^>]*)>`)
var mentionPageOpenRE = regexp.MustCompile(`(?i)^<mention-page\s+([^>]*)>`)
var mentionDatabaseOpenRE = regexp.MustCompile(`(?i)^<mention-database\s+([^>]*)>`)
var mentionUserOpenRE = regexp.MustCompile(`(?i)^<mention-user\s+([^>]*)>`)
var mentionDateOpenRE = regexp.MustCompile(`(?i)^<mention-date\s+([^>]*)>`)
var mentionLinkPreviewOpenRE = regexp.MustCompile(`(?i)^<mention-link-preview\s+([^>]*)>`)
var mentionTemplateOpenRE = regexp.MustCompile(`(?i)^<mention-template\s+([^>]*)>`)
var tableRowOpenRE = regexp.MustCompile(`(?i)^<tr>`)
var tableCellRE = regexp.MustCompile(`(?is)<t[dh]>(.*?)</t[dh]>`)
var pageMentionRE = regexp.MustCompile(`^@page\(([^)]+)\)`)
var databaseMentionRE = regexp.MustCompile(`^@database\(([^)]+)\)`)
var userMentionRE = regexp.MustCompile(`^@user\(([^)]+)\)`)

func ParseMarkdown(input string) (Document, error) {
	markdown := NormalizeMarkdown(input)
	lines := strings.Split(markdown, "\n")
	var (
		blocks       []Block
		firstHeading string
	)

	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		switch {
		case strings.HasPrefix(line, "```"):
			language := normalizeCodeLanguage(strings.TrimSpace(strings.TrimPrefix(line, "```")))
			var code []string
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "```" {
				code = append(code, strings.TrimRight(lines[i], " "))
				i++
			}
			if i >= len(lines) {
				return Document{}, fmt.Errorf("unterminated code fence")
			}
			blocks = append(blocks, Block{Kind: "code", Text: strings.TrimSpace(strings.Join(code, "\n")), CodeLanguage: language})
			i++
		case strings.HasPrefix(line, "$$"):
			expression, next, err := parseEquationBlock(lines, i)
			if err != nil {
				return Document{}, err
			}
			blocks = append(blocks, Block{Kind: "equation", Text: expression})
			i = next
		case isToggleStart(line):
			block, next, err := parseToggleBlock(lines, i)
			if err != nil {
				return Document{}, err
			}
			blocks = append(blocks, block)
			i = next
		case isNativeCalloutStart(line):
			block, next, err := parseNativeCalloutBlock(lines, i)
			if err != nil {
				return Document{}, err
			}
			blocks = append(blocks, block)
			i = next
		case isNativeTableStart(line):
			block, next, err := parseNativeTable(lines, i)
			if err != nil {
				return Document{}, err
			}
			blocks = append(blocks, block)
			i = next
		case isMarkdownTableStart(lines, i):
			block, next, err := parseMarkdownTable(lines, i)
			if err != nil {
				return Document{}, err
			}
			blocks = append(blocks, block)
			i = next
		case strings.HasPrefix(line, "# "):
			headingText := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if firstHeading == "" {
				firstHeading = headingText
			} else {
				blocks = append(blocks, newInlineBlock("heading_1", headingText))
			}
			i++
		case strings.HasPrefix(line, "## "):
			blocks = append(blocks, newInlineBlock("heading_2", strings.TrimSpace(strings.TrimPrefix(line, "## "))))
			i++
		case strings.HasPrefix(line, "### "):
			blocks = append(blocks, newInlineBlock("heading_3", strings.TrimSpace(strings.TrimPrefix(line, "### "))))
			i++
		case toDoRE.MatchString(line):
			for i < len(lines) && toDoRE.MatchString(strings.TrimSpace(lines[i])) {
				matches := toDoRE.FindStringSubmatch(strings.TrimSpace(lines[i]))
				blocks = append(blocks, Block{
					Kind:    "to_do",
					Text:    strings.TrimSpace(matches[2]),
					Inlines: parseInline(strings.TrimSpace(matches[2])),
					Checked: strings.EqualFold(matches[1], "x"),
				})
				i++
			}
		case strings.HasPrefix(line, "- "):
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "- "))
				blocks = append(blocks, newInlineBlock("bulleted_list_item", item))
				i++
			}
		case orderedListRE.MatchString(line):
			for i < len(lines) && orderedListRE.MatchString(strings.TrimSpace(lines[i])) {
				item := orderedListRE.ReplaceAllString(strings.TrimSpace(lines[i]), "")
				blocks = append(blocks, newInlineBlock("numbered_list_item", strings.TrimSpace(item)))
				i++
			}
		case calloutRE.MatchString(line):
			block, next := parseCalloutBlock(lines, i)
			blocks = append(blocks, block)
			i = next
		case strings.HasPrefix(line, "> "):
			var quote []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "> ") {
				quote = append(quote, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "> ")))
				i++
			}
			blocks = append(blocks, newInlineBlock("quote", strings.Join(quote, " ")))
		case dividerRE.MatchString(line):
			blocks = append(blocks, Block{Kind: "divider"})
			i++
		case isReferenceLine(line):
			block, ok := parseReferenceLine(line)
			if !ok {
				return Document{}, fmt.Errorf("invalid reference line %q", line)
			}
			blocks = append(blocks, block)
			i++
		case isAssetLine(line):
			block, ok := parseAssetLine(line)
			if !ok {
				return Document{}, fmt.Errorf("invalid asset line %q", line)
			}
			blocks = append(blocks, block)
			i++
		default:
			var paragraph []string
			for i < len(lines) {
				item := strings.TrimSpace(lines[i])
				if item == "" || isBlockStart(item) {
					break
				}
				paragraph = append(paragraph, item)
				i++
			}
			blocks = append(blocks, newInlineBlock("paragraph", strings.Join(paragraph, " ")))
		}
	}

	title := strings.TrimSpace(firstHeading)
	if title == "" {
		return Document{}, fmt.Errorf("markdown needs a level-1 heading for the page title")
	}
	return Document{Title: title, Markdown: markdown, Blocks: compactBlocks(blocks)}, nil
}

func ResolveRelativeAssetPaths(doc *Document, baseDir string) {
	if doc == nil {
		return
	}
	if doc.Markdown != "" {
		doc.Markdown = resolveRelativeAssetPathsInMarkdown(doc.Markdown, baseDir)
	}
	for i := range doc.Blocks {
		resolveRelativePathsForBlock(&doc.Blocks[i], baseDir)
	}
}

func (d Document) HasLocalUploads() bool {
	for _, block := range d.Blocks {
		if hasLocalUploadsInBlock(block) {
			return true
		}
	}
	return false
}

func (d Document) LocalUploadCaptions() []string {
	captions := make([]string, 0)
	for _, block := range d.Blocks {
		collectLocalUploadCaptions(block, &captions)
	}
	return captions
}

func compactBlocks(blocks []Block) []Block {
	out := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		switch block.Kind {
		case "image", "file", "pdf", "audio", "video":
			if strings.TrimSpace(block.AssetPath) == "" {
				continue
			}
		case "divider":
		case "toggle":
			block.Children = compactBlocks(block.Children)
			if len(block.Children) == 0 && block.Text == "" {
				continue
			}
		case "table":
			if len(block.TableRows) == 0 {
				continue
			}
		case "page_ref", "database_ref":
			if strings.TrimSpace(block.RefID) == "" {
				continue
			}
		default:
			block.Text = strings.TrimSpace(block.Text)
			if block.Text == "" && block.Kind != "code" && block.Kind != "callout" && block.Kind != "equation" && block.Kind != "to_do" {
				continue
			}
		}
		out = append(out, block)
	}
	return out
}

func NormalizeMarkdown(input string) string {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func RewriteMarkdownTitle(markdown string, title string) (string, error) {
	lines := strings.Split(NormalizeMarkdown(markdown), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			lines[i] = "# " + strings.TrimSpace(title)
			return strings.Join(lines, "\n"), nil
		}
	}
	return "", fmt.Errorf("markdown needs a level-1 heading for the page title")
}

func ReplaceLocalImageRefs(markdown string, currentMarkdown string) (string, error) {
	return ReplaceLocalAssetRefs(markdown, currentMarkdown)
}

func ReplaceLocalAssetRefs(markdown string, currentMarkdown string) (string, error) {
	currentByKey := map[string][]string{}
	for _, line := range strings.Split(NormalizeMarkdown(currentMarkdown), "\n") {
		block, ok := parseAssetLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		key := assetReplacementKey(block.Kind, block.Caption)
		currentByKey[key] = append(currentByKey[key], strings.TrimSpace(line))
	}

	var out []string
	for _, line := range strings.Split(NormalizeMarkdown(markdown), "\n") {
		trimmed := strings.TrimSpace(line)
		block, ok := parseAssetLine(trimmed)
		if !ok {
			out = append(out, line)
			continue
		}
		if isRemoteAssetPath(block.AssetPath) {
			out = append(out, line)
			continue
		}

		key := assetReplacementKey(block.Kind, block.Caption)
		replacements := currentByKey[key]
		if len(replacements) == 0 {
			return "", fmt.Errorf("local %s %q has no matching asset in the current Notion page; keep captions stable or seed the page first", block.Kind, block.Caption)
		}
		out = append(out, replacements[0])
		currentByKey[key] = replacements[1:]
	}

	return strings.Join(out, "\n"), nil
}

func CountImageLines(markdown string) int {
	count := 0
	for _, line := range strings.Split(NormalizeMarkdown(markdown), "\n") {
		if imageRE.MatchString(strings.TrimSpace(line)) {
			count++
		}
	}
	return count
}

func ExtractSection(markdown string, heading string, level int) (Section, error) {
	markdown = NormalizeMarkdown(markdown)
	lines := strings.Split(markdown, "\n")
	type headingEntry struct {
		Index int
		Level int
		Text  string
	}

	headings := make([]headingEntry, 0)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			headings = append(headings, headingEntry{Index: i, Level: 1, Text: strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))})
		case strings.HasPrefix(trimmed, "## "):
			headings = append(headings, headingEntry{Index: i, Level: 2, Text: strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))})
		case strings.HasPrefix(trimmed, "### "):
			headings = append(headings, headingEntry{Index: i, Level: 3, Text: strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))})
		}
	}

	matches := make([]headingEntry, 0)
	for _, item := range headings {
		if item.Text != strings.TrimSpace(heading) {
			continue
		}
		if level > 0 && item.Level != level {
			continue
		}
		matches = append(matches, item)
	}

	if len(matches) == 0 {
		return Section{}, fmt.Errorf("heading %q not found", heading)
	}
	if len(matches) > 1 {
		return Section{}, fmt.Errorf("heading %q matched %d sections; use a unique heading or specify the level", heading, len(matches))
	}

	match := matches[0]
	end := len(lines)
	for _, item := range headings {
		if item.Index <= match.Index {
			continue
		}
		if item.Level <= match.Level {
			end = item.Index
			break
		}
	}

	body := strings.TrimRight(strings.Join(lines[match.Index:end], "\n"), "\n")
	return Section{
		Heading: match.Text,
		Level:   match.Level,
		Start:   match.Index,
		End:     end,
		Body:    body,
	}, nil
}

func ExtractBlockSection(blocks []Block, heading string, level int) (BlockSection, error) {
	type headingEntry struct {
		Index int
		Level int
		Text  string
	}

	headings := make([]headingEntry, 0)
	for i, block := range blocks {
		level := headingLevelForKind(block.Kind)
		if level == 0 {
			continue
		}
		headings = append(headings, headingEntry{Index: i, Level: level, Text: strings.TrimSpace(block.Text)})
	}

	matches := make([]headingEntry, 0)
	for _, item := range headings {
		if item.Text != strings.TrimSpace(heading) {
			continue
		}
		if level > 0 && item.Level != level {
			continue
		}
		matches = append(matches, item)
	}

	if len(matches) == 0 {
		return BlockSection{}, fmt.Errorf("heading %q not found", heading)
	}
	if len(matches) > 1 {
		return BlockSection{}, fmt.Errorf("heading %q matched %d sections; use a unique heading or specify the level", heading, len(matches))
	}

	match := matches[0]
	end := len(blocks)
	for _, item := range headings {
		if item.Index <= match.Index {
			continue
		}
		if item.Level <= match.Level {
			end = item.Index
			break
		}
	}

	return BlockSection{
		Heading: match.Text,
		Level:   match.Level,
		Start:   match.Index,
		End:     end,
		Blocks:  append([]Block(nil), blocks[match.Index:end]...),
	}, nil
}

func isBlockStart(line string) bool {
	switch {
	case strings.HasPrefix(line, "```"):
		return true
	case strings.HasPrefix(line, "$$"):
		return true
	case isToggleStart(line):
		return true
	case isNativeCalloutStart(line):
		return true
	case isNativeTableStart(line):
		return true
	case strings.HasPrefix(line, "|"):
		return true
	case strings.HasPrefix(line, "# "):
		return true
	case strings.HasPrefix(line, "## "):
		return true
	case strings.HasPrefix(line, "### "):
		return true
	case toDoRE.MatchString(line):
		return true
	case strings.HasPrefix(line, "- "):
		return true
	case calloutRE.MatchString(line):
		return true
	case strings.HasPrefix(line, "> "):
		return true
	case dividerRE.MatchString(line):
		return true
	case isReferenceLine(line):
		return true
	case isAssetLine(line):
		return true
	case orderedListRE.MatchString(line):
		return true
	default:
		return false
	}
}

func headingLevelForKind(kind string) int {
	switch kind {
	case "heading_1":
		return 1
	case "heading_2":
		return 2
	case "heading_3":
		return 3
	default:
		return 0
	}
}

func normalizeCodeLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "text", "txt", "plain", "plaintext":
		return "plain text"
	default:
		return language
	}
}

func newInlineBlock(kind, text string) Block {
	return Block{
		Kind:    kind,
		Text:    text,
		Inlines: parseInline(text),
	}
}

func isNativeCalloutStart(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(line)), "<callout")
}

func parseNativeCalloutBlock(lines []string, start int) (Block, int, error) {
	line := strings.TrimSpace(lines[start])
	openEnd := strings.Index(line, ">")
	if openEnd < 0 {
		return Block{}, 0, fmt.Errorf("invalid <callout> block")
	}
	attrs := strings.TrimSpace(line[len("<callout"):openEnd])
	if strings.TrimSpace(line[openEnd+1:]) != "" {
		return Block{}, 0, fmt.Errorf("inline <callout> content is not supported")
	}

	i := start + 1
	var inner []string
	for i < len(lines) {
		current := strings.TrimSpace(lines[i])
		if current == "</callout>" {
			break
		}
		inner = append(inner, stripOneIndentLevel(lines[i]))
		i++
	}
	if i >= len(lines) {
		return Block{}, 0, fmt.Errorf("unterminated <callout> block")
	}

	children, err := parseMarkdownFragment(strings.Join(inner, "\n"))
	if err != nil {
		return Block{}, 0, err
	}

	block := Block{
		Kind:  "callout",
		Icon:  strings.TrimSpace(extractAttr(attrs, iconAttrRE)),
		Color: strings.TrimSpace(extractAttr(attrs, colorAttrRE)),
	}
	if block.Icon == "" {
		block.Icon = "💬"
	}
	if len(children) > 0 {
		if children[0].Kind == "paragraph" {
			block.Text = children[0].Text
			block.Inlines = children[0].Inlines
			block.Children = append([]Block(nil), children[1:]...)
		} else {
			block.Children = children
		}
	}
	return block, i + 1, nil
}

func resolveRelativeAssetPathsInMarkdown(markdown string, baseDir string) string {
	lines := strings.Split(NormalizeMarkdown(markdown), "\n")
	for i, line := range lines {
		block, ok := parseAssetLine(strings.TrimSpace(line))
		if !ok || isRemoteAssetPath(block.AssetPath) || filepath.IsAbs(block.AssetPath) {
			continue
		}
		lines[i] = strings.Replace(line, block.AssetPath, filepath.Clean(filepath.Join(baseDir, block.AssetPath)), 1)
	}
	return strings.Join(lines, "\n")
}

func isAssetLine(line string) bool {
	_, ok := parseAssetLine(strings.TrimSpace(line))
	return ok
}

func isReferenceLine(line string) bool {
	_, ok := parseReferenceLine(strings.TrimSpace(line))
	return ok
}

func parseAssetLine(line string) (Block, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Block{}, false
	}
	if matches := imageRE.FindStringSubmatch(line); len(matches) > 0 {
		return Block{
			Kind:      "image",
			Caption:   strings.TrimSpace(matches[1]),
			AssetPath: strings.TrimSpace(matches[2]),
		}, true
	}
	return parseTaggedAssetLine(line)
}

func parseTaggedAssetLine(line string) (Block, bool) {
	for _, kind := range []string{"file", "pdf", "audio", "video"} {
		prefix := "<" + kind + " "
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		if strings.HasSuffix(line, "/>") {
			attrs := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, prefix), "/>"))
			src := extractAssetSrc(attrs)
			if src == "" {
				return Block{}, false
			}
			return Block{Kind: kind, AssetPath: src}, true
		}

		closeTag := "</" + kind + ">"
		if !strings.HasSuffix(line, closeTag) {
			return Block{}, false
		}
		openEnd := strings.Index(line, ">")
		if openEnd < 0 {
			return Block{}, false
		}
		attrs := strings.TrimSpace(line[len(prefix):openEnd])
		src := extractAssetSrc(attrs)
		if src == "" {
			return Block{}, false
		}
		return Block{
			Kind:      kind,
			AssetPath: src,
			Caption:   strings.TrimSpace(line[openEnd+1 : len(line)-len(closeTag)]),
		}, true
	}
	return Block{}, false
}

func parseReferenceLine(line string) (Block, bool) {
	line = strings.TrimSpace(line)
	for _, kind := range []string{"page", "database"} {
		prefix := "<" + kind + " "
		closeTag := "</" + kind + ">"
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, closeTag) {
			continue
		}
		openEnd := strings.Index(line, ">")
		if openEnd < 0 {
			return Block{}, false
		}
		attrs := strings.TrimSpace(line[len(prefix):openEnd])
		ref := extractReferenceID(attrs)
		if ref == "" {
			return Block{}, false
		}
		label := strings.TrimSpace(line[openEnd+1 : len(line)-len(closeTag)])
		block := Block{Text: label, RefID: ref}
		if kind == "page" {
			block.Kind = "page_ref"
		} else {
			block.Kind = "database_ref"
		}
		return block, true
	}
	return Block{}, false
}

func extractAssetSrc(attrs string) string {
	matches := assetSrcAttrRE.FindStringSubmatch(attrs)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func extractReferenceID(attrs string) string {
	if matches := urlAttrRE.FindStringSubmatch(attrs); len(matches) > 0 {
		return normalizeNotionObjectID(matches[1])
	}
	return ""
}

func assetReplacementKey(kind string, caption string) string {
	return kind + "\x00" + strings.TrimSpace(caption)
}

func isAssetBlockKind(kind string) bool {
	switch kind {
	case "image", "file", "pdf", "audio", "video":
		return true
	default:
		return false
	}
}

func isRemoteAssetPath(path string) bool {
	parsed, err := url.Parse(strings.TrimSpace(path))
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func parseInline(text string) []Inline {
	if text == "" {
		return nil
	}
	return compactInlines(parseInlineStyled(text, Inline{}))
}

func normalizeNotionObjectID(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		path := strings.Trim(parsed.Path, "/")
		if path != "" {
			parts := strings.Split(path, "-")
			last := parts[len(parts)-1]
			if len(last) == 32 {
				return last
			}
			if len(strings.ReplaceAll(last, "-", "")) == 32 {
				return strings.ReplaceAll(last, "-", "")
			}
		}
	}
	clean := strings.ReplaceAll(trimmed, "-", "")
	if len(clean) != 32 {
		return trimmed
	}
	return clean[0:8] + "-" + clean[8:12] + "-" + clean[12:16] + "-" + clean[16:20] + "-" + clean[20:32]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func parseEquationBlock(lines []string, start int) (string, int, error) {
	line := strings.TrimSpace(lines[start])
	if line != "$$" {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "$$"), "$$")), start + 1, nil
	}
	var body []string
	i := start + 1
	for i < len(lines) && strings.TrimSpace(lines[i]) != "$$" {
		body = append(body, strings.TrimSpace(lines[i]))
		i++
	}
	if i >= len(lines) {
		return "", 0, fmt.Errorf("unterminated equation block")
	}
	return strings.TrimSpace(strings.Join(body, "\n")), i + 1, nil
}

func parseCalloutBlock(lines []string, start int) (Block, int) {
	first := strings.TrimSpace(lines[start])
	matches := calloutRE.FindStringSubmatch(first)
	kind := "NOTE"
	text := ""
	if len(matches) > 0 {
		kind = strings.TrimSpace(matches[1])
		text = strings.TrimSpace(matches[2])
	}
	body := []string{text}
	i := start + 1
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "> ") && !calloutRE.MatchString(strings.TrimSpace(lines[i])) {
		body = append(body, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "> ")))
		i++
	}
	content := strings.TrimSpace(strings.Join(compactMarkdown(body), " "))
	return Block{
		Kind:    "callout",
		Text:    content,
		Inlines: parseInline(content),
		Icon:    defaultCalloutIcon(kind),
	}, i
}

func isToggleStart(line string) bool {
	line = strings.TrimSpace(line)
	return line == "<details>" || strings.HasPrefix(line, "<details><summary>")
}

func parseToggleBlock(lines []string, start int) (Block, int, error) {
	line := strings.TrimSpace(lines[start])
	summary := ""
	i := start
	if strings.HasPrefix(line, "<details><summary>") && strings.HasSuffix(line, "</summary>") {
		summary = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<details><summary>"), "</summary>"))
		i++
	} else {
		i++
		if i >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[i]), "<summary>") || !strings.HasSuffix(strings.TrimSpace(lines[i]), "</summary>") {
			return Block{}, 0, fmt.Errorf("toggle block needs a <summary> line")
		}
		summary = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(lines[i]), "<summary>"), "</summary>"))
		i++
	}
	var inner []string
	depth := 1
	for i < len(lines) {
		current := strings.TrimSpace(lines[i])
		if current == "<details>" || strings.HasPrefix(current, "<details><summary>") {
			depth++
		}
		if current == "</details>" {
			depth--
			if depth == 0 {
				children, err := parseMarkdownFragment(strings.Join(inner, "\n"))
				if err != nil {
					return Block{}, 0, err
				}
				return Block{
					Kind:     "toggle",
					Text:     summary,
					Inlines:  parseInline(summary),
					Children: children,
				}, i + 1, nil
			}
		}
		inner = append(inner, lines[i])
		i++
	}
	return Block{}, 0, fmt.Errorf("unterminated <details> block")
}

func parseMarkdownFragment(markdown string) ([]Block, error) {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return nil, nil
	}
	doc, err := ParseMarkdown("# Fragment\n\n" + markdown)
	if err != nil {
		return nil, err
	}
	return doc.Blocks, nil
}

func isMarkdownTableStart(lines []string, start int) bool {
	if start+1 >= len(lines) {
		return false
	}
	header := strings.TrimSpace(lines[start])
	divider := strings.TrimSpace(lines[start+1])
	return strings.HasPrefix(header, "|") && strings.HasPrefix(divider, "|") && strings.Contains(divider, "---")
}

func parseMarkdownTable(lines []string, start int) (Block, int, error) {
	headerCells := splitMarkdownTableRow(lines[start])
	if len(headerCells) == 0 {
		return Block{}, 0, fmt.Errorf("invalid markdown table header")
	}
	i := start + 2
	rows := make([][][]Inline, 0)
	rows = append(rows, parseTableRowInlines(headerCells))
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
		rows = append(rows, parseTableRowInlines(splitMarkdownTableRow(lines[i])))
		i++
	}
	return Block{
		Kind:        "table",
		TableRows:   rows,
		TableHeader: true,
	}, i, nil
}

func isNativeTableStart(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(line)), "<table")
}

func parseNativeTable(lines []string, start int) (Block, int, error) {
	line := strings.TrimSpace(lines[start])
	openEnd := strings.Index(line, ">")
	if openEnd < 0 {
		return Block{}, 0, fmt.Errorf("invalid <table> block")
	}
	attrs := strings.TrimSpace(line[len("<table"):openEnd])
	hasHeaderRow := true
	if raw := strings.TrimSpace(extractAttr(attrs, headerRowAttrRE)); raw != "" {
		hasHeaderRow = !strings.EqualFold(raw, "false")
	}
	hasHeaderColumn := false
	if raw := strings.TrimSpace(extractAttr(attrs, headerColumnAttrRE)); raw != "" {
		hasHeaderColumn = strings.EqualFold(raw, "true")
	}

	rows := make([][][]Inline, 0)
	i := start + 1
	for i < len(lines) {
		current := strings.TrimSpace(lines[i])
		switch {
		case current == "</table>":
			if len(rows) == 0 {
				return Block{}, 0, fmt.Errorf("table needs at least one row")
			}
			return Block{
				Kind:         "table",
				TableRows:    rows,
				TableHeader:  hasHeaderRow,
				TableRowHead: hasHeaderColumn,
			}, i + 1, nil
		case current == "":
			i++
		case tableRowOpenRE.MatchString(current):
			row, next, err := parseNativeTableRow(lines, i)
			if err != nil {
				return Block{}, 0, err
			}
			rows = append(rows, row)
			i = next
		default:
			return Block{}, 0, fmt.Errorf("unexpected line inside <table>: %q", current)
		}
	}
	return Block{}, 0, fmt.Errorf("unterminated <table> block")
}

func parseNativeTableRow(lines []string, start int) ([][]Inline, int, error) {
	var rowLines []string
	i := start
	for i < len(lines) {
		current := strings.TrimSpace(lines[i])
		rowLines = append(rowLines, current)
		if current == "</tr>" {
			break
		}
		i++
	}
	if i >= len(lines) {
		return nil, 0, fmt.Errorf("unterminated <tr> row")
	}
	rowMarkdown := strings.Join(rowLines, "\n")
	matches := tableCellRE.FindAllStringSubmatch(rowMarkdown, -1)
	if len(matches) == 0 {
		return nil, 0, fmt.Errorf("table row has no cells")
	}
	row := make([][]Inline, 0, len(matches))
	for _, match := range matches {
		row = append(row, parseInline(strings.TrimSpace(htmlWhitespaceToSpaces(match[1]))))
	}
	return row, i + 1, nil
}

func splitMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func parseTableRowInlines(cells []string) [][]Inline {
	row := make([][]Inline, 0, len(cells))
	for _, cell := range cells {
		row = append(row, parseInline(cell))
	}
	return row
}

func defaultCalloutIcon(kind string) string {
	switch strings.TrimSpace(strings.ToUpper(kind)) {
	case "NOTE":
		return "📝"
	case "TIP":
		return "💡"
	case "IMPORTANT":
		return "❗"
	case "WARNING":
		return "⚠️"
	case "CAUTION":
		return "🚨"
	default:
		return "💬"
	}
}

func parseInlineStyled(text string, style Inline) []Inline {
	var out []Inline
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(strings.ToLower(text), "<span"):
			parsed, rest, ok := parseStyledSpan(text, style)
			if ok {
				out = append(out, parsed...)
				text = rest
				continue
			}
		case strings.HasPrefix(strings.ToLower(text), "<mention-"):
			inline, rest, ok := parseNativeMentionInline(text, style)
			if ok {
				out = append(out, inline)
				text = rest
				continue
			}
		case pageMentionRE.MatchString(text):
			matches := pageMentionRE.FindStringSubmatch(text)
			mention := style
			mention.Text = "@page(" + matches[1] + ")"
			mention.MentionType = "page"
			mention.MentionID = normalizeNotionObjectID(matches[1])
			out = append(out, mention)
			text = text[len(matches[0]):]
		case databaseMentionRE.MatchString(text):
			matches := databaseMentionRE.FindStringSubmatch(text)
			mention := style
			mention.Text = "@database(" + matches[1] + ")"
			mention.MentionType = "database"
			mention.MentionID = normalizeNotionObjectID(matches[1])
			out = append(out, mention)
			text = text[len(matches[0]):]
		case userMentionRE.MatchString(text):
			matches := userMentionRE.FindStringSubmatch(text)
			mention := style
			mention.Text = "@user(" + matches[1] + ")"
			mention.MentionType = "user"
			mention.MentionID = strings.TrimSpace(matches[1])
			out = append(out, mention)
			text = text[len(matches[0]):]
		case strings.HasPrefix(text, "`"):
			end := strings.Index(text[1:], "`")
			if end < 0 {
				out = append(out, inlineWithText(style, text))
				return out
			}
			code := style
			code.Code = true
			out = append(out, inlineWithText(code, text[1:1+end]))
			text = text[2+end:]
		case strings.HasPrefix(text, "**") || strings.HasPrefix(text, "__"):
			token := text[:2]
			end := strings.Index(text[2:], token)
			if end < 0 {
				out = append(out, inlineWithText(style, token))
				text = text[2:]
				continue
			}
			next := style
			next.Bold = true
			out = append(out, parseInlineStyled(text[2:2+end], next)...)
			text = text[4+end:]
		case strings.HasPrefix(text, "~~"):
			end := strings.Index(text[2:], "~~")
			if end < 0 {
				out = append(out, inlineWithText(style, "~~"))
				text = text[2:]
				continue
			}
			next := style
			next.Strikethrough = true
			out = append(out, parseInlineStyled(text[2:2+end], next)...)
			text = text[4+end:]
		case strings.HasPrefix(text, "<u>"):
			end := strings.Index(strings.ToLower(text), "</u>")
			if end < 0 {
				out = append(out, inlineWithText(style, "<u>"))
				text = text[3:]
				continue
			}
			next := style
			next.Underline = true
			out = append(out, parseInlineStyled(text[3:end], next)...)
			text = text[end+4:]
		case strings.HasPrefix(text, "*") || strings.HasPrefix(text, "_"):
			token := text[:1]
			end := strings.Index(text[1:], token)
			if end < 0 {
				out = append(out, inlineWithText(style, token))
				text = text[1:]
				continue
			}
			next := style
			next.Italic = true
			out = append(out, parseInlineStyled(text[1:1+end], next)...)
			text = text[2+end:]
		case strings.HasPrefix(text, "["):
			labelEnd := strings.Index(text, "](")
			if labelEnd < 0 {
				out = append(out, inlineWithText(style, "["))
				text = text[1:]
				continue
			}
			urlEnd := strings.Index(text[labelEnd+2:], ")")
			if urlEnd < 0 {
				out = append(out, inlineWithText(style, "["))
				text = text[1:]
				continue
			}
			next := style
			next.URL = strings.TrimSpace(text[labelEnd+2 : labelEnd+2+urlEnd])
			out = append(out, parseInlineStyled(text[1:labelEnd], next)...)
			text = text[labelEnd+3+urlEnd:]
		default:
			nextMarker := nextInlineMarker(text)
			plain := text
			if nextMarker >= 0 {
				plain = text[:nextMarker]
				text = text[nextMarker:]
			} else {
				text = ""
			}
			if plain != "" {
				out = append(out, inlineWithText(style, plain))
			}
		}
	}
	return out
}

func nextInlineMarker(text string) int {
	markers := []string{"<span", "<mention-", "@page(", "@database(", "@user(", "`", "**", "__", "~~", "<u>", "*", "_", "["}
	best := -1
	for _, marker := range markers {
		index := strings.Index(text, marker)
		if index < 0 {
			continue
		}
		if best < 0 || index < best {
			best = index
		}
	}
	return best
}

func inlineWithText(style Inline, text string) Inline {
	style.Text = text
	return style
}

func compactInlines(inlines []Inline) []Inline {
	if len(inlines) == 0 {
		return nil
	}
	out := make([]Inline, 0, len(inlines))
	for _, inline := range inlines {
		if inline.Text == "" {
			continue
		}
		if len(out) > 0 && sameInlineStyle(out[len(out)-1], inline) {
			out[len(out)-1].Text += inline.Text
			continue
		}
		out = append(out, inline)
	}
	return out
}

func sameInlineStyle(left Inline, right Inline) bool {
	return left.Code == right.Code &&
		left.Bold == right.Bold &&
		left.Italic == right.Italic &&
		left.Strikethrough == right.Strikethrough &&
		left.Underline == right.Underline &&
		left.URL == right.URL &&
		left.Color == right.Color &&
		left.MentionType == right.MentionType &&
		left.MentionID == right.MentionID &&
		left.MentionURL == right.MentionURL &&
		left.MentionStart == right.MentionStart &&
		left.MentionEnd == right.MentionEnd &&
		left.MentionTimeZone == right.MentionTimeZone &&
		left.MentionTemplateType == right.MentionTemplateType &&
		left.MentionTemplateValue == right.MentionTemplateValue
}

func parseStyledSpan(text string, style Inline) ([]Inline, string, bool) {
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, "<span") {
		return nil, text, false
	}
	openEnd := strings.Index(text, ">")
	closeStart := strings.Index(lower, "</span>")
	if openEnd < 0 || closeStart < 0 || closeStart < openEnd {
		return nil, text, false
	}
	attrs := text[:openEnd+1]
	body := text[openEnd+1 : closeStart]
	next := style
	if matches := colorOpenRE.FindStringSubmatch(attrs); len(matches) > 2 {
		next.Color = strings.TrimSpace(matches[2])
	}
	if underlineOpenRE.MatchString(attrs) {
		next.Underline = true
	}
	return parseInlineStyled(body, next), text[closeStart+7:], true
}

func parseNativeMentionInline(text string, style Inline) (Inline, string, bool) {
	type mentionSpec struct {
		openRE      *regexp.Regexp
		closeTag    string
		mentionType string
	}
	specs := []mentionSpec{
		{openRE: mentionPageOpenRE, closeTag: "</mention-page>", mentionType: "page"},
		{openRE: mentionDatabaseOpenRE, closeTag: "</mention-database>", mentionType: "database"},
		{openRE: mentionUserOpenRE, closeTag: "</mention-user>", mentionType: "user"},
		{openRE: mentionDateOpenRE, closeTag: "</mention-date>", mentionType: "date"},
		{openRE: mentionLinkPreviewOpenRE, closeTag: "</mention-link-preview>", mentionType: "link_preview"},
		{openRE: mentionTemplateOpenRE, closeTag: "</mention-template>", mentionType: "template_mention"},
	}

	lower := strings.ToLower(text)
	for _, spec := range specs {
		matches := spec.openRE.FindStringSubmatch(text)
		if len(matches) == 0 {
			continue
		}
		openEnd := strings.Index(text, ">")
		closeStart := strings.Index(lower, spec.closeTag)
		if openEnd < 0 || closeStart < openEnd {
			return Inline{}, text, false
		}
		attrs := matches[1]
		label := strings.TrimSpace(htmlWhitespaceToSpaces(text[openEnd+1 : closeStart]))
		mention := style
		mention.Text = label
		mention.MentionType = spec.mentionType
		switch spec.mentionType {
		case "page", "database":
			mention.MentionID = normalizeNotionObjectID(extractAttr(attrs, urlAttrRE))
		case "user":
			mention.MentionID = normalizeNotionObjectID(firstNonEmpty(extractAttr(attrs, idAttrRE), extractAttr(attrs, urlAttrRE)))
		case "date":
			mention.MentionStart = strings.TrimSpace(extractAttr(attrs, regexp.MustCompile(`\bstart="([^"]+)"`)))
			mention.MentionEnd = strings.TrimSpace(extractAttr(attrs, regexp.MustCompile(`\bend="([^"]+)"`)))
			mention.MentionTimeZone = strings.TrimSpace(extractAttr(attrs, regexp.MustCompile(`\btimezone="([^"]+)"`)))
		case "link_preview":
			mention.MentionURL = strings.TrimSpace(extractAttr(attrs, urlAttrRE))
		case "template_mention":
			mention.MentionTemplateType = strings.TrimSpace(extractAttr(attrs, regexp.MustCompile(`\btype="([^"]+)"`)))
			mention.MentionTemplateValue = strings.TrimSpace(extractAttr(attrs, regexp.MustCompile(`\bvalue="([^"]+)"`)))
		}
		return mention, text[closeStart+len(spec.closeTag):], true
	}
	return Inline{}, text, false
}

func extractAttr(attrs string, re *regexp.Regexp) string {
	matches := re.FindStringSubmatch(attrs)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1])
}

func stripOneIndentLevel(line string) string {
	switch {
	case strings.HasPrefix(line, "\t"):
		return strings.TrimPrefix(line, "\t")
	case strings.HasPrefix(line, "    "):
		return strings.TrimPrefix(line, "    ")
	default:
		return line
	}
}

func htmlWhitespaceToSpaces(text string) string {
	replacer := strings.NewReplacer("&nbsp;", " ", "\t", " ", "\n", " ")
	return strings.TrimSpace(replacer.Replace(text))
}

func resolveRelativePathsForBlock(block *Block, baseDir string) {
	if block == nil {
		return
	}
	if isAssetBlockKind(block.Kind) && block.AssetPath != "" && !filepath.IsAbs(block.AssetPath) {
		if parsed, err := url.Parse(block.AssetPath); err == nil && parsed.Scheme == "" {
			block.AssetPath = filepath.Clean(filepath.Join(baseDir, block.AssetPath))
		}
	}
	for i := range block.Children {
		resolveRelativePathsForBlock(&block.Children[i], baseDir)
	}
}

func hasLocalUploadsInBlock(block Block) bool {
	if isAssetBlockKind(block.Kind) && !isRemoteAssetPath(block.AssetPath) {
		return true
	}
	for _, child := range block.Children {
		if hasLocalUploadsInBlock(child) {
			return true
		}
	}
	return false
}

func collectLocalUploadCaptions(block Block, out *[]string) {
	if out == nil {
		return
	}
	if isAssetBlockKind(block.Kind) && !isRemoteAssetPath(block.AssetPath) {
		*out = append(*out, block.Caption)
	}
	for _, child := range block.Children {
		collectLocalUploadCaptions(child, out)
	}
}
