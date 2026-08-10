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

// extractVideos extracts video sources by delegating to extractAllMedia and
// discarding the audio result. This avoids maintaining a separate implementation
// alongside the unified extractAllMedia path — the three previously differed
// only in which media types they walked, not in how they walked them.
func (p *Processor) extractVideos(node *stdxhtml.Node, htmlContent string, canContainMedia bool) []VideoInfo {
	videos, _ := p.extractAllMedia(node, htmlContent, canContainMedia)
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
	var video VideoInfo
	foundSrc := false
	for _, attr := range n.Attr {
		switch attr.Key {
		case "src":
			if internal.IsValidURL(attr.Val) && internal.IsVideoURL(attr.Val) {
				video.URL = attr.Val
				video.Type = "embed"
				foundSrc = true
			}
		case "width":
			video.Width = attr.Val
		case "height":
			video.Height = attr.Val
		}
	}
	if !foundSrc {
		return VideoInfo{}
	}
	return video
}

func (p *Processor) parseEmbedNode(n *stdxhtml.Node) VideoInfo {
	var video VideoInfo
	foundMedia := false
	for _, attr := range n.Attr {
		switch attr.Key {
		case "src", "data":
			if internal.IsValidURL(attr.Val) && internal.IsVideoURL(attr.Val) {
				video.URL = attr.Val
				foundMedia = true
			}
		case "type":
			video.Type = attr.Val
		case "width":
			video.Width = attr.Val
		case "height":
			video.Height = attr.Val
		}
	}
	if !foundMedia {
		return VideoInfo{}
	}
	return video
}

// extractAudios extracts audio sources by delegating to extractAllMedia and
// discarding the video result. See extractVideos for the rationale.
func (p *Processor) extractAudios(node *stdxhtml.Node, htmlContent string, canContainMedia bool) []AudioInfo {
	_, audios := p.extractAllMedia(node, htmlContent, canContainMedia)
	return audios
}

// extractAllMedia extracts both videos and audios in a single DOM walk. When both
// PreserveVideos and PreserveAudios are enabled (the default), this replaces the
// two separate WalkNodes calls that extractVideos and extractAudios would each
// make, saving one full tree traversal per Extract call.
func (p *Processor) extractAllMedia(node *stdxhtml.Node, htmlContent string, canContainMedia bool) (videos []VideoInfo, audios []AudioInfo) {
	var videoSeen, audioSeen map[string]bool

	ensureVideoDedup := func() {
		if videoSeen == nil {
			videoSeen = make(map[string]bool, initialMapCap)
		}
		if videos == nil {
			videos = make([]VideoInfo, 0, initialSliceCap)
		}
	}
	ensureAudioDedup := func() {
		if audioSeen == nil {
			audioSeen = make(map[string]bool, initialMapCap)
		}
		if audios == nil {
			audios = make([]AudioInfo, 0, initialSliceCap)
		}
	}

	// Raw HTML extraction for iframe/embed/object (only when canContainMedia).
	// These may be removed by sanitization, so parse from raw HTML first.
	if canContainMedia {
		ensureVideoDedup()
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "iframe", "src"), videoSeen, videos)
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "embed", "src", "data"), videoSeen, videos)
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "object", "data"), videoSeen, videos)
	}

	// Single DOM walk for video, iframe, embed, object, AND audio elements.
	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if n.Type != stdxhtml.ElementNode {
			return true
		}
		switch n.Data {
		case "video":
			if video := p.parseVideoNode(n); video.URL != "" && !videoSeen[video.URL] {
				ensureVideoDedup()
				videoSeen[video.URL] = true
				videos = append(videos, video)
			}
		case "iframe":
			if video := p.parseIframeNode(n); video.URL != "" && !videoSeen[video.URL] {
				ensureVideoDedup()
				videoSeen[video.URL] = true
				videos = append(videos, video)
			}
		case "embed", "object":
			if video := p.parseEmbedNode(n); video.URL != "" && !videoSeen[video.URL] {
				ensureVideoDedup()
				videoSeen[video.URL] = true
				videos = append(videos, video)
			}
		case "audio":
			if audio := p.parseAudioNode(n); audio.URL != "" && !audioSeen[audio.URL] {
				ensureAudioDedup()
				audioSeen[audio.URL] = true
				audios = append(audios, audio)
			}
		}
		return true
	})

	// Regex scans for video and audio URLs in raw HTML (only when canContainMedia).
	if canContainMedia {
		ensureVideoDedup()
		for _, url := range videoRegex.FindAllString(htmlContent, maxRegexMatches) {
			if internal.IsValidURL(url) && !videoSeen[url] {
				videoSeen[url] = true
				videos = append(videos, VideoInfo{
					URL:  url,
					Type: internal.DetectVideoType(url),
				})
			}
		}
		ensureAudioDedup()
		for _, url := range audioRegex.FindAllString(htmlContent, maxRegexMatches) {
			if internal.IsValidURL(url) && !audioSeen[url] {
				audioSeen[url] = true
				audios = append(audios, AudioInfo{
					URL:  url,
					Type: internal.DetectAudioType(url),
				})
			}
		}
	}

	// Normalize to non-nil empty slices (see extractVideos/extractAudios rationale).
	if videos == nil {
		videos = make([]VideoInfo, 0)
	}
	if audios == nil {
		audios = make([]AudioInfo, 0)
	}
	return videos, audios
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
