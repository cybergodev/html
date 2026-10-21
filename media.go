package html

import (
	htmlstd "html"
	"strings"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// appendUniqueVideoURLs appends each url that is a valid, not-yet-seen video URL
// to videos, recording it in seen. It centralizes the validate-and-deduplicate
// logic shared by the iframe, embed, and object raw-HTML extraction paths.
// DetectVideoType is called once per url: a non-empty result is exactly
// IsVideoURL's condition (extension or embed-host match), so classifying and
// re-checking would re-lowercase and re-scan the same URL twice.
//
// URLs reach this function as slices of the raw document string, which on the
// zero-copy decode path is a view of the caller's input []byte (see
// internal.DetectAndConvertToUTF8String), so the retained value is cloned. This
// is the only raw-HTML funnel that reaches Result.Videos without passing
// through a ScanMediaURLs callback, so the clone has to happen here.
func appendUniqueVideoURLs(urls []string, seen map[string]bool, videos []VideoInfo) []VideoInfo {
	for _, url := range urls {
		if !internal.IsValidURL(url) || seen[url] {
			continue
		}
		if mediaType := internal.DetectVideoType(url); mediaType != "" {
			url = strings.Clone(url)
			seen[url] = true
			videos = append(videos, VideoInfo{
				URL:  url,
				Type: mediaType,
			})
		}
	}
	return videos
}

// extractMedia extracts video and audio sources in a single pass. wantVideos
// and wantAudios gate both the DOM walk branches and the raw-HTML scans, so a
// caller that preserves only one media kind never pays for the other: the
// earlier shape always ran the full combined extraction and threw half of it
// away, including a whole extra ScanMediaURLs pass over the document.
//
// Both returned slices are non-nil: an empty but present result is what
// callers marshal (JSON "videos": [] rather than null) and what the tests
// assert on.
func (p *Processor) extractMedia(node *stdxhtml.Node, htmlContent string, canContainMedia, wantVideos, wantAudios bool) (videos []VideoInfo, audios []AudioInfo) {
	var videoSeen, audioSeen map[string]bool

	// ensure lazily allocates the per-kind slice and dedup set, so a media kind
	// that yields nothing allocates nothing.
	ensureVideo := func() {
		if videoSeen == nil {
			videoSeen = make(map[string]bool, initialMapCap)
		}
		if videos == nil {
			videos = make([]VideoInfo, 0, initialSliceCap)
		}
	}
	ensureAudio := func() {
		if audioSeen == nil {
			audioSeen = make(map[string]bool, initialMapCap)
		}
		if audios == nil {
			audios = make([]AudioInfo, 0, initialSliceCap)
		}
	}
	// addVideo/addAudio are the single funnel for every media result, so the
	// dedup bookkeeping lives here. They deliberately do NOT clone the URL:
	// parseVideoNode and friends return values the HTML parser owns (the
	// tokenizer copies attribute values out of its buffer), and the two raw-HTML
	// paths that DO hand over slices of the decoded document —
	// appendUniqueVideoURLs and the ScanMediaURLs callbacks below — clone before
	// they get here. Cloning unconditionally would add an allocation per DOM
	// media element, which is the common case.
	addVideo := func(v VideoInfo) {
		if v.URL == "" || videoSeen[v.URL] {
			return
		}
		ensureVideo()
		videoSeen[v.URL] = true
		videos = append(videos, v)
	}
	addAudio := func(a AudioInfo) {
		if a.URL == "" || audioSeen[a.URL] {
			return
		}
		ensureAudio()
		audioSeen[a.URL] = true
		audios = append(audios, a)
	}

	// Raw-HTML extraction for iframe/embed/object happens first because
	// sanitization removes those elements from the DOM: parsing the raw markup
	// is the only way to see them.
	if canContainMedia && wantVideos {
		ensureVideo()
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "iframe", "src"), videoSeen, videos)
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "embed", "src", "data"), videoSeen, videos)
		videos = appendUniqueVideoURLs(
			p.extractTagAttributes(htmlContent, "object", "data"), videoSeen, videos)
	}

	// One DOM walk serves both kinds.
	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if n.Type != stdxhtml.ElementNode {
			return true
		}
		switch n.Data {
		case "video":
			if wantVideos {
				addVideo(p.parseVideoNode(n))
			}
		case "iframe":
			if wantVideos {
				addVideo(p.parseIframeNode(n))
			}
		case "embed", "object":
			if wantVideos {
				addVideo(p.parseEmbedNode(n))
			}
		case "audio":
			if wantAudios {
				addAudio(p.parseAudioNode(n))
			}
		}
		return true
	})

	// Regex-equivalent scans for media URLs in the raw HTML, which reach
	// content that never becomes a media element (JSON payloads, comments).
	// ScanMediaURLs hands back substrings of htmlContent, so the retained URL is
	// cloned (see appendUniqueVideoURLs for why that matters).
	if canContainMedia {
		if wantVideos {
			ensureVideo()
			internal.ScanMediaURLs(htmlContent, true, maxRegexMatches, func(url string) bool {
				if internal.IsValidURL(url) {
					addVideo(VideoInfo{URL: strings.Clone(url), Type: internal.DetectVideoType(url)})
				}
				return true
			})
		}
		if wantAudios {
			ensureAudio()
			internal.ScanMediaURLs(htmlContent, false, maxRegexMatches, func(url string) bool {
				if internal.IsValidURL(url) {
					addAudio(AudioInfo{URL: strings.Clone(url), Type: internal.DetectAudioType(url)})
				}
				return true
			})
		}
	}

	// Normalize only the kinds the caller asked for. A kind that was not
	// requested stays nil so it is omitted from the JSON result, matching what
	// the previous split extractors produced by assigning only their own field.
	if wantVideos && videos == nil {
		videos = make([]VideoInfo, 0)
	}
	if wantAudios && audios == nil {
		audios = make([]AudioInfo, 0)
	}
	return videos, audios
}

// parseVideoNode reads a <video> element. The URL comes from src, or from the
// first <source> child when the element declares none, and Type prefers an
// explicit <source type> (a server may serve a different container than the
// extension suggests) before falling back to the extension-derived MIME type.
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

	// Fill Type from the URL when no <source type> declared one. Without this
	// the field was populated only for <source>-based videos and iframes,
	// leaving the common "<video src=…>" shape (and every video found by the
	// raw-HTML scan) with an empty Type — so a caller could not tell an mp4
	// source from a webm one. DetectVideoType returns the container MIME type
	// ("video/mp4", "video/webm", …) or "" for an unrecognized URL, which is
	// left empty rather than guessed.
	if video.Type == "" {
		video.Type = internal.DetectVideoType(video.URL)
	}

	return video
}

// parseIframeNode reads an <iframe>, reporting Type "embed" because the frame
// itself is not a media container.
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

// parseEmbedNode reads an <embed> or <object>, whose source may live in src or
// data.
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
	// Mirror parseVideoNode: an <embed>/<object> without a type attribute still
	// reports its container MIME type rather than an empty one.
	if video.Type == "" {
		video.Type = internal.DetectVideoType(video.URL)
	}
	return video
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
		// A <source> child may declare its own type, which is authoritative.
		audio.URL, audio.Type = p.findSourceURL(n)
	}

	if !internal.IsValidURL(audio.URL) {
		return AudioInfo{}
	}

	// Fill Type from the URL when no <source type> declared one, matching
	// parseVideoNode: "<audio src=…>" otherwise reported an empty Type while a
	// <source>-based audio reported its container MIME type.
	if audio.Type == "" {
		audio.Type = internal.DetectAudioType(audio.URL)
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

		// Find the end of the opening tag, skipping quoted attribute values:
		// HTML permits '>' inside quotes, and cutting at the first one both
		// truncated the value being read and left the scan position inside
		// the tag — any real attribute after a '>' within an earlier quoted
		// value was never reached.
		tagEnd := indexTagEnd(htmlContent[tagStart:])
		if tagEnd == -1 {
			break
		}
		tagEnd += tagStart + 1

		tagContent := htmlContent[tagStart:tagEnd]

		// Extract requested attributes from this tag
		for _, attrName := range attrNames {
			if value := extractAttributeValue(tagContent, attrName); value != "" {
				// Attribute values here come from RAW, pre-parse HTML — unlike
				// the DOM paths, no parser has decoded character references
				// yet. Decode exactly one level so URLs extracted from tags the
				// sanitizer removes match their DOM-extracted counterparts
				// ("?a=1&amp;b=2" → "?a=1&b=2").
				results = append(results, htmlstd.UnescapeString(value))
			}
		}

		pos = tagEnd
	}

	return results
}

// indexTagEnd returns the index of the first '>' in s that lies outside a
// quoted attribute value, or -1. An unterminated quote falls back to the raw
// first '>' — the pre-existing truncation behavior (pinned by
// media_internal_test.go) — so a value is still read from a tag whose source
// was cut inside a quoted value.
func indexTagEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\'':
			end := strings.IndexByte(s[i+1:], s[i])
			if end < 0 {
				return strings.IndexByte(s, '>')
			}
			i += end + 1
		case '>':
			return i
		}
	}
	return -1
}

// findTagIgnoreCase returns the index of the first occurrence of lowerTag
// (e.g. "<iframe") in html, comparing ASCII case-insensitively, or -1 if
// absent. The scan jumps between '<' candidates with the vectorized
// strings.IndexByte and verifies each with EqualFold, so the FIRST match in
// document order wins regardless of letter case. A strings.Index exact-match
// fast path here would jump to a later exact-case occurrence and permanently
// skip an earlier mixed-case tag (e.g. <IFRAME> before <iframe>), silently
// dropping its URLs from extraction.
func findTagIgnoreCase(html, lowerTag string) int {
	tagLen := len(lowerTag)
	if tagLen == 0 || len(html) < tagLen {
		return -1
	}
	for i := 0; i+tagLen <= len(html); {
		if html[i] != '<' {
			next := strings.IndexByte(html[i:], '<')
			if next < 0 {
				return -1
			}
			i += next
			continue
		}
		if strings.EqualFold(html[i:i+tagLen], lowerTag) {
			return i
		}
		i++
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
		// Skip quoted attribute values: an occurrence of the name inside
		// another attribute's value (title="video src=here") is text, not an
		// attribute, and the first such decoy match used to win — the real
		// src after it was never read. Advancing to the closing quote keeps
		// name-position matches outside values; an unterminated quote means
		// the tag was truncated inside a value (the caller cut it at a '>'),
		// so no further name-position match is trustworthy.
		if q := tagContent[pos]; q == '"' || q == '\'' {
			end := strings.IndexByte(tagContent[pos+1:], q)
			if end < 0 {
				break
			}
			pos += end + 2
			continue
		}
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
