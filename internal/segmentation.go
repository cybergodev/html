// segmentation.go partitions a sanitized document into labeled page regions
// (header / navigation / main / aside / footer) for the reader-mode API. It is
// the structural inverse of article extraction: instead of selecting one main
// content node and discarding chrome, it labels every region so the caller can
// keep or drop each by kind.
package internal

import (
	"strings"

	"golang.org/x/net/html"
)

// Section kind values. These are the strings behind the root package's
// SectionKind constants; they appear in ReadResult JSON as-is.
const (
	SectionHeader     = "header"
	SectionNavigation = "navigation"
	SectionMain       = "main"
	SectionAside      = "aside"
	SectionFooter     = "footer"
)

// Residual-classification thresholds. A residual block whose text is mostly
// link text is navigation furniture (menus, tag clouds), not prose; the link
// count floor keeps a paragraph with two inline links out of the navigation
// bucket. Values mirror the spirit of navLinkTextThreshold in the scorer: nav
// bars are many short links.
const (
	residualNavMinLinks    = 3
	residualNavLinkDensity = 0.5
)

// PageSectionSpec is one discovered region of the page: the DOM node covering
// it, its kind (a Section* constant, or "" while the region is an unclassified
// residual), and whether it is page chrome (header/navigation/aside/footer) or
// primary content (main). Chrome regions are extracted with the raw-text-only
// skip policy (ExtractChromeTextWithStructure); main regions with the full
// content policy (CleanContentNode + ExtractTextWithStructureAndImages).
type PageSectionSpec struct {
	Node   *html.Node
	Kind   string
	Chrome bool
}

// SegmentPage partitions doc's <body> into ordered section specs covering the
// whole body: every body element ends up inside exactly one spec's subtree
// (spec subtrees are disjoint by construction). findArticle, which may be nil,
// locates the article node when no region is recognizable as main content (the
// div-soup fallback); the root package passes its extractArticleNode adapter
// so custom scorers keep working. Returns nil for a nil document or a body
// with no element children.
func SegmentPage(doc *html.Node, findArticle func(*html.Node) *html.Node) []PageSectionSpec {
	if doc == nil {
		return nil
	}
	root := FindElementByTag(doc, "body")
	if root == nil {
		root = doc
	}

	var specs []PageSectionSpec
	collectSections(root, &specs)
	if len(specs) == 0 {
		return nil
	}
	specs = classifyResiduals(root, specs, findArticle)
	if len(specs) == 0 {
		return nil
	}
	return specs
}

// collectSections walks container's element children in document order. A
// child that is itself a landmark becomes a spec and is not descended into (a
// page-level <header>'s nested <nav> belongs to the header section, and an
// <article> inside <main> is part of the main region). A child whose subtree
// contains no landmark is a residual spec covering the whole subtree (a
// token-classed such child is a landmark spec — the empty subtree is exactly
// the no-wrapper guard). Anything else is a layout wrapper and is descended
// into. Each non-semantic child pays exactly one containsLandmark subtree
// walk; the landmark facts it settles are consumed by both branches below.
func collectSections(container *html.Node, specs *[]PageSectionSpec) {
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if kind, chrome, ok := semanticSectionKind(child); ok {
			*specs = append(*specs, PageSectionSpec{Node: child, Kind: kind, Chrome: chrome})
			continue
		}
		if !containsLandmark(child) {
			if kind, chrome, ok := tokenSectionKind(child); ok {
				*specs = append(*specs, PageSectionSpec{Node: child, Kind: kind, Chrome: chrome})
				continue
			}
			*specs = append(*specs, PageSectionSpec{Node: child})
			continue
		}
		// Either a plain wrapper or a token-classed layout shell (a wrapper
		// such as div.main-content around a whole page layout carries the
		// token but is not the main region): descend either way.
		collectSections(child, specs)
	}
}

// semanticSectionKind classifies node by semantic tag name or ARIA role.
func semanticSectionKind(node *html.Node) (kind string, chrome bool, ok bool) {
	switch node.Data {
	case "nav":
		return SectionNavigation, true, true
	case "header":
		return SectionHeader, true, true
	case "footer":
		return SectionFooter, true, true
	case "aside":
		return SectionAside, true, true
	case "main", "article":
		return SectionMain, false, true
	}
	for _, attr := range node.Attr {
		if attr.Key != "role" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(attr.Val)) {
		case "navigation":
			return SectionNavigation, true, true
		case "banner":
			return SectionHeader, true, true
		case "contentinfo":
			return SectionFooter, true, true
		case "complementary":
			return SectionAside, true, true
		case "main", "article":
			return SectionMain, false, true
		}
	}
	return "", false, false
}

// chromeTokenKinds maps word-bounded class/id tokens to page-chrome kinds.
// Chrome tokens are consulted before main tokens: in a compound name such as
// "main-nav" both tokens match word-bounded, and the navigation reading is the
// right one. The vocabulary deliberately overlaps the DefaultScorer's
// RemovePatterns, which the same tokens already strip on the article path.
var chromeTokenKinds = [...]struct{ token, kind string }{
	{"nav", SectionNavigation},
	{"navigation", SectionNavigation},
	{"menu", SectionNavigation},
	{"breadcrumb", SectionNavigation},
	{"breadcrumbs", SectionNavigation},
	{"header", SectionHeader},
	{"masthead", SectionHeader},
	{"footer", SectionFooter},
	{"colophon", SectionFooter},
	{"sidebar", SectionAside},
	{"side-bar", SectionAside},
	{"aside", SectionAside},
}

// tokenSectionKind classifies node by word-bounded class/id tokens. The main
// tokens are primaryContentClassTokens (elements.go) — segmentation and
// article-mode primary-content detection deliberately share ONE list so the
// two modes cannot drift apart when a token is tuned for one of them; broad
// tokens such as "content" and "page" are excluded from that list precisely
// because they also name page-level chrome wrappers ("page-content",
// "content-header"). collectSections's containsLandmark check is the second
// line of defense — a token match whose subtree holds another landmark is a
// layout shell, not a region.
func tokenSectionKind(node *html.Node) (kind string, chrome bool, ok bool) {
	for _, attr := range node.Attr {
		if attr.Key != "class" && attr.Key != "id" {
			continue
		}
		lowerVal := strings.ToLower(attr.Val)
		for _, tk := range &chromeTokenKinds {
			if hasWordBoundary(lowerVal, tk.token, boundaryStandard) {
				return tk.kind, true, true
			}
		}
		for _, token := range &primaryContentClassTokens {
			if hasWordBoundary(lowerVal, token, boundaryStandard) {
				return SectionMain, false, true
			}
		}
	}
	return "", false, false
}

// containsLandmark reports whether any element in node's subtree, excluding
// node itself, is a landmark. A token match anywhere suffices: a token node
// that would itself be rejected as a wrapper only exists as a wrapper because
// a deeper landmark sits under it, and this same walk finds that one.
//
// Cost note: this is a subtree walk per candidate. It runs only for nodes on
// the wrapper chain between <body> and the landmarks (collectSections
// descends nowhere else), so the total is O(subtree × wrapper-depth) —
// shallow on real pages, and bounded by MaxDepth on hostile ones.
func containsLandmark(node *html.Node) bool {
	if node == nil {
		return false
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		found := false
		WalkNodes(child, func(n *html.Node) bool {
			if found {
				return false
			}
			if n.Type != html.ElementNode {
				return true
			}
			if _, _, ok := semanticSectionKind(n); ok {
				found = true
				return false
			}
			if _, _, ok := tokenSectionKind(n); ok {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// containsNode reports whether node is ancestor itself or one of its
// descendants, by walking the parent chain.
func containsNode(ancestor, node *html.Node) bool {
	for n := node; n != nil; n = n.Parent {
		if n == ancestor {
			return true
		}
	}
	return false
}

// classifyResiduals finalizes specs in place and returns the filtered slice:
//
//   - Residual roots the article-mode scorer would remove outright (ads,
//     social/related blocks, comments, cookie bars, hidden duplicates) are
//     dropped — reader mode wants them gone, exactly as article mode does.
//   - Remaining residuals get a kind: link-dense blocks (many links whose
//     text dominates the block) are navigation; blocks before the first main
//     region are header, after the last are footer, between two mains they
//     are content continuation; with no main anywhere, findArticle's node
//     (when its residual can be found) or failing that the largest residual
//     is promoted to main and the rest classified positionally around it.
func classifyResiduals(root *html.Node, specs []PageSectionSpec, findArticle func(*html.Node) *html.Node) []PageSectionSpec {
	kept := specs[:0]
	for _, sp := range specs {
		if sp.Kind == "" && ShouldRemoveElement(sp.Node) {
			continue
		}
		kept = append(kept, sp)
	}
	specs = kept

	hasMain := false
	for _, sp := range specs {
		if sp.Kind == SectionMain {
			hasMain = true
			break
		}
	}
	if !hasMain && findArticle != nil {
		if best := findArticle(root); best != nil {
			// Direction 1: the article node sits inside a residual — that
			// residual is the main region.
			for i := range specs {
				if specs[i].Kind == "" && containsNode(specs[i].Node, best) {
					specs[i].Kind = SectionMain
					hasMain = true
					break
				}
			}
			// Direction 2: the scorer widened the winner UPWARD
			// (climbToContentWrapper) or scored a wrapper element directly,
			// so best is an ANCESTOR spanning several residuals. Every
			// residual inside best is a fragment of the article: promote them
			// all. Without this, only the largest residual would be promoted
			// below, and the remaining fragments would be classified
			// positionally — typically as footer chrome, which then gets
			// chrome cleaning instead of content cleaning.
			if !hasMain {
				for i := range specs {
					if specs[i].Kind == "" && containsNode(best, specs[i].Node) {
						specs[i].Kind = SectionMain
						hasMain = true
					}
				}
			}
		}
	}
	if !hasMain {
		bestIdx, bestLen := -1, -1
		for i := range specs {
			if specs[i].Kind != "" {
				continue
			}
			if l := TextContentLength(specs[i].Node); l > bestLen {
				bestLen, bestIdx = l, i
			}
		}
		if bestIdx >= 0 && bestLen > 0 {
			specs[bestIdx].Kind = SectionMain
			hasMain = true
		}
	}

	firstMain, lastMain := -1, -1
	for i := range specs {
		if specs[i].Kind == SectionMain {
			if firstMain < 0 {
				firstMain = i
			}
			lastMain = i
		}
	}

	for i := range specs {
		if specs[i].Kind != "" {
			continue
		}
		sp := &specs[i]
		m := collectContentMetrics(sp.Node)
		if m.linkCount >= residualNavMinLinks && m.textLength > 0 &&
			float64(m.linkTextLength)/float64(m.textLength) > residualNavLinkDensity {
			sp.Kind, sp.Chrome = SectionNavigation, true
			continue
		}
		switch {
		case !hasMain || (firstMain >= 0 && i > firstMain && i < lastMain):
			// Either no main exists anywhere (this residual is the page's only
			// content) or the block sits between two main regions: content
			// continuation, not page furniture.
			sp.Kind = SectionMain
		case i < firstMain:
			sp.Kind, sp.Chrome = SectionHeader, true
		default:
			sp.Kind, sp.Chrome = SectionFooter, true
		}
	}
	return specs
}
