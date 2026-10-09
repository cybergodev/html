package html_test

// media_test.go — consolidated media (image/video/audio) extraction tests.
//
// Merges media_coverage_test.go, media_node_test.go and the media tests that
// previously lived in html_test.go (TestMediaExtraction, TestVideoEdgeCases,
// TestVideoExtractionComprehensive, TestAudioExtractionComprehensive,
// TestImageExtractionEdgeCases), dropping the scenarios that were tested two or
// three times across those files and keeping the strongest assertion of each.
//
// Two extraction code paths are deliberately kept orthogonal:
//   - default config (PreserveVideos=PreserveAudios=true) → extractAllMedia
//   - video-only / audio-only config → extractVideos / extractAudios
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
// (PreserveAudios=false), which routes through extractVideos instead of
// extractAllMedia.
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
// (PreserveVideos=false), which routes through extractAudios instead of
// extractAllMedia.
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
// extractAllMedia path: plain images, full attribute sets, <picture> fallbacks
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
// extractAllMedia path, table-driven over tag shapes and attribute sets.
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

// TestVideoEmbedExtraction covers iframe/embed/object video extraction through
// the regex fallback path (sanitization on, padded document).
func TestVideoEmbedExtraction(t *testing.T) {
	t.Parallel()

	t.Run("youtube iframe extracted as embed", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t, buildPaddedHTML(
			`<iframe src="https://www.youtube.com/embed/abc123" width="640" height="480"></iframe>`), true)

		if len(videos) == 0 {
			t.Fatal("expected at least one embed video")
		}
		assertVideo(t, videos[0], wantVideo{
			url: "https://www.youtube.com/embed/abc123",
			typ: "embed",
		})
	})

	t.Run("embed with video src URL extracted", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t, buildPaddedHTML(
			`<embed src="https://www.youtube.com/embed/test123" type="video/mp4" width="800" height="600">`), true)

		if len(videos) == 0 {
			t.Fatal("expected at least one video from embed tag")
		}
	})

	t.Run("embed with data attribute extracted", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t, buildPaddedHTML(
			`<embed data="https://player.vimeo.com/video/12345" type="application/x-shockwave-flash" width="400" height="300">`), true)

		if len(videos) == 0 {
			t.Fatal("expected at least one video from embed data attribute")
		}
	})

	t.Run("object with video data URL extracted", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t, buildPaddedHTML(
			`<object data="https://example.com/video.mp4" type="video/mp4"></object>`), true)

		if len(videos) == 0 {
			t.Fatal("expected at least one video from object data")
		}
		assertVideo(t, videos[0], wantVideo{url: "https://example.com/video.mp4"})
	})

	t.Run("non-video iframe ignored", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t, buildPaddedHTML(
			`<iframe src="https://example.com/page" width="800" height="600"></iframe>`), true)

		for _, v := range videos {
			if v.URL == "https://example.com/page" {
				t.Error("non-video iframe URL should not appear in videos")
			}
		}
	})

	t.Run("unsupported SWF embed rejected", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t, buildPaddedHTML(
			`<embed src="https://example.com/file.swf" type="application/octet-stream" width="100" height="100">`), true)

		// SWF is not a recognized video format (no matching extension or embed
		// host), so extraction must not yield it.
		for _, v := range videos {
			if v.URL == "https://example.com/file.swf" {
				t.Error("non-video embed URL should not appear in videos")
			}
		}
	})

	t.Run("iframe without src produces no video", func(t *testing.T) {
		t.Parallel()
		videos := extractMediaVideos(t,
			`<html><body><iframe width="640" height="480"></iframe></body></html>`, true)

		for _, v := range videos {
			if v.URL == "" {
				t.Error("iframe without src should not produce a video entry")
			}
		}
	})
}

// TestParseIframeNodeDOMPath exercises parseIframeNode via the DOM walk path
// by disabling sanitization so iframe tags survive into the parsed tree.
func TestParseIframeNodeDOMPath(t *testing.T) {
	t.Parallel()

	t.Run("iframe with video src via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><head><title>Iframe DOM Test</title></head><body>
			<article>
				<p>Main article content for extraction.</p>
				<iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ" width="640" height="480"></iframe>
			</article>
		</body></html>`, false)

		found := false
		for _, v := range videos {
			if v.URL == "https://www.youtube.com/embed/dQw4w9WgXcQ" {
				found = true
				if v.Type != "embed" {
					t.Errorf("expected type 'embed', got %q", v.Type)
				}
				break
			}
		}
		if !found {
			t.Error("iframe video not found via DOM path")
		}
	})

	t.Run("iframe with non-video src ignored via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><head><title>Non-video Iframe</title></head><body>
			<article>
				<p>Article content.</p>
				<iframe src="https://example.com/page" width="800" height="600"></iframe>
			</article>
		</body></html>`, false)

		for _, v := range videos {
			if v.URL == "https://example.com/page" {
				t.Error("non-video iframe should not appear in videos via DOM path")
			}
		}
	})

	t.Run("iframe without src produces empty video via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><body><article>
			<p>Content.</p>
			<iframe width="640" height="480"></iframe>
		</article></body></html>`, false)

		for _, v := range videos {
			if v.URL == "" {
				t.Error("iframe without src should not produce a video entry")
			}
		}
	})
}

// TestParseEmbedNodeDOMPath exercises parseEmbedNode via the DOM walk path
// by disabling sanitization so embed/object tags survive into the parsed tree.
func TestParseEmbedNodeDOMPath(t *testing.T) {
	t.Parallel()

	t.Run("embed with video src via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><head><title>Embed DOM Test</title></head><body>
			<article>
				<p>Article content.</p>
				<embed src="https://www.youtube.com/embed/test123" type="video/mp4" width="800" height="600">
			</article>
		</body></html>`, false)

		found := false
		for _, v := range videos {
			if v.URL == "https://www.youtube.com/embed/test123" {
				found = true
				break
			}
		}
		if !found {
			t.Error("embed video not found via DOM path")
		}
	})

	t.Run("embed with data attribute via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><body><article>
			<p>Content.</p>
			<embed data="https://player.vimeo.com/video/12345" type="application/x-shockwave-flash" width="400" height="300">
		</article></body></html>`, false)

		found := false
		for _, v := range videos {
			if v.URL == "https://player.vimeo.com/video/12345" {
				found = true
				break
			}
		}
		if !found {
			t.Error("embed data attribute video not found via DOM path")
		}
	})

	t.Run("embed with non-video URL ignored via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><body><article>
			<p>Content.</p>
			<embed src="https://example.com/file.swf" type="application/octet-stream">
		</article></body></html>`, false)

		for _, v := range videos {
			if v.URL == "https://example.com/file.swf" {
				t.Error("non-video embed should not appear in videos via DOM path")
			}
		}
	})

	t.Run("object tag with video data via DOM", func(t *testing.T) {
		videos := extractMediaVideos(t, `<html><body><article>
			<p>Content.</p>
			<object data="https://www.youtube.com/embed/obj123" type="video/mp4" width="320" height="240"></object>
		</article></body></html>`, false)

		found := false
		for _, v := range videos {
			if v.URL == "https://www.youtube.com/embed/obj123" {
				found = true
				break
			}
		}
		if !found {
			t.Error("object video not found via DOM path")
		}
	})
}

// TestExtractVideosOnly covers Processor.extractVideos by disabling
// PreserveAudios so the single-type media extraction path is taken instead of
// the combined extractAllMedia path.
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
