package html_test

// media_kind_matrix_test.go — pins the media matrix that Processor.extractMedia
// now serves through its wantVideos / wantAudios flags. The previous shape ran
// the full combined extraction and discarded half of it (including one whole
// raw-HTML media scan), so a single-kind configuration silently paid for the
// other kind; these assertions would fail if a flag stopped gating its branches.

import (
	"testing"

	"github.com/cybergodev/html"
)

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
