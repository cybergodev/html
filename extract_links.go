// extract_links.go holds the single-pass image/link collector and its
// pooled scratch buffers, including the node parsers for both.

package html

import (
	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// imageScratchPool and linkScratchPool back extractImagesAndLinks' append

// scratch. Results are built in a pooled, capacity-retaining slice and copied

// out once at exact size: the previous fixed-cap preallocations

// (make([]LinkInfo, 0, 128) alone) cost ~9.5 KB per extraction on every

// document — link-sparse ones included — and was a profiled ~8% of allocation

// bytes. The pooled scratch amortizes growth to zero across calls, and the

// exact-size copy gives the returned slice no surplus capacity to carry into

// the Result (and into cloneResult's copies).

var (
	imageScratchPool = internal.NewSlicePool[ImageInfo](initialSliceCap)
	linkScratchPool  = internal.NewSlicePool[LinkInfo](initialSliceCap)
)

// extractImagesAndLinks collects images and/or links from node in a single

// document-order traversal, assigning positions with the same semantics the

// previous separate collectors used:

//   - image positions count only <img> with a valid src (parseImageNode accepts),

//     keeping them contiguous and in lockstep with the [IMAGE:n] placeholders the

//     text pass emits (which guards on imgHasValidSrc).

//   - link positions count every <a> (invalid-href anchors still consume a

//     number but are not appended), matching the [LINK:n] placeholders the text

//     pass emits.

//
// The two counters are independent, so one walk yields the same results as the

// previous two. Pass wantImages/wantLinks false to skip (and avoid allocating)

// a kind; the traversal runs at most once.

//
// chromeMode selects the skip policy of the reader-mode chrome sections: the

// walk prunes raw-text subtrees (IsRawTextElement) to mirror

// ExtractChromeTextWithStructure, the pass those sections' [LINK:n]/[IMAGE:n]

// tokens are emitted by. The default (content) mode prunes nothing: on the

// article path CleanContentNode has already removed every subtree the text

// pass would skip, so an unpruned walk observes the same nodes. Pass node

// itself is never pruned in either mode.

func (p *Processor) extractImagesAndLinks(node *stdxhtml.Node, wantImages, wantLinks, chromeMode bool) (images []ImageInfo, links []LinkInfo) {
	if !wantImages && !wantLinks {
		return nil, nil
	}

	var imgBuf *[]ImageInfo
	var linkBuf *[]LinkInfo
	if wantImages {
		imgBuf = imageScratchPool.Get()
		defer imageScratchPool.Put(imgBuf)
	}
	if wantLinks {
		linkBuf = linkScratchPool.Get()
		defer linkScratchPool.Put(linkBuf)
	}

	imgPos, linkPos := 0, 0
	// This collector observes exactly the same node set as the text pass
	// (extractTextWithStructure): unmatched [IMAGE:n]/[LINK:n] tokens would
	// otherwise leak into Text and their URLs would be dropped.
	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if chromeMode && n != node && n.Type == stdxhtml.ElementNode && internal.IsRawTextElement(n.Data) {
			return false
		}
		if n.Type != stdxhtml.ElementNode {
			return true
		}
		switch n.Data {
		case "img":
			if !wantImages {
				return true
			}
			img := p.parseImageNode(n, imgPos+1)
			if img.URL == "" {
				return true
			}
			imgPos++
			*imgBuf = append(*imgBuf, img)
		case "a":
			if !wantLinks {
				return true
			}
			linkPos++
			link := p.parseLinkNode(n)
			link.Position = linkPos
			if link.URL != "" {
				*linkBuf = append(*linkBuf, link)
			}
		}
		return true
	})

	// Copy out at exact size. make(..., 0) on an empty result keeps the
	// historical non-nil empty slice (marshals as [] rather than null).
	if wantImages {
		images = make([]ImageInfo, len(*imgBuf))
		copy(images, *imgBuf)
	}
	if wantLinks {
		links = make([]LinkInfo, len(*linkBuf))
		copy(links, *linkBuf)
	}
	return images, links
}

func (p *Processor) parseImageNode(n *stdxhtml.Node, position int) ImageInfo {
	img := ImageInfo{Position: position}

	for _, attr := range n.Attr {
		switch attr.Key {
		case "src":
			if !internal.IsValidURL(attr.Val) {
				return ImageInfo{}
			}
			img.URL = attr.Val
		case "alt":
			img.Alt = attr.Val
		case "title":
			img.Title = attr.Val
		case "width":
			img.Width = attr.Val
		case "height":
			img.Height = attr.Val
		}
	}

	if img.URL == "" {
		return ImageInfo{}
	}

	img.IsDecorative = img.Alt == ""
	return img
}

func (p *Processor) parseLinkNode(n *stdxhtml.Node) LinkInfo {
	link := LinkInfo{}

	for _, attr := range n.Attr {
		switch attr.Key {
		case "href":
			if !internal.IsValidURL(attr.Val) {
				return LinkInfo{}
			}
			link.URL = attr.Val
		case "title":
			link.Title = attr.Val
		case "rel":
			// Check for nofollow in rel attribute (case-insensitive, no allocation)
			if containsASCIIFold(attr.Val, "nofollow") {
				link.IsNoFollow = true
			}
		}
	}

	if link.URL == "" {
		return LinkInfo{}
	}

	link.Text = internal.GetTextContent(n)
	link.IsExternal = internal.IsExternalURL(link.URL)
	return link
}
