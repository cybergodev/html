// extract_format.go holds the output-formatting stage of the pipeline:
// Markdown/HTML inline rendering, placeholder substitution, and the
// escaping helpers they share.

package html

import (
	htmlstd "html"
	"strconv"
	"strings"

	"github.com/cybergodev/html/internal"
	"github.com/cybergodev/html/internal/table"
	stdxhtml "golang.org/x/net/html"
)

// markdownEscapeReplacer escapes characters that could break Markdown link/image syntax.

var markdownEscapeReplacer = strings.NewReplacer(
	`\`, `\\`,
	`[`, `\[`,
	`]`, `\]`,
	`|`, `\|`,
)

// containsASCIIFold reports whether substr is contained in s, case-insensitively

// (ASCII only). Both s and substr are folded to lowercase before comparison.

func containsASCIIFold(s, substr string) bool {
	substrLen := len(substr)
	sLen := len(s)
	if substrLen > sLen {
		return false
	}
	for i := 0; i <= sLen-substrLen; i++ {
		match := true
		for j := range substrLen {
			c := s[i+j]
			sc := substr[j]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if c != sc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// escapeMarkdownText escapes characters that could break Markdown link/image syntax.

// Escapes ], [, \, and | to prevent injection of arbitrary Markdown content;

// a raw '|' inside a label ends the cell when the construct is substituted

// into a rendered Markdown table (the table escaper sees only the

// pre-substitution [LINK:n] text, so it cannot cover this).

func escapeMarkdownText(s string) string {
	// Fast path: strings.(*Replacer).Replace allocates a new string even when
	// nothing matches, so scan first and skip the replacer entirely when the
	// text contains none of the four escapable characters. Link and image
	// text almost never contains them, so this avoids an allocation per call.
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '[', ']', '|':
			return markdownEscapeReplacer.Replace(s)
		}
	}
	return s
}

// collapseInlineSpace trims s and collapses internal whitespace runs (space,

// tab, newline, CR) to single spaces. A link label captured between block

// elements inside one anchor (an icon <div> followed by a text <span>, the

// common "icon-list-item" shape) otherwise carries newlines into the rendered

// [label](URL), splitting the construct across lines and breaking most

// renderers' inline parsing. Also makes whitespace-only labels empty, so the

// "Link N" fallback fires for them.

//
// Fast path: a label whose only whitespace is single internal spaces — no

// tab/newline/CR, no whitespace run, no leading/trailing whitespace — is

// already in collapsed form, so it is returned without the builder copy.

// formatInlineLinks calls this once per link label, and the common label

// shape ("link 42", "Read more") needs no rewriting.

func collapseInlineSpace(s string) string {
	if !strings.ContainsAny(s, " \t\n\r") {
		return s
	}
	// needsRewrite reports whether any whitespace in s would be changed by the
	// collapsing pass below: an edge whitespace byte, a whitespace run, or a
	// non-space whitespace byte.
	needsRewrite := false
	prevSpace := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ':
			if i == 0 || i == len(s)-1 || prevSpace {
				needsRewrite = true
			}
			prevSpace = true
			continue
		case '\t', '\n', '\r':
			needsRewrite = true
		}
		prevSpace = false
	}
	if !needsRewrite {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\n', '\r':
			inSpace = true
		default:
			if inSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			inSpace = false
			b.WriteByte(c)
		}
	}
	return b.String()
}

// markdownURLDestReplacer percent-encodes the bytes that would break a

// Markdown link/image destination.

var markdownURLDestReplacer = strings.NewReplacer(
	"(", "%28",
	")", "%29",
	" ", "%20",
	"|", "%7C",
)

// escapeMarkdownURLDest percent-encodes the parentheses, spaces, and pipes

// in a Markdown link/image destination. An unescaped ')' terminates the

// destination early and spills the rest of the URL into rendered text, and

// per CommonMark unescaped whitespace also terminates it — so a destination

// like "my file.png" rendered as broken text rather than a link. A raw '|'

// does not terminate the destination inline, but when the construct lands in

// a rendered Markdown table it terminates the CELL (the table escaper sees

// only the pre-substitution [LINK:n] text), shifting every following column.

// Percent-encoding — rather than backslash-escaping — keeps the destination

// a valid URL that survives copy-paste. Fast path: most destinations contain

// none of these bytes.

func escapeMarkdownURLDest(url string) string {
	if !strings.ContainsAny(url, "() |") {
		return url
	}
	return markdownURLDestReplacer.Replace(url)
}

func (p *Processor) extractTextContent(node *stdxhtml.Node, tableFormat string) string {
	// A pooled TrackedBuilder retains its capacity across calls (unlike a pooled
	// *strings.Builder, whose Reset nils the buffer), so the document-length text
	// is appended into a buffer that is already large enough after warmup — no
	// per-call growth allocations or the GC churn they cause.
	tb := internal.GetTrackedBuilder()
	internal.ExtractTextWithStructureAndImages(node, tb, nil, nil, tableFormat)
	// CleanTextBytes reads the builder's buffer directly, skipping the
	// tb.String() intermediate copy the string entry point would make (the
	// returned string is a fresh allocation and never aliases the pooled buffer).
	result := internal.CleanTextBytes(tb.Bytes())
	internal.PutTrackedBuilder(tb)
	return result
}

// writeMarkdownInline writes a Markdown inline link — or, when image is true,

// the image form of the same construct (a leading '!') — escaping the label

// and destination exactly as both inline formatters require. It is the single

// place those rules live: images and links previously assembled the identical

// construct in two different ways, so an escaping fix could land in one and

// miss the other.

func writeMarkdownInline(tb *table.TrackedBuilder, image bool, label, url string) {
	if image {
		_ = tb.WriteByte('!')
	}
	_ = tb.WriteByte('[')
	if image {
		_, _ = tb.WriteString(escapeMarkdownText(label))
	} else {
		// A link label may still carry [IMAGE:n] placeholders (links are
		// substituted before images); they must survive verbatim so the image
		// pass can nest real image markdown inside the construct —
		// [text ![alt](img)](url), the linked-image idiom.
		_, _ = tb.WriteString(escapeMarkdownLinkLabel(label))
	}
	_, _ = tb.WriteString("](")
	_, _ = tb.WriteString(escapeMarkdownURLDest(url))
	_ = tb.WriteByte(')')
}

// escapeMarkdownLinkLabel escapes a link label exactly like escapeMarkdownText

// except that [IMAGE:n] placeholders pass through verbatim (see

// writeMarkdownInline): escaping them would turn the later nested image

// markdown into literal bracket text.

func escapeMarkdownLinkLabel(s string) string {
	if !strings.ContainsAny(s, `\[]|`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		if s[i] == '[' && strings.HasPrefix(s[i:], "[IMAGE:") {
			if end := imageTokenEnd(s, i); end > 0 {
				b.WriteString(s[i:end])
				i = end
				continue
			}
		}
		switch s[i] {
		case '\\', '[', ']', '|':
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// imageTokenEnd returns the index just past a well-formed [IMAGE:<digits>]

// token starting at i, or 0 when the span starting there is not one.

func imageTokenEnd(s string, i int) int {
	j := i + len("[IMAGE:")
	start := j
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j > start && j < len(s) && s[j] == ']' {
		return j + 1
	}
	return 0
}

// normalizeLinkLabelText reshapes a link label that contains image

// placeholders (links are substituted before images):

//   - decorative images (empty alt) are dropped — a menu icon carries nothing

//     a text consumer needs;

//   - a label left with images only (a logo link) becomes the non-decorative

//     images' alt texts joined by spaces — the accessible-name convention;

//   - a mixed label keeps its text plus the non-decorative image placeholders,

//     so the image pass can nest real image markdown inside the construct —

//     whichever order the text and images appear in;

//   - a well-formed marker that matches no image is page text that looks like

//     a token; it stays literally (the same policy as an unpaired [LINK:n]

//     marker) instead of being deleted only on image-bearing pages.

//
// The result is whitespace-collapsed; a label reduced to nothing keeps the

// caller's "Link N" fallback intact (empty return).

func normalizeLinkLabelText(label string, images []ImageInfo) string {
	if !strings.Contains(label, "[IMAGE:") || len(images) == 0 {
		return label
	}
	var b strings.Builder
	b.Grow(len(label))
	hasText := false
	var alts []string
	i := 0
	for i < len(label) {
		if label[i] == '[' && strings.HasPrefix(label[i:], "[IMAGE:") {
			if end := imageTokenEnd(label, i); end > 0 {
				span := label[i:end]
				if pos, err := strconv.Atoi(span[len("[IMAGE:") : len(span)-1]); err == nil {
					if img, ok := imageByPosition(images, pos); ok {
						// Decorative images (empty alt) are dropped. Non-decorative
						// ones keep their token unconditionally: a mixed label
						// keeps it whether the text was seen before OR after the
						// token (the image pass nests its markdown inside the
						// construct), and the images-only case below returns the
						// alt-text join and discards b wholesale — so an icon that
						// precedes the label text is not silently lost.
						if alt := strings.TrimSpace(img.Alt); alt != "" {
							alts = append(alts, alt)
							b.WriteString(span)
						}
						i = end
						continue
					}
				}
				// A well-formed marker that resolves to no image is page
				// text that looks like a token: emit it literally rather than
				// deleting the words — image-less labels keep such text
				// verbatim (early return above), so image-bearing ones must
				// too.
				b.WriteString(span)
				hasText = true
				i = end
				continue
			}
		}
		if c := label[i]; c != ' ' && c != '\t' {
			hasText = true
		}
		b.WriteByte(label[i])
		i++
	}
	if !hasText && len(alts) > 0 {
		return strings.Join(alts, " ")
	}
	return collapseInlineSpace(b.String())
}

// imageByPosition looks an image up by its [IMAGE:n] position. Positions are

// assigned sequentially (1, 2, …) during collection, so the common case is a

// direct index; the linear fallback keeps the lookup correct for any input,

// including hand-built slices the tests use.

func imageByPosition(images []ImageInfo, pos int) (ImageInfo, bool) {
	if pos >= 1 && pos <= len(images) && images[pos-1].Position == pos {
		return images[pos-1], true
	}
	for i := range images {
		if images[i].Position == pos {
			return images[i], true
		}
	}
	return ImageInfo{}, false
}

// linkByPosition looks a link up by its [LINK:n] position. Collector-produced

// slices carry strictly ascending positions (invalid-href anchors consume a

// number without being appended, so positions may skip but never repeat), so

// the ascending case is a binary search with no per-call map allocation; a

// non-ascending (hand-built) slice falls back to a linear scan. Position 0 was

// never inserted into the map this replaces and stays not-found.

func linkByPosition(links []LinkInfo, pos int) (LinkInfo, bool) {
	if pos <= 0 {
		return LinkInfo{}, false
	}
	ascending := true
	for i := 1; i < len(links); i++ {
		if links[i-1].Position >= links[i].Position {
			ascending = false
			break
		}
	}
	if !ascending {
		// Last-wins, matching the map this replaces (a later duplicate
		// overwrote the earlier entry). Well-formed lists have no duplicates.
		var found LinkInfo
		ok := false
		for i := range links {
			if links[i].Position == pos {
				found = links[i]
				ok = true
			}
		}
		return found, ok
	}
	lo, hi := 0, len(links)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if links[mid].Position < pos {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(links) && links[lo].Position == pos {
		return links[lo], true
	}
	return LinkInfo{}, false
}

// writeHTMLAttr writes a quoted HTML attribute, escaping the value so it

// cannot break out of the attribute. Both inline HTML formatters emit

// attributes this way.

func writeHTMLAttr(tb *table.TrackedBuilder, name, value string) {
	_ = tb.WriteByte(' ')
	_, _ = tb.WriteString(name)
	_, _ = tb.WriteString(`="`)
	_, _ = tb.WriteString(htmlstd.EscapeString(value))
	_ = tb.WriteByte('"')
}

// writeHTMLAnchor writes an inline HTML anchor around text.

func writeHTMLAnchor(tb *table.TrackedBuilder, href, title, text string) {
	_, _ = tb.WriteString("<a")
	writeHTMLAttr(tb, "href", href)
	if title != "" {
		writeHTMLAttr(tb, "title", title)
	}
	_, _ = tb.WriteString(">")
	_, _ = tb.WriteString(htmlstd.EscapeString(text))
	_, _ = tb.WriteString("</a>")
}

// writeHTMLImg writes the inline HTML <img> element for img: the src/alt pair

// plus the advisory title and the dimensional attributes when present. It is

// the single spelling of the HTML image form (previously inlined in

// formatInlineImages' replacement builder).

func writeHTMLImg(tb *table.TrackedBuilder, img ImageInfo) {
	_, _ = tb.WriteString("<img")
	writeHTMLAttr(tb, "src", img.URL)
	writeHTMLAttr(tb, "alt", img.Alt)
	// The advisory title is a native HTML attribute; ImageInfo.Title was
	// collected but never rendered in either inline format.
	if img.Title != "" {
		writeHTMLAttr(tb, "title", img.Title)
	}
	if img.Width != "" {
		writeHTMLAttr(tb, "width", img.Width)
	}
	if img.Height != "" {
		writeHTMLAttr(tb, "height", img.Height)
	}
	_, _ = tb.WriteString(">")
}

// formatInlineImages substitutes [IMAGE:n] placeholders in textWithPlaceholders

// with formatted images. It runs AFTER formatInlineLinks (whose output may

// still carry [IMAGE:n] tokens inside link labels — there they become nested

// image markdown). A linear scan replaces each token directly into a pooled

// TrackedBuilder — the same shape formatInlineLinks uses — instead of the

// strings.NewReplacer previously built per call: the Replacer constructs a

// replace trie from the replacement list on every extraction, which the

// profiler attributed to this path. A token whose position matches no collected

// image (page text that looks like a token) is written verbatim, exactly as it

// survived the Replacer form; an unrecognized format also leaves the text

// unchanged (every token falls to the verbatim branch).

func (p *Processor) formatInlineImages(textWithPlaceholders string, images []ImageInfo, format string) string {
	if len(images) == 0 || format == "placeholder" || format == "none" {
		return textWithPlaceholders
	}

	// Fast check: any [IMAGE: present? (mirrors formatInlineLinks' gate)
	if !strings.Contains(textWithPlaceholders, "[IMAGE:") {
		return textWithPlaceholders
	}

	tb := internal.GetTrackedBuilder()
	defer internal.PutTrackedBuilder(tb)
	tb.Grow(len(textWithPlaceholders))

	i := 0
	n := len(textWithPlaceholders)
	for i < n {
		if textWithPlaceholders[i] == '[' && i+7 <= n && textWithPlaceholders[i:i+7] == "[IMAGE:" {
			j := i + 7
			numStart := j
			for j < n && textWithPlaceholders[j] >= '0' && textWithPlaceholders[j] <= '9' {
				j++
			}
			if j > numStart && j < n && textWithPlaceholders[j] == ']' {
				position, err := strconv.Atoi(textWithPlaceholders[numStart:j])
				if err == nil {
					if img, ok := imageByPosition(images, position); ok {
						switch format {
						case "markdown":
							// Collapse like link labels (see collapseInlineSpace): a
							// multiline or space-padded alt breaks the inline markdown
							// construct exactly as a newline-bearing label would.
							altText := collapseInlineSpace(img.Alt)
							if altText == "" {
								altText = "Image " + strconv.Itoa(img.Position)
							}
							writeMarkdownInline(tb, true, altText, img.URL)
						case "html":
							writeHTMLImg(tb, img)
						default:
							// Unknown format: keep the token (matches the Replacer
							// form, which had no replacement pair for it).
							_, _ = tb.WriteString(textWithPlaceholders[i : j+1])
						}
						j++ // skip ']'
						i = j
						continue
					}
				}
			}
			// Not a well-formed, resolvable token: write the '[' and resume
			// scanning after it so the rest of the text is preserved.
			_ = tb.WriteByte(textWithPlaceholders[i])
			i++
			continue
		}
		_ = tb.WriteByte(textWithPlaceholders[i])
		i++
	}
	return tb.String()
}

// formatInlineLinks substitutes [LINK:n]…[/LINK] placeholders. It runs BEFORE

// formatInlineImages (its output may still carry [IMAGE:n] tokens — inside

// link labels they become nested image markdown on the image pass), and

// images is the section's image list: labels holding only images take their

// alt texts (a logo link), and decorative (empty-alt) images are dropped

// from labels (a menu icon). See normalizeLinkLabelText.

func (p *Processor) formatInlineLinks(textWithPlaceholders string, links []LinkInfo, images []ImageInfo, format string) string {
	// No len(links) guard here: a document whose every anchor carries an
	// invalid href still emits [LINK:n] tokens (each <a> consumes a
	// position), and returning early on the empty list would leak them into
	// the output. The contains-token fast check below covers the common
	// no-token case, and unknown positions render their label text.
	if format == "none" {
		return textWithPlaceholders
	}

	// Fast check: any [LINK: present?
	if !strings.Contains(textWithPlaceholders, "[LINK:") {
		return textWithPlaceholders
	}

	// Position lookups go through linkByPosition (binary search over the
	// collector's strictly ascending positions) instead of the map this loop
	// used to build per call — one fewer allocation per formatted section on
	// the reader-mode/link-format path.
	//
	// Direct string scanning to avoid regex overhead. The TrackedBuilder
	// retains its backing array across pool round-trips, unlike the
	// *strings.Builder previously pooled here whose Reset() nils the buffer
	// (see pool.go).
	tb := internal.GetTrackedBuilder()
	defer internal.PutTrackedBuilder(tb)
	tb.Grow(len(textWithPlaceholders))

	i := 0
	n := len(textWithPlaceholders)
	// noEndTagFrom is the smallest index from which a scan for the closing
	// [/LINK] token is known to have failed: no closing token exists at or
	// after it. Closing-tag scans only ever start further right, so one
	// failed scan settles every later one. Without this floor, text holding
	// many unmatched "[LINK:n]" tokens rescanned the whole tail once per
	// token, making replacement quadratic (measured 128ms at 8k tokens on a
	// 64KB document, ~31s of CPU at 1MB) while pinning the withTimeout
	// goroutine slot for the duration.
	noEndTagFrom := n + 1
	for i < n {
		// Look for [LINK:
		if textWithPlaceholders[i] == '[' && i+6 <= n && textWithPlaceholders[i:i+6] == "[LINK:" {
			// Parse position number
			j := i + 6
			numStart := j
			for j < n && textWithPlaceholders[j] >= '0' && textWithPlaceholders[j] <= '9' {
				j++
			}
			if j > numStart && j < n && textWithPlaceholders[j] == ']' {
				position, err := strconv.Atoi(textWithPlaceholders[numStart:j])
				if err == nil {
					j++ // skip ']'
					// Find [/LINK]. The textStart < noEndTagFrom guard skips
					// scans that a previous failure already proved futile,
					// keeping the whole pass linear.
					textStart := j
					endTag := "[/LINK]"
					found := false
					for textStart < noEndTagFrom && j <= n-len(endTag) {
						if textWithPlaceholders[j] == '[' && textWithPlaceholders[j:j+7] == endTag {
							// Found the closing tag. Reshape the label: collapse
							// whitespace (block children inside the anchor put
							// newlines here), then apply the image rules —
							// decorative icons dropped, images-only labels
							// become their alt texts.
							linkText := normalizeLinkLabelText(collapseInlineSpace(textWithPlaceholders[textStart:j]), images)

							link, ok := linkByPosition(links, position)
							if !ok {
								// Unknown position, output just the text
								_, _ = tb.WriteString(linkText)
							} else {
								if linkText == "" {
									linkText = "Link " + strconv.Itoa(position)
								}
								switch format {
								case "markdown":
									writeMarkdownInline(tb, false, linkText, link.URL)
								case "html":
									writeHTMLAnchor(tb, link.URL, link.Title, linkText)
								default:
									_, _ = tb.WriteString(linkText)
								}
							}
							j += len(endTag)
							found = true
							break
						}
						j++
					}
					if !found && textStart < noEndTagFrom {
						// This scan ran to the end without a match: no closing
						// token exists at or after textStart.
						noEndTagFrom = textStart
					}
					if !found {
						// No closing [/LINK] tag present: write the opening tag
						// literally and resume scanning from textStart so that
						// trailing content is preserved rather than dropped.
						_, _ = tb.WriteString(textWithPlaceholders[i:textStart])
						j = textStart
					}
					i = j
					continue
				}
			}
			// Failed to parse, write as-is
			_ = tb.WriteByte(textWithPlaceholders[i])
			i++
		} else {
			_ = tb.WriteByte(textWithPlaceholders[i])
			i++
		}
	}

	return tb.String()
}
