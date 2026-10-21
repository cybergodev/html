package html

// source_flag_regression_test.go — regression for the D-002 P1 fix: the
// <source> dispatch gate in ExtractAllLinks admitted the element when EITHER
// IncludeVideos or IncludeAudios was set, and extractSourceLinks classified
// the resource by MIME type without re-checking the matching flag — so with
// IncludeVideos=false and IncludeAudios=true, a video <source> still appeared
// in the results as Type "video".

import (
	"testing"
)

const sourceFlagDoc = `<html><body>
	<video controls><source src="/movie.mp4" type="video/mp4"></video>
	<audio controls><source src="/theme.mp3" type="audio/mpeg"></audio>
</body></html>`

func TestSourceRespectsPerTypeIncludeFlags(t *testing.T) {
	t.Parallel()

	findByURL := func(links []LinkResource, url string) *LinkResource {
		for i := range links {
			if links[i].URL == url {
				return &links[i]
			}
		}
		return nil
	}

	// Videos disabled, audios enabled: the video <source> must not appear,
	// the audio <source> must.
	cfg := DefaultConfig()
	cfg.IncludeVideos = false
	cfg.IncludeAudios = true
	res, err := ExtractAllLinks([]byte(sourceFlagDoc), cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}
	if m := findByURL(res, "/movie.mp4"); m != nil {
		t.Errorf("video <source> leaked despite IncludeVideos=false: %+v", m)
	}
	if a := findByURL(res, "/theme.mp3"); a == nil {
		t.Errorf("audio <source> missing despite IncludeAudios=true (got %d links)", len(res))
	} else if a.Type != "audio" {
		t.Errorf("audio <source> typed %q, want %q", a.Type, "audio")
	}

	// Symmetric case: videos enabled, audios disabled.
	cfg2 := DefaultConfig()
	cfg2.IncludeVideos = true
	cfg2.IncludeAudios = false
	res2, err := ExtractAllLinks([]byte(sourceFlagDoc), cfg2)
	if err != nil {
		t.Fatalf("ExtractAllLinks(2): %v", err)
	}
	if m := findByURL(res2, "/movie.mp4"); m == nil {
		t.Errorf("video <source> missing despite IncludeVideos=true (got %d links)", len(res2))
	} else if m.Type != "video" {
		t.Errorf("video <source> typed %q, want %q", m.Type, "video")
	}
	if a := findByURL(res2, "/theme.mp3"); a != nil {
		t.Errorf("audio <source> leaked despite IncludeAudios=false: %+v", a)
	}
}
