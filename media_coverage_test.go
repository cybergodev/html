package html_test

// media_coverage_test.go — Covers the extractVideos (0%) and extractAudios (0%)
// code paths in media.go. These methods are only called when PreserveVideos and
// PreserveAudios are set asymmetrically (one true, the other false); the default
// (both true) takes the combined extractAllMedia path instead. See
// extract.go:828–833.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

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

// TestExtractVideosOnly covers Processor.extractVideos by disabling
// PreserveAudios so the single-type media extraction path is taken instead of
// the combined extractAllMedia path.
func TestExtractVideosOnly(t *testing.T) {
	t.Parallel()

	t.Run("video tag with src", func(t *testing.T) {
		t.Parallel()
		p := newVideoOnlyProcessor(t)
		defer p.Close()

		result, err := p.Extract([]byte(
			`<html><body><video src="clip.mp4" poster="thumb.jpg" width="1280" height="720"></video></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) == 0 {
			t.Fatal("expected at least one video")
		}
		if result.Videos[0].URL != "clip.mp4" {
			t.Errorf("URL = %q, want 'clip.mp4'", result.Videos[0].URL)
		}
		// Audios must be empty because PreserveAudios=false.
		if len(result.Audios) != 0 {
			t.Errorf("expected 0 audios, got %d", len(result.Audios))
		}
	})

	t.Run("video with source children", func(t *testing.T) {
		t.Parallel()
		p := newVideoOnlyProcessor(t)
		defer p.Close()

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
		defer p.Close()

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
		defer p.Close()

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
		defer p.Close()

		result, err := p.Extract([]byte(
			`<html><body><audio src="track.mp3" controls></audio></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Audios) == 0 {
			t.Fatal("expected at least one audio")
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
		defer p.Close()

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
	})

	t.Run("no media yields empty slice", func(t *testing.T) {
		t.Parallel()
		p := newAudioOnlyProcessor(t)
		defer p.Close()

		result, err := p.Extract([]byte(`<html><body><p>Text only</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Audios) != 0 {
			t.Errorf("expected 0 audios, got %d", len(result.Audios))
		}
	})
}
