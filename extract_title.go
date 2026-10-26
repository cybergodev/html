// extract_title.go holds title resolution: the metadata/DOM ladder,
// site-name suffix stripping, and separator splitting.

package html

import (
	"strings"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// resolveTitle picks the document title. When metadata extraction is enabled

// and the document declares one, the metadata title (JSON-LD headline over

// og:title) wins: producers curate it to be suffix-free. Otherwise the DOM

// ladder applies: <title> → h1 inside the selected article → first document

// h1 → first h2.

//
// A metadata title is stripped of a site-name suffix only when the trailing

// segment exactly matches the declared site name (og:site_name / JSON-LD

// publisher). The length-heuristic stripper used for <title> is deliberately

// NOT applied here: metadata headlines legitimately contain " - " subtitles

// ("GDP Falls 0.3% - Worst Quarter Since 2020") that a short tail would

// falsely truncate.

func (p *Processor) resolveTitle(doc, contentNode *stdxhtml.Node, method string, meta internal.Metadata) string {
	if t := strings.TrimSpace(meta.Title); t != "" {
		return stripExactSiteNameSuffix(t, meta.SiteName)
	}
	return p.extractTitle(doc, contentNode, method)
}

// stripExactSiteNameSuffix removes a trailing " - Site" style segment from

// title only when that segment equals siteName (case-insensitive). Returns

// title unchanged when siteName is empty or no separator's trailing segment

// matches.

func stripExactSiteNameSuffix(title, siteName string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		return title
	}
	for _, sep := range titleSeparators {
		if before, after, ok := splitAtLastSeparator(title, sep); ok && strings.EqualFold(after, siteName) {
			return before
		}
	}
	return title
}

func (p *Processor) extractTitle(doc, contentNode *stdxhtml.Node, method string) string {
	if doc == nil {
		return ""
	}
	// Collect the first <title>, <h1>, and <h2> in a single document-order
	// traversal. The previous implementation issued up to three separate
	// FindElementByTag calls (one full tree walk each); documents lacking a
	// <title> or <h1> — common — paid for all three. The nil guards keep
	// the FIRST occurrence of each tag: without them the walk kept
	// overwriting the candidates, so the fallback title became the LAST
	// h1/h2 it saw.
	var titleNode, h1Node, h2Node *stdxhtml.Node
	internal.WalkNodes(doc, func(n *stdxhtml.Node) bool {
		if n.Type == stdxhtml.ElementNode {
			switch n.Data {
			case "title":
				if titleNode == nil {
					titleNode = n
				}
			case "h1":
				if h1Node == nil {
					h1Node = n
				}
			case "h2":
				if h2Node == nil {
					h2Node = n
				}
			}
		}
		// Returning false only prunes descent into this node's subtree — the
		// walk itself keeps draining its stack, so this is a pruning hint,
		// not a termination. Once all three candidates are located, every
		// remaining stacked node returns false too; the nil guards above keep
		// the first occurrence of each tag.
		return titleNode == nil || h1Node == nil || h2Node == nil
	})
	if titleNode != nil {
		if title := internal.GetTextContent(titleNode); title != "" {
			return p.stripSiteName(title)
		}
	}
	// An h1 inside the selected article is the article's own headline; the
	// first document h1 may instead belong to site chrome (a logo band, a
	// page masthead) that happens to precede the article in document order.
	if method == extractionMethodArticle && contentNode != nil {
		if h1 := internal.FindElementByTag(contentNode, "h1"); h1 != nil {
			if title := internal.GetTextContent(h1); title != "" {
				return title
			}
		}
	}
	if h1Node != nil {
		if title := internal.GetTextContent(h1Node); title != "" {
			return title
		}
	}
	if h2Node != nil {
		return internal.GetTextContent(h2Node)
	}
	return ""
}

// titleSeparators are the single-character delimiters that conventionally join a

// page title to its site name ("My Post - Example.com", "My Post | Site"). They

// are matched in order so a title using " - " strips at that delimiter before a

// later one is considered.

var titleSeparators = []string{" - ", " | ", " · ", " – ", " — ", " » ", " « "}

// splitAtLastSeparator splits title at the last occurrence of sep, returning

// the trimmed halves. ok is false when sep does not occur at a usable

// position — absent, at index 0 (no leading half), or with either trimmed

// half empty. It backs both title-suffix strippers so the separator scan

// exists exactly once: the exact-match and heuristic policies differ (see

// stripExactSiteNameSuffix vs stripSiteName), the split mechanics must not.

func splitAtLastSeparator(title, sep string) (before, after string, ok bool) {
	idx := strings.LastIndex(title, sep)
	if idx <= 0 {
		return "", "", false
	}
	before = strings.TrimSpace(title[:idx])
	after = strings.TrimSpace(title[idx+len(sep):])
	if before == "" || after == "" {
		return "", "", false
	}
	return before, after, true
}

// maxSiteNameSuffixLen is the longest trailing segment that stripSiteName will

// treat as a site name. A page's site name is a short proper noun ("Example

// News", "Acme Corp"); a genuinely long second part of a two-part title is not

// a site name and must stay intact.

const maxSiteNameSuffixLen = 30

// stripSiteName removes a trailing site name from a page title of the common

// "Title - Site", "Title | Site", or "Title · Site" form. It is deliberately

// conservative so it never mutilates a legitimate title:

//
//   - It only trims at the LAST occurrence of a separator, so an internal " - "

//     inside the title is preserved and only the trailing part is removed.

//   - The trailing part must be short (≤ maxSiteNameSuffixLen) and the leading

//     part non-trivial (≥ 3 chars), so a two-part title whose second part is

//     substantial is kept whole, and a bare "A - B" or "3 - 4" is untouched.

func (p *Processor) stripSiteName(title string) string {
	if title == "" {
		return ""
	}
	for _, sep := range titleSeparators {
		before, after, ok := splitAtLastSeparator(title, sep)
		if !ok {
			continue
		}
		if len(after) <= maxSiteNameSuffixLen && len(before) >= 3 {
			return before
		}
		return title // matched a separator but the tail is not a short site name
	}
	return title
}
