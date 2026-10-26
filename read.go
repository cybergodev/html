// read.go provides the reader-mode API: a full-page structured reading view
// (ReadPage) that keeps every page region — navigation, main content, aside,
// footer — as separately labeled sections with inline hyperlinks, instead of
// selecting one article and discarding the rest. Its primary use is producing
// a "clean page" to feed to LLMs and similar text-only consumers.
package html

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// SectionKind labels a page region in a ReadResult.
type SectionKind string

// Section kinds reported by ReadPage. Header, Navigation, Aside, and Footer
// are page chrome; Main is the primary content region (there may be more than
// one, e.g. an article followed by a comments block both classified as main).
const (
	// SectionHeader is the page header region: logo bands, mastheads, and
	// search bars preceding the main content. A header's nested <nav> menu is
	// part of the header section.
	SectionHeader SectionKind = "header"
	// SectionNavigation is a navigation region: menus, breadcrumb trails, and
	// link-dense residual blocks (tag clouds, "popular links").
	SectionNavigation SectionKind = "navigation"
	// SectionMain is the primary content region: <main>/<article> landmarks,
	// article-classed wrappers, or the scorer-selected article node on pages
	// without recognizable landmarks.
	SectionMain SectionKind = "main"
	// SectionAside is a secondary content region: sidebars and
	// complementary/aside landmarks.
	SectionAside SectionKind = "aside"
	// SectionFooter is the page footer region: colophons and legal/privacy
	// blocks after the main content.
	SectionFooter SectionKind = "footer"
)

// sectionKindLabels maps SectionKind values to the headings RenderText emits.
// A map rather than a switch: RenderText runs once per document, not per node.
var sectionKindLabels = map[SectionKind]string{
	SectionHeader:     "Header",
	SectionNavigation: "Navigation",
	SectionMain:       "Main Content",
	SectionAside:      "Aside",
	SectionFooter:     "Footer",
}

// PageSection is one labeled region of a ReadResult.
type PageSection struct {
	// Kind labels the region (header, navigation, main, aside, footer).
	Kind SectionKind `json:"kind"`
	// Text is the region's extracted text with inline hyperlinks preserved —
	// Config.InlineLinkFormat selects the style ("markdown" → [text](URL),
	// the default; "html" → <a href="...">text</a>; "none" is treated as
	// "markdown" in reader mode) — and images formatted per
	// Config.InlineImageFormat.
	Text string `json:"text"`
	// PlainText is the region's text WITHOUT inline links and images — the
	// same words (link labels included) minus the [text](URL)/<a> constructs
	// and image placeholders. It backs ReadResult.RenderText and the
	// ReadPageToMarkdownText family; Text backs Render and the
	// ReadPageToMarkdown family.
	PlainText string `json:"plain_text,omitempty"`
	// Links lists the region's <a> elements in document order with positions
	// local to the contributing region — adjacent same-kind sections are
	// merged without renumbering, so positions identify a link's source
	// region, not an offset into the merged Text. Empty when PreserveLinks is
	// false.
	Links []LinkInfo `json:"links,omitempty"`
	// Images lists the region's <img> elements in document order, positions
	// local to the contributing region (see Links). Empty when
	// PreserveImages is false; the linked form in Text still renders images
	// per Config.InlineImageFormat regardless of this field.
	Images []ImageInfo `json:"images,omitempty"`
	// WordCount is the number of words in PlainText (CJK-aware, like
	// Result.WordCount) — URLs from the linked form never count as words.
	WordCount int `json:"word_count"`
}

// ReadResult is the full-page reading view returned by ReadPage: the page's
// regions in document order, each labeled by kind, plus the document metadata.
// Unlike Result (which holds the selected article only), nothing is discarded
// by kind — callers decide which sections to keep, and Render / RenderText
// flatten them into one document with / without inline links.
type ReadResult struct {
	// Title is the document title, resolved with the same priority ladder as
	// Result.Title (metadata headline > <title> > article h1 > first h1/h2).
	Title string `json:"title"`
	// Author is from JSON-LD author or <meta name="author">, as in Result.
	Author string `json:"author,omitempty"`
	// PublishedTime is the publication timestamp, as in Result. The zero time
	// means unknown.
	PublishedTime time.Time `json:"published_time,omitzero"`
	// Description is the page summary, as in Result.
	Description string `json:"description,omitempty"`
	// SiteName is the publishing site's name, as in Result.
	SiteName string `json:"site_name,omitempty"`
	// CanonicalURL is the document's canonical location, as in Result.
	CanonicalURL string `json:"canonical_url,omitempty"`
	// Sections lists the page regions in document order. Adjacent regions of
	// the same kind are merged; regions that yield no text, links, or images
	// are dropped.
	Sections []PageSection `json:"sections"`
	// ProcessingTime is the wall-clock time spent on this extraction. It is
	// omitted from JSON.
	ProcessingTime time.Duration `json:"-"`
}

// Render flattens the sections into a single document WITH inline hyperlinks:
// the title (when known) as a top-level heading, then each section under a
// "# <Kind>" heading with its Text (links per Config.InlineLinkFormat, always
// preserved in reader mode). It is the linked one-string form to feed text
// consumers such as LLM prompts; the empty result renders as "". See
// RenderText for the link-free variant.
func (r *ReadResult) Render() string {
	return r.render(func(sec *PageSection) string { return sec.Text }, true)
}

// RenderText flattens the sections into a single document WITHOUT inline
// links or images: same section headings and words (link labels included) as
// Render, but plain. A section with no plain text of its own (an icon-only
// nav whose anchors carry no words) appears in Render — which can still show
// its links — but is omitted here, since plain text has nothing to show for
// it. It is the one-string form when the consumer wants prose only — no
// [text](URL) constructs; the empty result renders as "".
func (r *ReadResult) RenderText() string {
	return r.render(func(sec *PageSection) string { return sec.PlainText }, false)
}

// render is the shared flattener behind Render and RenderText: title as a
// top-level heading (when known), then each section under a "# <Kind>"
// heading with the text returned by pick. withLinks also treats a links-only
// section (text empty but links present) as renderable.
func (r *ReadResult) render(pick func(*PageSection) string, withLinks bool) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	if t := strings.TrimSpace(r.Title); t != "" {
		b.WriteString("# ")
		b.WriteString(t)
		b.WriteString("\n\n")
	}
	for i := range r.Sections {
		sec := &r.Sections[i]
		text := strings.TrimSpace(pick(sec))
		if text == "" && (!withLinks || len(sec.Links) == 0) {
			continue
		}
		label, ok := sectionKindLabels[sec.Kind]
		if !ok {
			label = string(sec.Kind)
		}
		b.WriteString("# ")
		b.WriteString(label)
		b.WriteString("\n\n")
		if text != "" {
			b.WriteString(text)
			b.WriteString("\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ReadPage extracts a full-page reading view from HTML bytes with automatic
// encoding detection: every region of the page (header, navigation, main
// content, aside, footer) is kept as a labeled section with its text and
// inline hyperlinks, unlike Extract which returns the selected article only.
//
// HTML, CSS, and JS markup is removed (sanitization strips script/style and
// dangerous constructs); what remains in each section's Text is content text
// plus hyperlinks. Hyperlinks are ALWAYS preserved — Config.InlineLinkFormat
// selects their style ("markdown" → [text](URL), the default, "html" →
// <a href="...">text</a>); "none" is treated as "markdown" in reader mode.
// Use ExtractText when link-free plain text is wanted.
//
//	cfg := html.DefaultConfig()          // links already render as [text](URL)
//	cfg.InlineLinkFormat = "html"        // or keep <a> anchors instead
//	processor, _ := html.New(cfg)
//	page, _ := processor.ReadPage(htmlBytes)
//	fmt.Println(page.Render())           // with links; RenderText() without
//
// Relative link and image URLs are resolved to absolute URLs when
// Config.ResolveRelativeURLs is true (the default), against Config.BaseURL or
// a base detected from the document (<base>, og:url, rel=canonical).
//
// Section cleaning is kind-aware: main regions get the article-grade cleaning
// (ads, hidden nodes, and nested navigation removed); chrome regions
// (header/navigation/aside/footer) keep their menus — a header's <nav> is its
// substance — and are stripped of hidden nodes only. "Hidden" there means the
// unambiguous signals: the hidden attribute, aria-hidden="true", an inline
// display:none/visibility:hidden style, and always-hidden utility classes
// (d-none, sr-only, invisible, ...). The bare "hidden"/"hide" class token and
// responsive-visibility classes ("hidden md:block", "xl:hidden") mark
// hover/click- or breakpoint-revealed menus (dropdown panels, drawers) and
// are kept — the reading view has no pointer or viewport to resolve them
// against, and their link data is precisely what ReadPage preserves. Residual
// blocks the article scorer would remove outright (ads, social, related,
// comments, cookie bars) are dropped entirely. A later chrome section whose
// whitespace-normalized text exactly duplicates an earlier section of the
// same kind (the same menu visibly rendered twice) is dropped; main sections
// are never deduplicated.
//
// ReadPage does not use the extraction cache (its result type differs from
// Result's); repeated reads reprocess the input.
//
// Returns the same errors as [Processor.Extract].
func (p *Processor) ReadPage(htmlBytes []byte) (*ReadResult, error) {
	return recoverPanic(func() (*ReadResult, error) {
		return p.readPageWithContext(context.Background(), htmlBytes)
	})
}

// ReadPageWithContext extracts a full-page reading view from HTML bytes with
// context support for cancellation. See [Processor.ReadPage] for the output
// contract; this variant additionally returns context.Canceled or
// context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ReadPageWithContext(ctx context.Context, htmlBytes []byte) (*ReadResult, error) {
	return recoverPanic(func() (*ReadResult, error) {
		return p.readPageWithContext(ctx, htmlBytes)
	})
}

// ReadPageFromFile extracts a full-page reading view from an HTML file with
// automatic encoding detection. The character encoding (Windows-1252, UTF-8,
// GBK, Shift_JIS, etc.) is detected from the file contents. See
// [Processor.ReadPage] for the output contract; use this when you have a file
// path instead of raw bytes.
//
// Returns the same errors as [Processor.ReadPage], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures (see Config.AllowedBaseDir to confine reads to a
// trusted directory).
func (p *Processor) ReadPageFromFile(filePath string) (*ReadResult, error) {
	return recoverPanic(func() (*ReadResult, error) {
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}
		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return p.ReadPage(data)
	})
}

// ReadPageFromFileWithContext extracts a full-page reading view from an HTML
// file with context support for cancellation. See
// [Processor.ReadPageFromFile]; this variant additionally returns
// context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ReadPageFromFileWithContext(ctx context.Context, filePath string) (*ReadResult, error) {
	return recoverPanic(func() (*ReadResult, error) {
		// Early cancellation check, matching ExtractFromFileWithContext: the
		// file read below is not cancellable, so observing ctx first keeps a
		// cancelled context and a file error from swapping precedence.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}
		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return p.ReadPageWithContext(ctx, data)
	})
}

// ReadPageToMarkdown extracts a full-page reading view and returns it as one
// Markdown document WITH inline hyperlinks: section headings plus links as
// [text](URL) and images as ![alt](URL), the ready-to-paste form for LLM
// prompts. It is the reader-mode counterpart of ExtractToMarkdown: the
// processor's inline formats are overridden with "markdown" for this call
// only. For the same document without links see ReadPageToMarkdownText.
// Thread-safe: runs on a transient processor, like the ExtractTo* family.
// Returns the same errors as [Processor.Extract].
func (p *Processor) ReadPageToMarkdown(htmlBytes []byte) (string, error) {
	return recoverPanic(func() (string, error) {
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		result, err := p.buildFormatProcessor("markdown", "markdown").ReadPage(htmlBytes)
		if err != nil {
			return "", err
		}
		return result.Render(), nil
	})
}

// ReadPageToMarkdownWithContext extracts a full-page reading view and returns
// it as one Markdown document with inline links, with context support for
// cancellation. See [Processor.ReadPageToMarkdown]; this variant additionally
// returns context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ReadPageToMarkdownWithContext(ctx context.Context, htmlBytes []byte) (string, error) {
	return recoverPanic(func() (string, error) {
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		result, err := p.buildFormatProcessor("markdown", "markdown").ReadPageWithContext(ctx, htmlBytes)
		if err != nil {
			return "", err
		}
		return result.Render(), nil
	})
}

// ReadPageToMarkdownText extracts a full-page reading view and returns it as
// one Markdown document WITHOUT inline links or images: the same section
// headings, paragraphs, lists, and tables, with link labels kept as plain
// words. Use it when the consumer wants prose only; ReadPageToMarkdown keeps
// the [text](URL) constructs. Thread-safe: runs on a transient processor.
// Returns the same errors as [Processor.Extract].
func (p *Processor) ReadPageToMarkdownText(htmlBytes []byte) (string, error) {
	return recoverPanic(func() (string, error) {
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		result, err := p.buildFormatProcessor("none", "none").ReadPage(htmlBytes)
		if err != nil {
			return "", err
		}
		return result.RenderText(), nil
	})
}

// ReadPageToMarkdownTextWithContext extracts a full-page reading view and
// returns it as one Markdown document without inline links, with context
// support for cancellation. See [Processor.ReadPageToMarkdownText]; this
// variant additionally returns context.Canceled or context.DeadlineExceeded
// when ctx is cancelled.
func (p *Processor) ReadPageToMarkdownTextWithContext(ctx context.Context, htmlBytes []byte) (string, error) {
	return recoverPanic(func() (string, error) {
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		result, err := p.buildFormatProcessor("none", "none").ReadPageWithContext(ctx, htmlBytes)
		if err != nil {
			return "", err
		}
		return result.RenderText(), nil
	})
}

// ReadPageFromFileToMarkdown extracts a full-page reading view from an HTML
// file and returns it as one Markdown document WITH inline hyperlinks
// ([text](URL), ![alt](URL)). The character encoding (Windows-1252, UTF-8,
// GBK, Shift_JIS, etc.) is detected from the file contents. See
// [Processor.ReadPageToMarkdown] for the output contract; use this when you
// have a file path instead of raw bytes. For the link-free form see
// ReadPageFromFileToMarkdownText.
//
// Returns the same errors as [Processor.ReadPage], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures (see Config.AllowedBaseDir to confine reads to a
// trusted directory).
func (p *Processor) ReadPageFromFileToMarkdown(filePath string) (string, error) {
	return recoverPanic(func() (string, error) {
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return "", err
		}
		return p.ReadPageToMarkdown(data)
	})
}

// ReadPageFromFileToMarkdownWithContext extracts a full-page reading view from
// an HTML file and returns it as one Markdown document with inline links,
// with context support for cancellation. See
// [Processor.ReadPageFromFileToMarkdown]; this variant additionally returns
// context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ReadPageFromFileToMarkdownWithContext(ctx context.Context, filePath string) (string, error) {
	return recoverPanic(func() (string, error) {
		// Early cancellation check, matching ExtractFromFileWithContext /
		// extractFromFileWithFormatsWithContext: without it, an already
		// cancelled ctx still read the whole (uncancellable) file before
		// failing inside the inner WithContext call — the same input returned
		// context.Canceled on one path and a *FileError on the other.
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return "", err
		}
		return p.ReadPageToMarkdownWithContext(ctx, data)
	})
}

// ReadPageFromFileToMarkdownText extracts a full-page reading view from an
// HTML file and returns it as one Markdown document WITHOUT inline links or
// images (link labels kept as plain words). The character encoding is
// detected from the file contents. See [Processor.ReadPageToMarkdownText]
// for the output contract; use this when you have a file path instead of raw
// bytes.
//
// Returns the same errors as [Processor.ReadPage], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures (see Config.AllowedBaseDir).
func (p *Processor) ReadPageFromFileToMarkdownText(filePath string) (string, error) {
	return recoverPanic(func() (string, error) {
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return "", err
		}
		return p.ReadPageToMarkdownText(data)
	})
}

// ReadPageFromFileToMarkdownTextWithContext extracts a full-page reading view
// from an HTML file and returns it as one Markdown document without inline
// links, with context support for cancellation. See
// [Processor.ReadPageFromFileToMarkdownText]; this variant additionally
// returns context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ReadPageFromFileToMarkdownTextWithContext(ctx context.Context, filePath string) (string, error) {
	return recoverPanic(func() (string, error) {
		// Early cancellation check, matching ExtractFromFileWithContext: the
		// file read below is not cancellable, so observing ctx first keeps a
		// cancelled context and a file error from swapping precedence.
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if p == nil || p.closed.Load() {
			return "", ErrProcessorClosed
		}
		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return "", err
		}
		return p.ReadPageToMarkdownTextWithContext(ctx, data)
	})
}

// readPageWithContext is the shared ReadPage pipeline: validation, encoding
// detection, and the timeout-wrapped processing stage. It mirrors
// extractCoreWithContext minus the cache (ReadPage does not cache), so the
// returned result is owned by the caller without cloning.
func (p *Processor) readPageWithContext(ctx context.Context, htmlBytes []byte) (*ReadResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := p.validateInput(htmlBytes, "ReadPage"); err != nil {
		return nil, err
	}

	startTime := time.Now()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	utf8String, err := p.detectEncoding(htmlBytes)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	result, err := runWithTimeout(p, ctx, func(deadlineCtx context.Context) (*ReadResult, error) {
		return p.readContentWithContext(deadlineCtx, utf8String)
	})
	if err != nil {
		p.stats.errorCount.Add(1)
		if p.audit != nil && errors.Is(err, ErrProcessingTimeout) {
			p.audit.RecordTimeout(p.config.ProcessingTimeout)
		}
		return nil, err
	}

	result.ProcessingTime = time.Since(startTime)
	p.stats.totalProcessTime.Add(int64(result.ProcessingTime))
	p.stats.totalProcessed.Add(1)
	return result, nil
}

// readContentWithContext parses and sanitizes the document, then segments it
// into labeled sections. It mirrors processContentWithContext's stage order
// (parse, depth validation, metadata, sanitization, cooperative cancellation
// checks between stages) with segmentation replacing article selection.
func (p *Processor) readContentWithContext(ctx context.Context, htmlContent string) (*ReadResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if p.isBlankContent(htmlContent) {
		// Sections is non-nil so the zero document marshals as
		// {"sections":[]} rather than null, matching the empty-slice JSON
		// convention of Result's media lists.
		return &ReadResult{Sections: []PageSection{}}, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	doc, err := stdxhtml.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidHTML, err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := p.validateDepthTraversal(doc, 0); err != nil {
		return nil, err
	}

	var meta internal.Metadata
	if p.config.ExtractMetadata {
		meta = internal.CollectMetadata(doc)
	}

	if p.config.EnableSanitization {
		if p.audit != nil && p.config.Audit.Enabled {
			internal.SanitizeDOMReader(doc, p.audit)
		} else {
			internal.SanitizeDOMReader(doc, internal.NoOpAuditRecorder{})
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return p.readFromDocument(doc, meta)
}

// readFromDocument segments the sanitized document and builds the ReadResult:
// per-section text with inline links (and images) formatted per config, links
// resolved against the page base, empty sections dropped, and adjacent
// same-kind sections merged.
func (p *Processor) readFromDocument(doc *stdxhtml.Node, meta internal.Metadata) (*ReadResult, error) {
	result := &ReadResult{}

	// Resolve the base once: sections' link/image URLs are resolved against it
	// before inline formatting, so the rendered [text](URL)/<a href> forms
	// carry absolute URLs — what a text-only consumer needs to act on a link.
	baseURL := p.config.BaseURL
	if p.config.ResolveRelativeURLs && baseURL == "" {
		baseURL = p.detectBaseURL(doc)
	}

	// The article-node fallback for pages whose regions are all div soup with
	// no landmarks: reuse the exact selection logic of the Extract path so
	// custom scorers and the listing heuristic keep working. A body fallback
	// (extractionMethodBody) is not a main-region signal; residuals are then
	// classified positionally around the largest one.
	findArticle := func(d *stdxhtml.Node) *stdxhtml.Node {
		if !p.config.ExtractArticle {
			return nil
		}
		if n, m := p.extractArticleNode(d); n != nil &&
			(m == extractionMethodArticle || m == extractionMethodListing) {
			return n
		}
		return nil
	}

	specs := internal.SegmentPage(doc, findArticle)

	// Title resolution prefers an h1 inside the main region, mirroring
	// extractFromDocument's contentNode preference.
	var mainNode *stdxhtml.Node
	for i := range specs {
		if specs[i].Kind == internal.SectionMain {
			mainNode = specs[i].Node
			break
		}
	}
	titleMethod := extractionMethodBody
	if mainNode != nil {
		titleMethod = extractionMethodArticle
	}
	result.Title = p.resolveTitle(doc, mainNode, titleMethod, meta)
	if p.config.ExtractMetadata {
		result.Author = meta.Author
		result.PublishedTime = parsePublishedTime(meta.PublishedTime)
		result.Description = meta.Description
		result.SiteName = meta.SiteName
		result.CanonicalURL = meta.CanonicalURL
	}

	imageFormat := p.imageFormat
	// Reader mode always preserves hyperlinks — they are the reading view's
	// contract, so Config.InlineLinkFormat only selects the STYLE ("markdown"
	// or "html") and its DefaultConfig value "none" (right for Extract, where
	// plain text is a legitimate choice) is treated as "markdown". Without
	// this, ReadPage's flat output would look exactly like
	// ReadPageToMarkdown's yet silently carry no links — a trap for callers.
	// ExtractText remains the API for link-free plain text.
	linkFormat := p.linkFormat
	if linkFormat == "none" {
		linkFormat = "markdown"
	}
	needsImageTokens := imageFormat != "none"
	needsLinkTokens := linkFormat != "none" // always true; kept as the invariant expression
	wantImages := p.config.PreserveImages || needsImageTokens
	wantLinks := p.config.PreserveLinks || needsLinkTokens

	sections := make([]PageSection, 0, len(specs))
	for i := range specs {
		spec := &specs[i]

		// A section root that is itself hidden is invisible content: drop the
		// whole section (cleaning never removes roots, so the check belongs
		// here). Chrome roots use the reader-mode hidden test, which drops the
		// always-hidden roots (d-none, the hidden attribute, inline hiding
		// styles) but keeps roots revealed only by breakpoint or interaction
		// (a bare-"hidden" nav drawer, a "hidden md:block" menu) — the reading
		// view has no viewport or pointer to resolve them against. Main roots
		// use the strict reader variant, so a token-classed main wrapper
		// collapsed with a bare "hidden" (an SPA shell) still drops whole
		// while its responsive form survives. Semantic <main>/<article> roots
		// are governed by the primary-container class exemption in both
		// variants: only the hidden attribute, aria-hidden, or an inline
		// hiding style ever hides them.
		if spec.Chrome {
			if internal.IsHiddenElementReader(spec.Node) {
				continue
			}
		} else if internal.IsHiddenElementReaderStrict(spec.Node) {
			continue
		}

		if spec.Chrome {
			internal.RemoveHiddenElementsReader(spec.Node)
		} else {
			internal.CleanContentNode(spec.Node)
		}

		images, links := p.extractImagesAndLinks(spec.Node, wantImages, wantLinks, spec.Chrome)
		// The ResolveRelativeURLs contract lives in resolveURLIfEnabled
		// (links.go); a hand-inlined gate here could drift from the rest of
		// the link family.
		for j := range images {
			images[j].URL = p.resolveURLIfEnabled(baseURL, images[j].URL)
		}
		for j := range links {
			links[j].URL = p.resolveURLIfEnabled(baseURL, links[j].URL)
		}

		// Link tokens are always emitted (reader mode preserves links), so the
		// token-bearing raw text yields BOTH renderings without a second DOM
		// walk: the linked form substitutes the tokens per linkFormat, the
		// plain form strips them (link labels kept, image placeholders
		// dropped). When image tokens are off, raw carries link tokens only.
		var imageCounter, linkCounter int
		var imgCounter, linkCounterPtr *int
		if needsImageTokens {
			imgCounter = &imageCounter
		}
		linkCounterPtr = &linkCounter // needsLinkTokens is invariantly true
		raw := p.sectionText(spec, imgCounter, linkCounterPtr)
		// Links before images (same order as the Extract path): labels carry
		// [IMAGE:n] placeholders through the link pass, then the image pass
		// nests their markdown inside the constructs. Re-normalize after the
		// substitution: a placeholder that collapses to nothing (an
		// empty-label anchor with no usable href) leaves its two flanking
		// spaces adjacent, and the plain path below re-cleans — the linked
		// form must not disagree with it. (Safe since the table renderer
		// stopped emitting column-width padding.)
		text := internal.CleanTextBytes([]byte(
			p.formatInlineImages(p.formatInlineLinks(raw, links, images, linkFormat), images, imageFormat)))
		plain := internal.CleanTextBytes([]byte(stripImageTokens(stripLinkTokens(raw))))

		if strings.TrimSpace(text) == "" && strings.TrimSpace(plain) == "" && len(links) == 0 && len(images) == 0 {
			continue
		}
		words, _ := p.countWords(plain)
		section := PageSection{
			Kind:      SectionKind(spec.Kind),
			Text:      text,
			PlainText: plain,
			WordCount: words,
		}
		if p.config.PreserveLinks {
			section.Links = links
		}
		if p.config.PreserveImages {
			section.Images = images
		}
		sections = append(sections, section)
	}

	// Dedup before merging: merge fuses adjacent same-kind sections first and
	// would bake the duplicate's text into the kept section, where a signature
	// match can no longer find it.
	result.Sections = mergeAdjacentSections(dedupChromeSections(sections))
	return result, nil
}

// dedupChromeSections drops later chrome sections (every kind but main) whose
// whitespace-normalized text exactly duplicates an earlier section of the
// same kind. Component-rendered pages sometimes carry two visible copies of
// the same menu — responsive duplicates distinguished only by external CSS
// the DOM cannot see (the common one-hidden-copy case is already handled by
// the hidden-element checks). For a text consumer the second copy is pure
// noise. Main sections are never deduplicated: repeated content is rare and
// dropping it is a data-loss risk chrome text never carries.
//
// Matching is exact after whitespace removal, so a re-wrapped copy of the
// same menu (block elements introducing different newlines) still matches,
// while near-duplicates (one copy with extra items or different labels) are
// intentionally kept. Sections whose stripped text is empty (icon-only menus
// whose anchors carry no text) never participate: their signature cannot
// distinguish different link sets, so conflating them would drop a distinct
// menu.
func dedupChromeSections(sections []PageSection) []PageSection {
	if len(sections) < 2 {
		return sections
	}
	var sb strings.Builder
	seen := make(map[string]struct{}, 4)
	kept := sections[:0]
	for _, s := range sections {
		if s.Kind != SectionMain && strings.Trim(s.Text, " \t\n\r") != "" {
			sb.Reset()
			sb.WriteString(string(s.Kind))
			sb.WriteByte(0)
			writeSpaceStripped(&sb, s.Text)
			sig := sb.String()
			if _, dup := seen[sig]; dup {
				continue
			}
			seen[sig] = struct{}{}
		}
		kept = append(kept, s)
	}
	return kept
}

// writeSpaceStripped appends text's bytes to sb with ASCII whitespace bytes
// removed, so the same words in the same order produce the same signature
// regardless of the line breaks intervening block wrappers introduced.
func writeSpaceStripped(sb *strings.Builder, text string) {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case ' ', '\t', '\n', '\r':
		default:
			sb.WriteByte(text[i])
		}
	}
}

// stripLinkTokens removes the [LINK:n] and [/LINK] placeholder markers the
// section text pass emits, keeping the link text between them — the exact,
// non-heuristic source of PageSection.PlainText (no markdown re-parsing of
// the linked form, which would misfire on link-shaped text inside code
// blocks). An opening marker with no matching [/LINK] is emitted literally,
// mirroring formatInlineLinks' resilience to page text that merely looks like
// a token.
func stripLinkTokens(text string) string {
	if !strings.Contains(text, "[LINK:") {
		return text
	}
	tb := internal.GetTrackedBuilder()
	defer internal.PutTrackedBuilder(tb)
	tb.Grow(len(text))
	const (
		open  = "[LINK:"
		close = "[/LINK]"
	)
	i, n := 0, len(text)
	for i < n {
		if text[i] == '[' && i+len(open) <= n && text[i:i+len(open)] == open {
			j := i + len(open)
			for j < n && text[j] >= '0' && text[j] <= '9' {
				j++
			}
			if j > i+len(open) && j < n && text[j] == ']' {
				if k := strings.Index(text[j+1:], close); k >= 0 {
					_, _ = tb.WriteString(text[j+1 : j+1+k])
					i = j + 1 + k + len(close)
					continue
				}
			}
			// Not a well-formed token: emit the marker literally and resume
			// after it so trailing content is preserved.
			_, _ = tb.WriteString(open)
			i += len(open)
			continue
		}
		_ = tb.WriteByte(text[i])
		i++
	}
	return tb.String()
}

// stripImageTokens removes [IMAGE:n] tokens from the section text so
// PlainText carries words only. Tokens are removed wherever they occur, not
// only on their own line: the table flattener inlines a cell's image token
// into a single-line row ("| … | [IMAGE:1] |"), and a link label can carry
// one mid-sentence, so a line-scoped stripper leaked literal tokens into
// plain output. A line left empty by token removal (the text pass emits each
// placeholder on its own line) is dropped; a line with no token is kept
// verbatim, and a malformed marker is emitted literally, mirroring
// stripLinkTokens' resilience to page text that merely looks like a token.
// Image tokens reach this stripper only when an inline image format is
// configured; with images off the text pass emits none.
func stripImageTokens(text string) string {
	if !strings.Contains(text, "[IMAGE:") {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, "[IMAGE:") {
			line = removeImageTokenSpans(line)
			if strings.TrimSpace(line) == "" {
				continue
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// removeImageTokenSpans strips well-formed [IMAGE:<digits>] spans from s,
// emitting a malformed marker literally so trailing content is preserved.
func removeImageTokenSpans(s string) string {
	tb := internal.GetTrackedBuilder()
	defer internal.PutTrackedBuilder(tb)
	tb.Grow(len(s))
	const open = "[IMAGE:"
	i, n := 0, len(s)
	for i < n {
		if s[i] == '[' && i+len(open) <= n && s[i:i+len(open)] == open {
			if end := imageTokenEnd(s, i); end > 0 {
				i = end
				continue
			}
			_, _ = tb.WriteString(open)
			i += len(open)
			continue
		}
		_ = tb.WriteByte(s[i])
		i++
	}
	return tb.String()
}

// sectionText extracts spec's text through the mode-appropriate walker. The
// pooled TrackedBuilder is obtained and returned here so both modes share the
// buffer-reuse discipline of the Extract path.
func (p *Processor) sectionText(spec *internal.PageSectionSpec, imageCounter, linkCounter *int) string {
	tb := internal.GetTrackedBuilder()
	if spec.Chrome {
		internal.ExtractChromeTextWithStructure(spec.Node, tb, imageCounter, linkCounter, p.config.TableFormat)
	} else {
		internal.ExtractTextWithStructureAndImages(spec.Node, tb, imageCounter, linkCounter, p.config.TableFormat)
	}
	text := internal.CleanTextBytes(tb.Bytes())
	internal.PutTrackedBuilder(tb)
	return text
}

// mergeAdjacentSections merges consecutive sections of the same kind into one
// (their text joined by a blank line, links concatenated, word counts
// summed), keeping document order. Multiple header or navigation regions are
// common (top bar + sidebar menu); one labeled block per kind run reads
// cleaner for text-only consumers.
func mergeAdjacentSections(sections []PageSection) []PageSection {
	if len(sections) == 0 {
		return sections
	}
	merged := sections[:0]
	for _, s := range sections {
		if n := len(merged); n > 0 && merged[n-1].Kind == s.Kind {
			prev := &merged[n-1]
			if prev.Text != "" && s.Text != "" {
				prev.Text += "\n\n"
			}
			prev.Text += s.Text
			if prev.PlainText != "" && s.PlainText != "" {
				prev.PlainText += "\n\n"
			}
			prev.PlainText += s.PlainText
			prev.WordCount += s.WordCount
			prev.Links = append(prev.Links, s.Links...)
			continue
		}
		merged = append(merged, s)
	}
	return merged
}

// ============================================================================
// Package-level Convenience Functions
// ============================================================================

// ReadPage extracts a full-page reading view from HTML bytes with automatic
// encoding detection. This is a convenience function that uses a pooled
// Processor for efficiency; for repeated reads, create a Processor directly.
// See [Processor.ReadPage] for the output contract.
//
// An optional Config can be provided to customize the read (InlineLinkFormat,
// BaseURL, ...). If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [Extract], plus ErrMultipleConfigs if more than
// one Config is provided and ErrInvalidConfig (wrapped in *ConfigError) if the
// supplied configuration is invalid.
func ReadPage(htmlBytes []byte, cfg ...Config) (*ReadResult, error) {
	return withConfig(cfg, func(p *Processor) (*ReadResult, error) {
		return p.ReadPage(htmlBytes)
	})
}

// ReadPageWithContext extracts a full-page reading view from HTML bytes with
// context support for cancellation. See [ReadPage] and
// [Processor.ReadPageWithContext].
func ReadPageWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (*ReadResult, error) {
	return withConfig(cfg, func(p *Processor) (*ReadResult, error) {
		return p.ReadPageWithContext(ctx, htmlBytes)
	})
}

// ReadPageFromFile extracts a full-page reading view from an HTML file with
// automatic encoding detection. This is a convenience function that uses a
// pooled Processor; see [Processor.ReadPageFromFile].
//
// An optional Config can be provided to customize the read (InlineLinkFormat,
// BaseURL, ...). If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ReadPage], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures (see Config.AllowedBaseDir).
func ReadPageFromFile(filePath string, cfg ...Config) (*ReadResult, error) {
	return withConfig(cfg, func(p *Processor) (*ReadResult, error) {
		return p.ReadPageFromFile(filePath)
	})
}

// ReadPageFromFileWithContext extracts a full-page reading view from an HTML
// file with context support for cancellation. See [ReadPageFromFile] and
// [Processor.ReadPageFromFileWithContext].
func ReadPageFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (*ReadResult, error) {
	return withConfig(cfg, func(p *Processor) (*ReadResult, error) {
		return p.ReadPageFromFileWithContext(ctx, filePath)
	})
}

// ReadPageToMarkdown extracts a full-page reading view and returns it as one
// Markdown document WITH inline links ([text](URL), images as ![alt](URL)) —
// the ready-to-paste form for LLM prompts. This is a convenience function
// that uses a pooled Processor; see [Processor.ReadPageToMarkdown]. For the
// link-free form see ReadPageToMarkdownText.
//
// An optional Config can be provided; if no config is provided, DefaultConfig()
// is used.
//
// Returns the same errors as [ReadPage].
func ReadPageToMarkdown(htmlBytes []byte, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageToMarkdown(htmlBytes)
	})
}

// ReadPageToMarkdownWithContext extracts a full-page reading view and returns
// it as one Markdown document with inline links, with context support for
// cancellation. See [ReadPageToMarkdown] and
// [Processor.ReadPageToMarkdownWithContext].
func ReadPageToMarkdownWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageToMarkdownWithContext(ctx, htmlBytes)
	})
}

// ReadPageToMarkdownText extracts a full-page reading view and returns it as
// one Markdown document WITHOUT inline links or images (link labels kept as
// plain words). This is a convenience function that uses a pooled Processor;
// see [Processor.ReadPageToMarkdownText].
//
// An optional Config can be provided; if no config is provided, DefaultConfig()
// is used.
//
// Returns the same errors as [ReadPage].
func ReadPageToMarkdownText(htmlBytes []byte, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageToMarkdownText(htmlBytes)
	})
}

// ReadPageToMarkdownTextWithContext extracts a full-page reading view and
// returns it as one Markdown document without inline links, with context
// support for cancellation. See [ReadPageToMarkdownText] and
// [Processor.ReadPageToMarkdownTextWithContext].
func ReadPageToMarkdownTextWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageToMarkdownTextWithContext(ctx, htmlBytes)
	})
}

// ReadPageFromFileToMarkdown extracts a full-page reading view from an HTML
// file and returns it as one Markdown document WITH inline links. This is a
// convenience function that uses a pooled Processor; see
// [Processor.ReadPageFromFileToMarkdown]. For the link-free form see
// ReadPageFromFileToMarkdownText.
//
// An optional Config can be provided; if no config is provided,
// DefaultConfig() is used.
//
// Returns the same errors as [ReadPage], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures (see Config.AllowedBaseDir).
func ReadPageFromFileToMarkdown(filePath string, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageFromFileToMarkdown(filePath)
	})
}

// ReadPageFromFileToMarkdownWithContext extracts a full-page reading view from
// an HTML file and returns it as one Markdown document with inline links,
// with context support for cancellation. See [ReadPageFromFileToMarkdown] and
// [Processor.ReadPageFromFileToMarkdownWithContext].
func ReadPageFromFileToMarkdownWithContext(ctx context.Context, filePath string, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageFromFileToMarkdownWithContext(ctx, filePath)
	})
}

// ReadPageFromFileToMarkdownText extracts a full-page reading view from an
// HTML file and returns it as one Markdown document WITHOUT inline links or
// images (link labels kept as plain words). This is a convenience function
// that uses a pooled Processor; see
// [Processor.ReadPageFromFileToMarkdownText].
//
// An optional Config can be provided; if no config is provided,
// DefaultConfig() is used.
//
// Returns the same errors as [ReadPage], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures (see Config.AllowedBaseDir).
func ReadPageFromFileToMarkdownText(filePath string, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageFromFileToMarkdownText(filePath)
	})
}

// ReadPageFromFileToMarkdownTextWithContext extracts a full-page reading view
// from an HTML file and returns it as one Markdown document without inline
// links, with context support for cancellation. See
// [ReadPageFromFileToMarkdownText] and
// [Processor.ReadPageFromFileToMarkdownTextWithContext].
func ReadPageFromFileToMarkdownTextWithContext(ctx context.Context, filePath string, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ReadPageFromFileToMarkdownTextWithContext(ctx, filePath)
	})
}
