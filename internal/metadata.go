// metadata.go collects structured metadata (JSON-LD, OpenGraph, standard
// <meta>, canonical link) from a parsed document. It must run BEFORE
// SanitizeDOM: the sanitizer strips <script> elements, which carry the JSON-LD
// payloads, and this pass is the only consumer that needs them.
package internal

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// maxJSONLDScriptBytes caps how much of a single ld+json script block is
// parsed. Metadata blocks are a few KB; a block larger than this is treated
// as unparsable rather than paying a full JSON decode on hostile input (the
// input size is already bounded by Config.MaxInputSize, this keeps the
// per-block parse cost bounded too).
const maxJSONLDScriptBytes = 1 << 20

// Metadata holds the structured-metadata values a document declares, with
// source priorities resolved. Empty fields mean the document declares no such
// value. PublishedTime stays a raw ISO-8601 string; the caller owns parsing
// because producers emit a wide range of layouts.
type Metadata struct {
	// Title is the metadata title: the article-typed JSON-LD headline over
	// og:title (a WebPage/WebSite node's name is the last resort — it names
	// the site, not the page). It is usually already free of site-name
	// suffixes.
	Title string
	// Author is from JSON-LD author (string, {name}, or an array of either)
	// over <meta name="author">.
	Author string
	// PublishedTime is from article:published_time over JSON-LD
	// datePublished, raw ISO-8601.
	PublishedTime string
	// Description is from og:description over JSON-LD description over
	// <meta name="description">.
	Description string
	// SiteName is from og:site_name over the JSON-LD publisher name (or the
	// name of a separate Organization/WebSite graph node).
	SiteName string
	// CanonicalURL is from <link rel="canonical"> over og:url.
	CanonicalURL string
}

// rawMetadata holds every source value before priority resolution. Each field
// is first-wins: the first matching element in document order wins, matching
// how consumers read duplicated og:/meta tags.
type rawMetadata struct {
	ogTitle          string
	ogDescription    string
	ogSiteName       string
	ogURL            string
	articlePublished string
	metaDescription  string
	metaAuthor       string
	canonicalLink    string

	// ld fields, filled by applyJSONLD with the same two-pass priority
	// (article-typed blocks first, then any block for remaining gaps).
	ldHeadline    string
	ldName        string
	ldAuthor      string
	ldPublished   string
	ldDescription string
	ldSiteName    string
}

// CollectMetadata walks doc once and returns the resolved metadata. It never
// fails: malformed JSON-LD blocks and missing elements simply leave fields
// empty.
func CollectMetadata(doc *html.Node) Metadata {
	if doc == nil {
		return Metadata{}
	}

	var raw rawMetadata
	var ldBlocks []string

	WalkNodes(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "script":
			if strings.EqualFold(strings.TrimSpace(attrVal(n, "type")), "application/ld+json") {
				if text := scriptText(n, maxJSONLDScriptBytes); text != "" {
					ldBlocks = append(ldBlocks, text)
				}
			}
		case "meta":
			content := strings.TrimSpace(attrVal(n, "content"))
			if content == "" {
				return true
			}
			// og:*/article:* come via property=; classic tags via name=.
			switch strings.ToLower(strings.TrimSpace(attrVal(n, "property"))) {
			case "og:title":
				setFirst(&raw.ogTitle, content)
			case "og:description":
				setFirst(&raw.ogDescription, content)
			case "og:site_name":
				setFirst(&raw.ogSiteName, content)
			case "og:url":
				setFirst(&raw.ogURL, content)
			case "article:published_time":
				setFirst(&raw.articlePublished, content)
			default:
				switch strings.ToLower(strings.TrimSpace(attrVal(n, "name"))) {
				case "description":
					setFirst(&raw.metaDescription, content)
				case "author":
					setFirst(&raw.metaAuthor, content)
				}
			}
		case "link":
			if strings.EqualFold(strings.TrimSpace(attrVal(n, "rel")), "canonical") {
				setFirst(&raw.canonicalLink, strings.TrimSpace(attrVal(n, "href")))
			}
		}
		return true
	})

	applyJSONLD(&raw, ldBlocks)
	return raw.resolve()
}

// attrVal returns the value of the first attribute named key (case-sensitive
// match on the key, matching how the parser reports well-formed markup).
func attrVal(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// scriptText concatenates the text children of a script element, reading at
// most limit bytes. Parsers put script bodies in a single text node; the loop
// keeps odd multi-node shapes working too.
func scriptText(n *html.Node, limit int) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.TextNode {
			continue
		}
		if b.Len()+len(c.Data) > limit {
			b.WriteString(c.Data[:limit-b.Len()])
			break
		}
		b.WriteString(c.Data)
	}
	return strings.TrimSpace(b.String())
}

// setFirst assigns value to dst only when dst is empty (first-wins).
func setFirst(dst *string, value string) {
	if *dst == "" {
		*dst = value
	}
}

// ldNode is the subset of schema.org fields this pass consumes. Unknown
// fields are ignored by encoding/json.
type ldNode struct {
	Type          string          `json:"@type"`
	Headline      string          `json:"headline"`
	Name          string          `json:"name"`
	Author        json.RawMessage `json:"author"`
	DatePublished string          `json:"datePublished"`
	Description   string          `json:"description"`
	Publisher     ldPublisher     `json:"publisher"`
	Graph         []ldNode        `json:"@graph"`
}

type ldPublisher struct {
	Name string `json:"name"`
}

// articleTypes are the schema.org types whose headline/author/datePublished
// describe an article rather than the page or site; a block of one of these
// types wins over a WebPage/WebSite block for the article-scoped fields.
var articleTypes = map[string]bool{
	"article":            true,
	"newsarticle":        true,
	"blogposting":        true,
	"socialmediaposting": true,
	"techarticle":        true,
	"report":             true,
	"scholarlyarticle":   true,
	"liveblogposting":    true,
}

// applyJSONLD parses the collected ld+json blocks into raw. Each block may be
// a single object, an array of objects, or an object with an embedded @graph
// array; all shapes are flattened. Two passes give article-typed nodes
// priority for the article-scoped fields, then any node fills the remaining
// gaps (WebPage name as a headline fallback, top-level Organization publisher).
func applyJSONLD(raw *rawMetadata, blocks []string) {
	var nodes []ldNode
	for _, block := range blocks {
		nodes = append(nodes, parseLDBlock(block)...)
	}

	for i := range nodes {
		if articleTypes[strings.ToLower(nodes[i].Type)] {
			fillFromLDNode(raw, nodes[i], true)
		}
	}
	for i := range nodes {
		if !articleTypes[strings.ToLower(nodes[i].Type)] {
			fillFromLDNode(raw, nodes[i], false)
		}
	}
}

// parseLDBlock decodes one ld+json block into the flattened node list, or
// returns nil for empty or malformed JSON.
func parseLDBlock(block string) []ldNode {
	trimmed := strings.TrimSpace(block)
	if trimmed == "" {
		return nil
	}
	var nodes []ldNode
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(block), &nodes); err != nil {
			return nil
		}
	} else {
		var node ldNode
		if err := json.Unmarshal([]byte(block), &node); err != nil {
			return nil
		}
		nodes = []ldNode{node}
	}

	// Flatten @graph payloads in place.
	var flat []ldNode
	for _, node := range nodes {
		flat = append(flat, node)
		flat = append(flat, node.Graph...)
	}
	return flat
}

// fillFromLDNode fills raw's ld fields from node, first-wins. A node without
// a headline falls back to its name only in the non-article pass: a WebPage's
// name is the page title, but an article block's name is often the CMS
// internal name, not the headline.
func fillFromLDNode(raw *rawMetadata, node ldNode, fromArticle bool) {
	if headline := strings.TrimSpace(node.Headline); headline != "" {
		setFirst(&raw.ldHeadline, headline)
	} else if !fromArticle {
		setFirst(&raw.ldName, strings.TrimSpace(node.Name))
	}
	setFirst(&raw.ldAuthor, authorFromJSON(node.Author))
	setFirst(&raw.ldPublished, strings.TrimSpace(node.DatePublished))
	setFirst(&raw.ldDescription, strings.TrimSpace(node.Description))
	setFirst(&raw.ldSiteName, strings.TrimSpace(node.Publisher.Name))
	// A separate Organization/WebSite node in the graph is the site-name
	// source when the article references its publisher only by @id (which
	// this pass does not resolve).
	if !fromArticle {
		switch strings.ToLower(node.Type) {
		case "organization", "website":
			setFirst(&raw.ldSiteName, strings.TrimSpace(node.Name))
		}
	}
}

// authorFromJSON extracts a display-ready author from the schema.org author
// property, whose value may be a plain string, an object with a name field
// ({"@type":"Person","name":"Jane"}), or an array mixing both. Returns "" for
// anything else.
func authorFromJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && strings.TrimSpace(obj.Name) != "" {
		return strings.TrimSpace(obj.Name)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, item := range arr {
			if name := authorFromJSON(item); name != "" {
				parts = append(parts, name)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// resolve folds the raw source values into the final Metadata with the
// documented source priorities.
func (raw *rawMetadata) resolve() Metadata {
	var m Metadata

	// Title: the article-typed JSON-LD headline first, then og:title, and the
	// non-article ldName (a WebPage/WebSite node's name) only as a last
	// resort — that name is usually the SITE's name, not the page's title,
	// so it must not beat the page-scoped og:title.
	if raw.ldHeadline != "" {
		m.Title = raw.ldHeadline
	} else if raw.ogTitle != "" {
		m.Title = raw.ogTitle
	} else {
		m.Title = raw.ldName
	}

	if raw.ldAuthor != "" {
		m.Author = raw.ldAuthor
	} else {
		m.Author = raw.metaAuthor
	}

	if raw.articlePublished != "" {
		m.PublishedTime = raw.articlePublished
	} else {
		m.PublishedTime = raw.ldPublished
	}

	switch {
	case raw.ogDescription != "":
		m.Description = raw.ogDescription
	case raw.ldDescription != "":
		m.Description = raw.ldDescription
	default:
		m.Description = raw.metaDescription
	}

	if raw.ogSiteName != "" {
		m.SiteName = raw.ogSiteName
	} else {
		m.SiteName = raw.ldSiteName
	}

	if raw.canonicalLink != "" {
		m.CanonicalURL = raw.canonicalLink
	} else {
		m.CanonicalURL = raw.ogURL
	}

	return m
}
