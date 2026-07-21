package html

import (
	"strings"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// appendUniqueVideoURLs appends each url that is a valid, not-yet-seen video URL
// to videos, recording it in seen. It centralizes the validate-and-deduplicate
// logic shared by the iframe, embed, and object raw-HTML extraction paths.
func appendUniqueVideoURLs(urls []string, seen map[string]bool, videos []VideoInfo) []VideoInfo {
	for _, url := range urls {
		if internal.IsValidURL(url) && internal.IsVideoURL(url) && !seen[url] {
			seen[url] = true
			videos = append(videos, VideoInfo{
				URL:  url,
				Type: internal.DetectVideoType(url),
			})
		}
	}
	return videos
}

func (p *Processor) extractVideos(node *stdxhtml.Node, htmlContent string, canContainMedia bool) []VideoInfo {
	var videos []VideoInfo
	var seen map[string]bool

	// ensureDedup lazily materializes the result slice and dedup map on the first
	// match. A nil map read is safe (returns false), so the DOM walk can test
	// !seen[url] before allocating; only an actual match pays for the slice and
	// map. On a no-media document — the common case, where canContainMedia is
	// false and the walk finds no <video>/<iframe>/<embed>/<object> — neither is
	// ever allocated. The result is normalized to a non-nil empty slice at the
	// end so callers (incl. JSON marshaling) see [] rather than null.
	ensureDedup := func() {
		if seen == nil {
			seen = make(map[string]bool, initialMapCap)
		}
		if videos == nil {
			videos = make([]VideoInfo, 0, initialSliceCap)
		}
	}

	// canContainMedia is computed once by the caller (extractFromDocument) and
	// shared with extractAudios: HasMediaReference scans the whole document, and
	// the video and audio gates evaluate the identical, content-only condition.
	// The raw-HTML scans below (iframe/embed/object attribute extraction and the
	// video-URL regex) can only yield a result when the content references a media
	// extension or a known embed host. Skip them entirely when it provably does not;
	// the DOM walk below still finds <video>/<iframe>/<embed>/<object> elements.

	// First, extract from the HTML content directly for iframe/embed/object tags
	// These may be removed by sanitization, so we parse them from raw HTML first.
	// All three share identical validate/dedup logic (appendUniqueVideoURLs).
	if canContainMedia {
		// These scans write to seen, so the map (and slice) must exist now.
		ensureDedup()
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "iframe", "src"), seen, videos)
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "embed", "src", "data"), seen, videos)
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "object", "data"), seen, videos)
	}

	// Then extract from the DOM tree (for video tags and any iframe/embed/object that survived sanitization)
	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if n.Type != stdxhtml.ElementNode {
			return true
		}

		switch n.Data {
		case "video":
			if video := p.parseVideoNode(n); video.URL != "" && !seen[video.URL] {
				ensureDedup()
				seen[video.URL] = true
				videos = append(videos, video)
			}

		case "iframe":
			if video := p.parseIframeNode(n); video.URL != "" && !seen[video.URL] {
				ensureDedup()
				seen[video.URL] = true
				videos = append(videos, video)
			}

		case "embed", "object":
			if video := p.parseEmbedNode(n); video.URL != "" && !seen[video.URL] {
				ensureDedup()
				seen[video.URL] = true
				videos = append(videos, video)
			}
		}
		return true
	})

	// Finally, use regex to find any video URLs in the HTML content
	if canContainMedia {
		matches := videoRegex.FindAllString(htmlContent, maxRegexMatches)
		for _, url := range matches {
			if internal.IsValidURL(url) && !seen[url] {
				seen[url] = true
				videos = append(videos, VideoInfo{
					URL:  url,
					Type: internal.DetectVideoType(url),
				})
			}
		}
	}

	// Preserve the non-nil-empty contract of the previous eager-make: callers and
	// JSON marshaling distinguish a nil slice (null) from an empty one ([]).
	// make([]VideoInfo, 0) is allocation-free (zerobase slice) on this path.
	if videos == nil {
		return make([]VideoInfo, 0)
	}
	return videos
}

func (p *Processor) parseVideoNode(n *stdxhtml.Node) VideoInfo {
	video := VideoInfo{}
	for _, attr := range n.Attr {
		switch attr.Key {
		case "src":
			if !internal.IsValidURL(attr.Val) {
				return VideoInfo{}
			}
			video.URL = attr.Val
		case "poster":
			video.Poster = attr.Val
		case "width":
			video.Width = attr.Val
		case "height":
			video.Height = attr.Val
		case "duration":
			video.Duration = attr.Val
		}
	}

	if video.URL == "" {
		video.URL, video.Type = p.findSourceURL(n)
	}

	if !internal.IsValidURL(video.URL) {
		return VideoInfo{}
	}

	return video
}

func (p *Processor) parseIframeNode(n *stdxhtml.Node) VideoInfo {
	for _, attr := range n.Attr {
		if attr.Key == "src" && internal.IsValidURL(attr.Val) && internal.IsVideoURL(attr.Val) {
			video := VideoInfo{URL: attr.Val, Type: "embed"}
			for _, a := range n.Attr {
				switch a.Key {
				case "width":
					video.Width = a.Val
				case "height":
					video.Height = a.Val
				}
			}
			return video
		}
	}
	return VideoInfo{}
}

func (p *Processor) parseEmbedNode(n *stdxhtml.Node) VideoInfo {
	for _, attr := range n.Attr {
		if (attr.Key == "src" || attr.Key == "data") && internal.IsValidURL(attr.Val) && internal.IsVideoURL(attr.Val) {
			video := VideoInfo{URL: attr.Val}
			for _, a := range n.Attr {
				switch a.Key {
				case "type":
					video.Type = a.Val
				case "width":
					video.Width = a.Val
				case "height":
					video.Height = a.Val
				}
			}
			return video
		}
	}
	return VideoInfo{}
}

func (p *Processor) extractAudios(node *stdxhtml.Node, htmlContent string, canContainMedia bool) []AudioInfo {
	var audios []AudioInfo
	var seen map[string]bool

	// ensureDedup lazily materializes the result slice and dedup map on the first
	// match (see extractVideos for the full rationale). A no-media document
	// allocates neither; the result is normalized to a non-nil empty slice below.
	ensureDedup := func() {
		if seen == nil {
			seen = make(map[string]bool, initialMapCap)
		}
		if audios == nil {
			audios = make([]AudioInfo, 0, initialSliceCap)
		}
	}

	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if n.Type == stdxhtml.ElementNode && n.Data == "audio" {
			if audio := p.parseAudioNode(n); audio.URL != "" && !seen[audio.URL] {
				ensureDedup()
				seen[audio.URL] = true
				audios = append(audios, audio)
			}
		}
		return true
	})

	// The audio-URL regex can only match when the content references a media
	// extension; skip it when it provably does not. The DOM walk above still finds
	// <audio>/<source> elements regardless of their URL extension.
	// canContainMedia is computed once by the caller and shared with extractVideos.
	if canContainMedia {
		// The regex writes to seen; ensure it (and audios) exist. If the DOM walk
		// above already matched, this is a no-op.
		ensureDedup()
		matches := audioRegex.FindAllString(htmlContent, maxRegexMatches)
		for _, url := range matches {
			if internal.IsValidURL(url) && !seen[url] {
				seen[url] = true
				audios = append(audios, AudioInfo{
					URL:  url,
					Type: internal.DetectAudioType(url),
				})
			}
		}
	}

	if audios == nil {
		return make([]AudioInfo, 0)
	}
	return audios
}

func (p *Processor) parseAudioNode(n *stdxhtml.Node) AudioInfo {
	audio := AudioInfo{}
	for _, attr := range n.Attr {
		switch attr.Key {
		case "src":
			if !internal.IsValidURL(attr.Val) {
				return AudioInfo{}
			}
			audio.URL = attr.Val
		case "duration":
			audio.Duration = attr.Val
		}
	}

	if audio.URL == "" {
		audio.URL, audio.Type = p.findSourceURL(n)
	}

	if !internal.IsValidURL(audio.URL) {
		return AudioInfo{}
	}

	return audio
}

func (p *Processor) findSourceURL(n *stdxhtml.Node) (url, mediaType string) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == stdxhtml.ElementNode && c.Data == "source" {
			var srcURL, srcType string
			for _, attr := range c.Attr {
				switch attr.Key {
				case "src":
					srcURL = attr.Val
				case "type":
					srcType = attr.Val
				}
			}
			if srcURL != "" {
				return srcURL, srcType
			}
		}
	}
	return "", ""
}

// extractTagAttributes extracts specified attributes from all occurrences of a tag in HTML content.
// This function operates on raw HTML strings before sanitization, allowing extraction from
// tags that might be removed during HTML sanitization (e.g., iframe, embed, object).
func (p *Processor) extractTagAttributes(htmlContent, tagName string, attrNames ...string) []string {
	results := make([]string, 0, extractTagCap)
	// Convert tag name to lowercase once for comparison
	lowerTag := "<" + strings.ToLower(tagName)

	pos := 0
	for pos < len(htmlContent) {
		// Find the next occurrence of the tag using case-insensitive search
		// We'll search in chunks to avoid converting the entire HTML to lowercase
		tagStart := findTagIgnoreCase(htmlContent[pos:], lowerTag)
		if tagStart == -1 {
			break
		}
		tagStart += pos

		// Verify it's a complete tag name (not a partial match)
		if tagStart+len(lowerTag) < len(htmlContent) {
			nextChar := htmlContent[tagStart+len(lowerTag)]
			// The tag name should be followed by whitespace, '>', or '/'
			if nextChar != ' ' && nextChar != '\t' && nextChar != '\n' &&
				nextChar != '\r' && nextChar != '>' && nextChar != '/' {
				pos = tagStart + len(lowerTag)
				continue
			}
		}

		// Find the end of the opening tag
		tagEnd := strings.IndexByte(htmlContent[tagStart:], '>')
		if tagEnd == -1 {
			break
		}
		tagEnd += tagStart + 1

		tagContent := htmlContent[tagStart:tagEnd]

		// Extract requested attributes from this tag
		for _, attrName := range attrNames {
			if value := extractAttributeValue(tagContent, attrName); value != "" {
				results = append(results, value)
			}
		}

		pos = tagEnd
	}

	return results
}

// findTagIgnoreCase performs case-insensitive tag search more efficiently
// by using a combination of Index for candidate positions and EqualFold for verification
func findTagIgnoreCase(html, lowerTag string) int {
	if len(lowerTag) == 0 || len(html) < len(lowerTag) {
		return -1
	}

	// Fast path: try exact match first (most common case)
	if idx := strings.Index(html, lowerTag); idx >= 0 {
		return idx
	}

	// For case-insensitive search, check positions where first character matches (case-insensitive)
	tagLen := len(lowerTag)
	firstChar := lowerTag[0]

	for i := 0; i <= len(html)-tagLen; i++ {
		c := html[i]
		// Quick ASCII case-insensitive check for first character
		cfc := c
		if cfc >= 'A' && cfc <= 'Z' {
			cfc += 32
		}
		if cfc != firstChar {
			continue
		}

		// Found potential match, verify with EqualFold for full case-insensitive comparison
		candidate := html[i : i+tagLen]
		if strings.EqualFold(candidate, lowerTag) {
			return i
		}
	}

	return -1
}

// extractAttributeValue extracts a single attribute value from a tag string.
// It handles quoted (single and double) and unquoted attribute values.
// The attribute name is matched case-insensitively (HTML attribute names are
// case-insensitive, and raw HTML may use any case).
func extractAttributeValue(tagContent, attrName string) string {
	// search is compared byte-for-byte against lowercased content bytes below,
	// so normalize it to lowercase once. Without this, an uppercase attrName
	// would never match (the scan lowercases content but not the search string).
	search := strings.ToLower(attrName) + "="
	searchLen := len(search)
	tagLen := len(tagContent)

	// Case-insensitive search for the attribute
	pos := 0
	for pos <= tagLen-searchLen {
		match := true
		for j := 0; j < searchLen; j++ {
			c := tagContent[pos+j]
			sc := search[j]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			if c != sc {
				match = false
				break
			}
		}
		if !match {
			pos++
			continue
		}

		// Verify we're matching a complete attribute name (not a substring)
		if pos > 0 {
			prevChar := tagContent[pos-1]
			if prevChar != ' ' && prevChar != '\t' && prevChar != '\n' && prevChar != '\r' {
				pos++
				continue
			}
		}

		valueStart := pos + searchLen

		// Skip whitespace after '='
		for valueStart < tagLen {
			c := tagContent[valueStart]
			if c != ' ' && c != '\t' {
				break
			}
			valueStart++
		}

		if valueStart >= tagLen {
			return ""
		}

		// Extract quoted or unquoted value
		switch tagContent[valueStart] {
		case '"', '\'':
			quote := tagContent[valueStart]
			valueStart++
			valueEnd := strings.IndexByte(tagContent[valueStart:], quote)
			if valueEnd == -1 {
				return strings.TrimSpace(tagContent[valueStart:])
			}
			return strings.TrimSpace(tagContent[valueStart : valueStart+valueEnd])
		default:
			valueEnd := valueStart
			for valueEnd < tagLen {
				c := tagContent[valueEnd]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' {
					break
				}
				valueEnd++
			}
			return strings.TrimSpace(tagContent[valueStart:valueEnd])
		}
	}

	return ""
}
