// extract_article.go holds article-body selection: candidate scoring,
// the article cluster expansion, and its tuning constants.

package html

import (
	"strings"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// minArticleSiblingChars is the minimum text length a sibling block must carry

// to be considered part of a split article. A block below this is a stray

// wrapper (an empty spacer div, a one-word divider) rather than a byline,

// author-note, or section that belongs with the article.

const minArticleSiblingChars = 40

// extractArticleNode returns the node representing the article body together

// with the extraction method: extractionMethodArticle when a candidate scored

// above zero, extractionMethodBody for the <body> fallback. The returned node

// is nil only for a nil document.

//
// For the built-in DefaultScorer, the winning candidate is widened by the

// scorer's score-guarded ancestor climb (climbToContentWrapper): real articles

// are frequently split into a headline block, a body block, and a byline block

// under wrappers, and the climb captures those sibling fragments whenever the

// wrapper scores at least as well as the fragment. Custom scorers keep the

// expandToArticleCluster sibling heuristic instead — they have no access to

// the fold's scores.

func (p *Processor) extractArticleNode(doc *stdxhtml.Node) (*stdxhtml.Node, string) {
	if doc == nil {
		return nil, extractionMethodBody
	}

	// Fast path: the built-in DefaultScorer can find the best article node from a
	// single bottom-up metrics pass (FindBestArticleNode), avoiding both the O(N²)
	// per-candidate subtree re-walk that Score+collectContentMetrics would repeat
	// for every non-inline element, and the map allocation that
	// ScoreArticleCandidates would require. Custom scorers lack that method and
	// fall back to the per-node Score() loop, whose behavior is unchanged.
	var bestNode *stdxhtml.Node
	isDefault := false
	hookTried := false
	switch s := p.scorer.(type) {
	case *internal.DefaultScorer:
		bestNode = s.FindBestArticleNode(doc)
		isDefault = true
	case *scorerAdapter:
		// Optional single-pass selection hook: a custom scorer implementing
		// ArticleFinder searches the tree itself, avoiding the per-element
		// Score enumeration below. A nil return is the scorer's "no article"
		// verdict and skips the enumeration too.
		if af, ok := s.external.(ArticleFinder); ok {
			hookTried = true
			if cn := af.FindArticleNode(contentNodeAdapter{doc}); cn != nil {
				if wrapped, ok := cn.(contentNodeAdapter); ok && wrapped.Node != nil {
					bestNode = wrapped.Node
				}
			}
		}
	}
	if bestNode == nil && !isDefault && !hookTried {
		// Enumeration fallback for a custom Scorer without the ArticleFinder
		// hook. The best candidate is tracked inline in walk order, so an
		// exact tie resolves first-wins — reproducible run to run, matching
		// FindBestArticleNode's candidateCollector.record. (The previous
		// map + SelectBestCandidate form resolved ties by map iteration
		// order, which Go randomizes per run.)
		bestScore := 0
		internal.WalkNodes(doc, func(n *stdxhtml.Node) bool {
			if n.Type == stdxhtml.ElementNode && !internal.IsInlineElement(n.Data) {
				if score := p.scorer.Score(n); score > bestScore {
					bestScore = score
					bestNode = n
				}
			}
			return true
		})
	}
	if bestNode == nil {
		return internal.FindElementByTag(doc, "body"), extractionMethodBody
	}
	if isDefault {
		// FindBestArticleNode already applied the score-guarded climb; running
		// expandToArticleCluster on top would re-widen on the unguarded sibling
		// heuristic and pull penalized chrome back in.
		return bestNode, extractionMethodArticle
	}
	return p.expandToArticleCluster(bestNode), extractionMethodArticle
}

// expandToArticleCluster widens bestNode to its parent when the article is split

// across sibling blocks under a content wrapper. It is deliberately conservative:

//
//   - A semantic container (<article>/<main>, role=article/main) already marks

//     the article boundary and is returned unchanged — the author drew the line.

//   - The parent must be a non-chrome element (ShouldRemoveElement false), not

//     nil, and not <body>/<html>. Blocks spread directly under <body> are not

//     expanded: there the highest-scoring node already tends to be <body> itself,

//     and when it is not, the competing sibling is far more likely unrelated

//     ("related content") than a split of the same article.

//   - At least one sibling must be a content-bearing, non-chrome, block-level

//     element (≥ minArticleSiblingChars of text).

//   - bestNode must still dominate the parent: its text is at least half the

//     parent's text. A parent with a sibling that rivals or exceeds bestNode is

//     more likely two separate sections than one split article.

//
// When the guards pass, the parent is returned and CleanContentNode (run by the

// caller) strips any remaining chrome among the siblings, so the full article

// survives. When any guard fails, bestNode is returned unchanged.

func (p *Processor) expandToArticleCluster(bestNode *stdxhtml.Node) *stdxhtml.Node {
	if bestNode == nil {
		return nil
	}

	// A semantic container already delimits the article.
	switch bestNode.Data {
	case "article", "main":
		return bestNode
	}
	for _, attr := range bestNode.Attr {
		if attr.Key == "role" {
			switch strings.ToLower(attr.Val) {
			case "main", "article":
				return bestNode
			}
		}
	}

	parent := bestNode.Parent
	if parent == nil || parent.Type != stdxhtml.ElementNode {
		return bestNode
	}
	if parent.Data == "body" || parent.Data == "html" {
		return bestNode
	}
	if internal.ShouldRemoveElement(parent) {
		return bestNode
	}

	// bestNode must still be the dominant block of the parent: otherwise the
	// siblings are competing sections, not a split of one article.
	if internal.TextContentLength(bestNode)*2 < internal.TextContentLength(parent) {
		return bestNode
	}

	// Find at least one content-bearing, non-chrome, block-level sibling.
	for sib := parent.FirstChild; sib != nil; sib = sib.NextSibling {
		if sib == bestNode || sib.Type != stdxhtml.ElementNode {
			continue
		}
		if internal.ShouldRemoveElement(sib) || internal.IsInlineElement(sib.Data) {
			continue
		}
		if internal.TextContentLength(sib) >= minArticleSiblingChars {
			return parent
		}
	}
	return bestNode
}
