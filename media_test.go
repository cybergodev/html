package html_test

// media_test.go — consolidated media (image/video/audio) extraction tests.
//
// Merges media_coverage_test.go, media_node_test.go and the media tests that
// previously lived in html_test.go (TestMediaExtraction, TestVideoEdgeCases,
// TestVideoExtractionComprehensive, TestAudioExtractionComprehensive,
// TestImageExtractionEdgeCases), dropping the scenarios that were tested two or
// three times across those files and keeping the strongest assertion of each.
// Later rounds folded in media_type_test.go (VideoInfo.Type / AudioInfo.Type
// inference) and media_kind_matrix_test.go (the PreserveVideos/PreserveAudios
// flag matrix), and table-drove the embed regex-path cases and the two DOM-path
// suites (now TestEmbedNodesDOMPath).
//
// Two extraction code paths are deliberately kept orthogonal:
//   - default config (PreserveVideos=PreserveAudios=true) → both kinds
//   - video-only / audio-only config → extractMedia with one want flag set
//     (see extract.go; only reachable when the flags are set asymmetrically)

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// buildPaddedHTML creates an HTML document with padding to ensure regex-based extraction
// is triggered and the content is large enough for full processing.
func buildPaddedHTML(inner string) string {
	var sb strings.Builder
	sb.WriteString("<html><head><title>Test</title></head><body>")
	for range 50 {
		sb.WriteString("<p>This is paragraph content to pad the HTML for testing purposes.</p>")
	}
	sb.WriteString(inner)
	for range 50 {
		sb.WriteString("<p>This is paragraph content to pad the HTML for testing purposes.</p>")
	}
	sb.WriteString("</body></html>")
	return sb.String()
}

// newVideoOnlyProcessor returns a processor configured for video-only extraction
// (PreserveAudios=false), which routes through extractMedia with wantAudios=false.
func newVideoOnlyProcessor(t *testing.T) *html.Processor {
	t.Helper()
	cfg := html.DefaultConfig()
	cfg.PreserveVideos = true
	cfg.PreserveAudios = false
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return p
}

// newAudioOnlyProcessor returns a processor configured for audio-only extraction
// (PreserveVideos=false), which routes through extractMedia with wantVideos=false.
func newAudioOnlyProcessor(t *testing.T) *html.Processor {
	t.Helper()
	cfg := html.DefaultConfig()
	cfg.PreserveVideos = false
	cfg.PreserveAudios = true
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return p
}

// extractMediaVideos builds a processor with the requested sanitization setting,
// runs Extract on htmlContent, and returns the extracted videos. It centralizes the
// processor lifecycle and error handling that every case below would otherwise repeat.
func extractMediaVideos(t *testing.T, htmlContent string, sanitize bool) []html.VideoInfo {
	t.Helper()
	cfg := html.DefaultConfig()
	cfg.EnableSanitization = sanitize
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()
	result, err := p.Extract([]byte(htmlContent))
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}
	return result.Videos
}

// wantVideo pins the expected field values of an extracted video. Only fields
// with a non-empty want are asserted, so a table row can focus on the
// attributes it means to pin.
type wantVideo struct {
	url, typ, poster, width, height, duration string
}

func assertVideo(t *testing.T, got html.VideoInfo, want wantVideo) {
	t.Helper()
	if want.url != "" && got.URL != want.url {
		t.Errorf("URL = %q, want %q", got.URL, want.url)
	}
	if want.typ != "" && got.Type != want.typ {
		t.Errorf("Type = %q, want %q", got.Type, want.typ)
	}
	if want.poster != "" && got.Poster != want.poster {
		t.Errorf("Poster = %q, want %q", got.Poster, want.poster)
	}
	if want.width != "" && got.Width != want.width {
		t.Errorf("Width = %q, want %q", got.Width, want.width)
	}
	if want.height != "" && got.Height != want.height {
		t.Errorf("Height = %q, want %q", got.Height, want.height)
	}
	if want.duration != "" && got.Duration != want.duration {
		t.Errorf("Duration = %q, want %q", got.Duration, want.duration)
	}
}

// TestImageExtraction covers <img> attribute extraction through the default
// combined media path: plain images, full attribute sets, <picture> fallbacks
// and srcset handling.
func TestImageExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		html      string
		wantLen   int // exact expected image count
		wantImg   int // index to assert against want (asserted when want is set)
		altSet    bool
		want      wantVideo
		wantAlt   string
		wantTitle string
		wantW     string
		wantH     string
	}{
		{
			name: "images extracted with attributes",
			html: `<html><body>
				<img src="img1.jpg" alt="Image 1" width="800" height="600">
				<img src="img2.png" alt="Image 2">
			</body></html>`,
			wantLen: 2,
			wantImg: 0,
			want:    wantVideo{url: "img1.jpg"},
			wantAlt: "Image 1",
			wantW:   "800",
		},
		{
			name: "img with all attributes",
			html: `<html><body>
				<img src="photo.jpg" alt="Photo" width="800" height="600" title="My Photo">
			</body></html>`,
			wantLen:   1,
			wantImg:   0,
			want:      wantVideo{url: "photo.jpg"},
			wantAlt:   "Photo",
			wantTitle: "My Photo",
			wantW:     "800",
			wantH:     "600",
		},
		{
			name: "picture element with source and img fallback",
			html: `<html><body>
				<picture>
					<source srcset="image.webp" type="image/webp">
					<source srcset="image.jpg" type="image/jpeg">
					<img src="image-fallback.jpg" alt="Fallback">
				</picture>
			</body></html>`,
			// At least the <img> fallback must be extracted; exact count is not
			// pinned because srcset handling is version-dependent.
			wantLen: -1,
		},
		{
			name: "img with srcset attribute",
			html: `<html><body>
				<img srcset="small.jpg 300w, medium.jpg 600w, large.jpg 1200w"
				     src="fallback.jpg" alt="Responsive image">
			</body></html>`,
			wantLen: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := html.New()
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = p.Close() }()

			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}
			if tt.wantLen >= 0 && len(result.Images) != tt.wantLen {
				t.Fatalf("Got %d images, want %d", len(result.Images), tt.wantLen)
			}
			if tt.wantLen < 0 && len(result.Images) == 0 {
				t.Fatal("expected at least one image")
			}
			if tt.want.url != "" {
				img := result.Images[tt.wantImg]
				if img.URL != tt.want.url {
					t.Errorf("URL = %q, want %q", img.URL, tt.want.url)
				}
				if tt.wantAlt != "" && img.Alt != tt.wantAlt {
					t.Errorf("Alt = %q, want %q", img.Alt, tt.wantAlt)
				}
				if tt.wantTitle != "" && img.Title != tt.wantTitle {
					t.Errorf("Title = %q, want %q", img.Title, tt.wantTitle)
				}
				if tt.wantW != "" && img.Width != tt.wantW {
					t.Errorf("Width = %q, want %q", img.Width, tt.wantW)
				}
				if tt.wantH != "" && img.Height != tt.wantH {
					t.Errorf("Height = %q, want %q", img.Height, tt.wantH)
				}
			}
		})
	}
}

// TestVideoExtraction covers <video> element extraction through the default
// combined media path, table-driven over tag shapes and attribute sets.
func TestVideoExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		html    string
		wantLen int // -1 means "at least one"
		want    wantVideo
	}{
		{
			name: "video tag with all attributes",
			html: `<html><body>
				<video src="video.mp4" poster="poster.jpg" width="800" height="600" duration="120"></video>
			</body></html>`,
			wantLen: 1,
			want:    wantVideo{url: "video.mp4", poster: "poster.jpg", width: "800", height: "600", duration: "120"},
		},
		{
			name: "video with track child and poster",
			html: `<html><body>
				<video src="video.mp4" poster="poster.jpg" width="1920" height="1080">
					<track kind="subtitles" src="subs.vtt" srclang="en">
				</video>
			</body></html>`,
			wantLen: 1,
			want:    wantVideo{url: "video.mp4", poster: "poster.jpg", width: "1920", height: "1080"},
		},
		{
			name: "video with source children extracts first source",
			html: `<html><body>
				<video>
					<source src="video1.mp4" type="video/mp4">
					<source src="video2.webm" type="video/webm">
				</video>
			</body></html>`,
			wantLen: -1,
			want:    wantVideo{url: "video1.mp4"},
		},
		{
			name:    "no media yields empty slice",
			html:    `<html><body><p>No media here</p></body></html>`,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := html.New()
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = p.Close() }()

			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}
			if tt.wantLen >= 0 && len(result.Videos) != tt.wantLen {
				t.Fatalf("Got %d videos, want %d", len(result.Videos), tt.wantLen)
			}
			if tt.wantLen < 0 && len(result.Videos) == 0 {
				t.Fatal("expected at least one video")
			}
			if len(result.Videos) > 0 {
				assertVideo(t, result.Videos[0], tt.want)
			}
		})
	}
}

func TestVideoAudioEmptySrcFallsBackToSource(t *testing.T) {
	t.Parallel()

	// An explicit src="" is absence, not invalidity: the element's <source>
	// children (and the poster) must survive, exactly as when the src
	// attribute were missing. The raw-HTML scan cannot rescue this — it only
	// matches absolute http(s) URLs — so only the DOM path can produce the
	// poster-bearing entry.
	doc := []byte(`<html><body><article>
		<video src="" poster="/p.jpg"><source src="clip.mp4" type="video/mp4"></video>
		<audio src=""><source src="tone.mp3" type="audio/mpeg"></audio>
		<p>Prose so the article is found.</p>
	</article></body></html>`)

	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0
	result, err := html.Extract(doc, cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	var sawVideo, sawAudio bool
	for _, v := range result.Videos {
		if v.URL == "clip.mp4" && v.Poster == "/p.jpg" {
			sawVideo = true
		}
	}
	for _, a := range result.Audios {
		if a.URL == "tone.mp3" {
			sawAudio = true
		}
	}
	if !sawVideo {
		t.Errorf("empty-src video did not fall back to its <source> child: %+v", result.Videos)
	}
	if !sawAudio {
		t.Errorf("empty-src audio did not fall back to its <source> child: %+v", result.Audios)
	}
}

// TestVideoEmbedExtraction covers iframe/embed/object video extraction
// through the regex fallback path (sanitization on, padded document),
// table-driven over the embed shapes: each row pins the video URLs that must
// appear and those that must not. An empty-string forbid is the "iframe
// without src" contract — no entry with an empty URL.
func TestVideoEmbedExtraction(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		inner   string
		wants   []string
		typ     string // Type pinned on wanted URLs when set
		forbids []string
	}{
		{
			name:  "youtube iframe extracted as embed",
			inner: `<iframe src="https://www.youtube.com/embed/abc123" width="640" height="480"></iframe>`,
			wants: []string{"https://www.youtube.com/embed/abc123"},
			typ:   "embed",
		},
		{
			name:  "embed with video src URL extracted",
			inner: `<embed src="https://www.youtube.com/embed/test123" type="video/mp4" width="800" height="600">`,
			wants: []string{"https://www.youtube.com/embed/test123"},
		},
		{
			name:  "embed with data attribute extracted",
			inner: `<embed data="https://player.vimeo.com/video/12345" type="application/x-shockwave-flash" width="400" height="300">`,
			wants: []string{"https://player.vimeo.com/video/12345"},
		},
		{
			name:  "object with video data URL extracted",
			inner: `<object data="https://example.com/video.mp4" type="video/mp4"></object>`,
			wants: []string{"https://example.com/video.mp4"},
		},
		{
			name:    "non-video iframe ignored",
			inner:   `<iframe src="https://example.com/page" width="800" height="600"></iframe>`,
			forbids: []string{"https://example.com/page"},
		},
		{
			// SWF is not a recognized video format (no matching extension or
			// embed host), so extraction must not yield it.
			name:    "unsupported SWF embed rejected",
			inner:   `<embed src="https://example.com/file.swf" type="application/octet-stream" width="100" height="100">`,
			forbids: []string{"https://example.com/file.swf"},
		},
		{
			name:    "iframe without src produces no video",
			inner:   `<iframe width="640" height="480"></iframe>`,
			forbids: []string{""},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			videos := extractMediaVideos(t, buildPaddedHTML(tt.inner), true)
			for _, want := range tt.wants {
				found := false
				for _, v := range videos {
					if v.URL == want {
						found = true
						if tt.typ != "" && v.Type != tt.typ {
							t.Errorf("%s: Type = %q, want %q", want, v.Type, tt.typ)
						}
						break
					}
				}
				if !found {
					t.Errorf("video %q missing from regex-path result: %+v", want, videos)
				}
			}
			for _, forbid := range tt.forbids {
				for _, v := range videos {
					if v.URL == forbid {
						t.Errorf("forbidden video URL %q in regex-path result: %+v", forbid, videos)
					}
				}
			}
		})
	}
}

// TestEmbedNodesDOMPath exercises parseIframeNode / parseEmbedNode via the
// DOM walk path by disabling sanitization so the tags survive into the parsed
// tree. Table-driven over the same shapes as the regex-path table above —
// the two code paths are deliberately kept orthogonal (see the file header):
// each row pins the URLs that must appear on this path and those that must
// not, with the empty-string forbid again meaning "no empty-URL entry".
func TestEmbedNodesDOMPath(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		inner   string
		wants   []string
		typ     string
		forbids []string
	}{
		{
			name: "iframe with video src via DOM",
			inner: `<article>
				<p>Main article content for extraction.</p>
				<iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ" width="640" height="480"></iframe>
			</article>`,
			wants: []string{"https://www.youtube.com/embed/dQw4w9WgXcQ"},
			typ:   "embed",
		},
		{
			name:    "iframe with non-video src ignored via DOM",
			inner:   `<article><p>Article content.</p><iframe src="https://example.com/page" width="800" height="600"></iframe></article>`,
			forbids: []string{"https://example.com/page"},
		},
		{
			name:    "iframe without src produces no video via DOM",
			inner:   `<article><p>Content.</p><iframe width="640" height="480"></iframe></article>`,
			forbids: []string{""},
		},
		{
			name: "embed with video src via DOM",
			inner: `<article>
				<p>Article content.</p>
				<embed src="https://www.youtube.com/embed/test123" type="video/mp4" width="800" height="600">
			</article>`,
			wants: []string{"https://www.youtube.com/embed/test123"},
		},
		{
			name:  "embed with data attribute via DOM",
			inner: `<article><p>Content.</p><embed data="https://player.vimeo.com/video/12345" type="application/x-shockwave-flash" width="400" height="300"></article>`,
			wants: []string{"https://player.vimeo.com/video/12345"},
		},
		{
			name:    "embed with non-video URL ignored via DOM",
			inner:   `<article><p>Content.</p><embed src="https://example.com/file.swf" type="application/octet-stream"></article>`,
			forbids: []string{"https://example.com/file.swf"},
		},
		{
			name:  "object tag with video data via DOM",
			inner: `<article><p>Content.</p><object data="https://www.youtube.com/embed/obj123" type="video/mp4" width="320" height="240"></object></article>`,
			wants: []string{"https://www.youtube.com/embed/obj123"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := "<html><head><title>Embed DOM Test</title></head><body>" + tt.inner + "</body></html>"
			videos := extractMediaVideos(t, doc, false)
			for _, want := range tt.wants {
				found := false
				for _, v := range videos {
					if v.URL == want {
						found = true
						if tt.typ != "" && v.Type != tt.typ {
							t.Errorf("%s: Type = %q, want %q", want, v.Type, tt.typ)
						}
						break
					}
				}
				if !found {
					t.Errorf("video %q missing from DOM-path result: %+v", want, videos)
				}
			}
			for _, forbid := range tt.forbids {
				for _, v := range videos {
					if v.URL == forbid {
						t.Errorf("forbidden video URL %q in DOM-path result: %+v", forbid, videos)
					}
				}
			}
		})
	}
}

// TestExtractVideosOnly covers the video-only configuration (PreserveAudios
// off, so extractMedia is called with wantAudios=false and the audio branches
// and audio raw-HTML scan are skipped).
func TestExtractVideosOnly(t *testing.T) {
	t.Parallel()

	t.Run("video tag with src", func(t *testing.T) {
		t.Parallel()
		p := newVideoOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(
			`<html><body><video src="clip.mp4" poster="thumb.jpg" width="1280" height="720"></video></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) == 0 {
			t.Fatal("expected at least one video")
		}
		assertVideo(t, result.Videos[0], wantVideo{
			url: "clip.mp4", poster: "thumb.jpg", width: "1280", height: "720",
		})
		// Audios must be empty because PreserveAudios=false.
		if len(result.Audios) != 0 {
			t.Errorf("expected 0 audios, got %d", len(result.Audios))
		}
	})

	t.Run("video with source children", func(t *testing.T) {
		t.Parallel()
		p := newVideoOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body>
			<video>
				<source src="hi.mp4" type="video/mp4">
				<source src="lo.webm" type="video/webm">
			</video>
		</body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) == 0 {
			t.Fatal("expected at least one video from <source> tags")
		}
	})

	t.Run("iframe embed video via regex", func(t *testing.T) {
		t.Parallel()
		p := newVideoOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		// Large enough to trigger the regex-based extraction path.
		htmlContent := buildPaddedHTML(
			`<iframe src="https://www.youtube.com/embed/abc123"></iframe>`)
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		found := false
		for _, v := range result.Videos {
			if strings.Contains(v.URL, "youtube.com/embed/abc123") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find youtube embed video via regex extraction")
		}
	})

	t.Run("no media yields empty slice", func(t *testing.T) {
		t.Parallel()
		p := newVideoOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(`<html><body><p>No media here</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) != 0 {
			t.Errorf("expected 0 videos, got %d", len(result.Videos))
		}
	})
}

// TestExtractAudiosOnly covers Processor.extractAudios by disabling
// PreserveVideos so the single-type media extraction path is taken.
func TestExtractAudiosOnly(t *testing.T) {
	t.Parallel()

	t.Run("audio tag with src", func(t *testing.T) {
		t.Parallel()
		p := newAudioOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(
			`<html><body><audio src="track.mp3" controls></audio></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Audios) != 1 {
			t.Fatalf("expected exactly 1 audio, got %d", len(result.Audios))
		}
		if result.Audios[0].URL != "track.mp3" {
			t.Errorf("URL = %q, want 'track.mp3'", result.Audios[0].URL)
		}
		// Videos must be empty because PreserveVideos=false.
		if len(result.Videos) != 0 {
			t.Errorf("expected 0 videos, got %d", len(result.Videos))
		}
	})

	t.Run("audio with source children", func(t *testing.T) {
		t.Parallel()
		p := newAudioOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body>
			<audio>
				<source src="song.mp3" type="audio/mpeg">
				<source src="song.ogg" type="audio/ogg">
			</audio>
		</body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Audios) == 0 {
			t.Fatal("expected at least one audio from <source> tags")
		}
		if result.Audios[0].URL != "song.mp3" {
			t.Errorf("first source URL = %q, want 'song.mp3'", result.Audios[0].URL)
		}
	})

	t.Run("no media yields empty slice", func(t *testing.T) {
		t.Parallel()
		p := newAudioOnlyProcessor(t)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(`<html><body><p>Text only</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Audios) != 0 {
			t.Errorf("expected 0 audios, got %d", len(result.Audios))
		}
	})
}

// ---------------------------------------------------------------------------
// Media Type inference (moved from the former media_type_test.go)
// ---------------------------------------------------------------------------

// media_type_test.go — pins VideoInfo.Type / AudioInfo.Type. They used to be
// populated only when the markup declared a type (<source type>, <embed type>)
// or for iframe embeds, so the common "<video src=movie.mp4>" shape and every
// video found by the raw-HTML scan reported an empty Type.

func TestVideoTypeFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		url  string
		typ  string
	}{
		{
			name: "video src infers container from extension",
			html: `<video src="movie.mp4"></video>`,
			url:  "movie.mp4",
			typ:  "video/mp4",
		},
		{
			name: "video src infers webm",
			html: `<video src="/media/clip.webm"></video>`,
			url:  "/media/clip.webm",
			typ:  "video/webm",
		},
		{
			name: "query string does not defeat the extension",
			html: `<video src="movie.mp4?v=2#t=10"></video>`,
			url:  "movie.mp4?v=2#t=10",
			typ:  "video/mp4",
		},
		{
			name: "source child infers when it declares no type",
			html: `<video><source src="clip.mkv"></video>`,
			url:  "clip.mkv",
			typ:  "video/x-matroska",
		},
		{
			name: "explicit source type wins over the extension",
			html: `<video><source src="stream.mp4" type="video/webm"></video>`,
			url:  "stream.mp4",
			typ:  "video/webm",
		},
		{
			name: "embed infers container",
			html: `<embed src="/clip.webm">`,
			url:  "/clip.webm",
			typ:  "video/webm",
		},
		{
			name: "unrecognized extension stays empty",
			html: `<video src="https://cdn.example/stream"></video>`,
			url:  "https://cdn.example/stream",
			typ:  "",
		},
		{
			name: "iframe embed host still reports embed",
			html: `<iframe src="https://www.youtube.com/embed/abc"></iframe>`,
			url:  "https://www.youtube.com/embed/abc",
			typ:  "embed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultConfig()
			cfg.MaxCacheEntries = 0
			res, err := html.Extract([]byte("<html><body>"+tt.html+"</body></html>"), cfg)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			var got *html.VideoInfo
			for i := range res.Videos {
				if res.Videos[i].URL == tt.url {
					got = &res.Videos[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("video %q not extracted: %+v", tt.url, res.Videos)
			}
			if got.Type != tt.typ {
				t.Errorf("Type = %q, want %q", got.Type, tt.typ)
			}
		})
	}
}

func TestAudioTypeFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		url  string
		typ  string
	}{
		{
			name: "audio src infers container from extension",
			html: `<audio src="sound.mp3"></audio>`,
			url:  "sound.mp3",
			typ:  "audio/mpeg",
		},
		{
			name: "source child infers when it declares no type",
			html: `<audio><source src="sound.ogg"></audio>`,
			url:  "sound.ogg",
			typ:  "audio/ogg",
		},
		{
			name: "explicit source type wins",
			html: `<audio><source src="sound.mp3" type="audio/flac"></audio>`,
			url:  "sound.mp3",
			typ:  "audio/flac",
		},
		{
			name: "unrecognized extension stays empty",
			html: `<audio src="https://cdn.example/stream"></audio>`,
			url:  "https://cdn.example/stream",
			typ:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultConfig()
			cfg.MaxCacheEntries = 0
			res, err := html.Extract([]byte("<html><body>"+tt.html+"</body></html>"), cfg)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			var got *html.AudioInfo
			for i := range res.Audios {
				if res.Audios[i].URL == tt.url {
					got = &res.Audios[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("audio %q not extracted: %+v", tt.url, res.Audios)
			}
			if got.Type != tt.typ {
				t.Errorf("Type = %q, want %q", got.Type, tt.typ)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Media kind matrix (moved from the former media_kind_matrix_test.go)
// ---------------------------------------------------------------------------

// media_kind_matrix_test.go — pins the media matrix that Processor.extractMedia
// now serves through its wantVideos / wantAudios flags. The previous shape ran
// the full combined extraction and discarded half of it (including one whole
// raw-HTML media scan), so a single-kind configuration silently paid for the
// other kind; these assertions would fail if a flag stopped gating its branches.

const mediaMatrixDoc = `<html><body>
	<video src="/v.mp4"></video>
	<audio src="/a.mp3"></audio>
	<iframe src="https://www.youtube.com/embed/emb"></iframe>
	<p>raw references: /raw.mp4 and /raw.mp3</p>
	</body></html>`

func TestMediaKindMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		videos   bool
		audios   bool
		wantVids bool
		wantAud  bool
	}{
		{"both enabled", true, true, true, true},
		{"videos only", true, false, true, false},
		{"audios only", false, true, false, true},
		{"neither", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultConfig()
			cfg.MaxCacheEntries = 0
			cfg.PreserveVideos = tt.videos
			cfg.PreserveAudios = tt.audios

			res, err := html.Extract([]byte(mediaMatrixDoc), cfg)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}

			if got := len(res.Videos) > 0; got != tt.wantVids {
				t.Errorf("Videos = %d, want present=%v", len(res.Videos), tt.wantVids)
			}
			if got := len(res.Audios) > 0; got != tt.wantAud {
				t.Errorf("Audios = %d, want present=%v", len(res.Audios), tt.wantAud)
			}
			// A kind the caller did not ask for must stay nil so it is omitted
			// from JSON rather than serialized as an empty array.
			if !tt.videos && res.Videos != nil {
				t.Errorf("Videos = %#v, want nil when PreserveVideos is false", res.Videos)
			}
			if !tt.audios && res.Audios != nil {
				t.Errorf("Audios = %#v, want nil when PreserveAudios is false", res.Audios)
			}
		})
	}
}

// TestMediaKindsStayDeduplicated keeps the dedup contract across the merged
// implementation: a URL repeated in markup and in raw text appears once.
func TestMediaKindsStayDeduplicated(t *testing.T) {
	t.Parallel()

	doc := `<html><body>
		<video src="/dup.mp4"></video>
		<video src="/dup.mp4"></video>
		<audio src="/dup.mp3"></audio>
		<audio src="/dup.mp3"></audio>
		<p>/dup.mp4 /dup.mp3</p>
		</body></html>`

	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := html.Extract([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Videos) != 1 {
		t.Errorf("Videos = %+v, want exactly one deduplicated entry", res.Videos)
	}
	if len(res.Audios) != 1 {
		t.Errorf("Audios = %+v, want exactly one deduplicated entry", res.Audios)
	}
}

// TestSingleKindSkipsTheOtherScan documents the work the flags save: with one
// kind disabled, a document whose only media is the other kind still returns
// nothing for the disabled kind — proving its raw-HTML scan did not run and
// leak entries back in.
func TestSingleKindSkipsTheOtherScan(t *testing.T) {
	t.Parallel()

	// The raw-HTML media scanner matches absolute http(s) URLs only, so the
	// reference has to be absolute for this test to exercise it at all.
	doc := `<html><body><p>only https://e.com/only.mp3 here</p></body></html>`

	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.PreserveVideos = true
	cfg.PreserveAudios = false

	res, err := html.Extract([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Audios) != 0 || res.Audios != nil {
		t.Errorf("Audios = %+v, want nil when PreserveAudios is false", res.Audios)
	}

	// The same document with audio enabled does report it, so the assertion
	// above is about the flag, not about the scanner missing the URL.
	cfg2 := html.DefaultConfig()
	cfg2.MaxCacheEntries = 0
	cfg2.PreserveVideos = false
	cfg2.PreserveAudios = true
	res2, err := html.Extract([]byte(doc), cfg2)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res2.Audios) != 1 {
		t.Fatalf("Audios = %+v, want the .mp3 found by the raw-HTML scan", res2.Audios)
	}
}
