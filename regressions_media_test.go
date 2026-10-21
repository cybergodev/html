package html

// regressions_media_test.go — regressions for the D-002 media extraction and
// ownership fixes, gathered from the former single-topic files:
//
//   - iframe/embed <source> attribute scanning no longer lets a decoy value
//     inside another attribute's quoted value hijack the real src, nor does a
//     literal '>' inside a quoted value end the tag scan early;
//   - the inline-image HTML formatter keeps the title attribute;
//   - media URLs retained in the Result (and the cached Result) never alias the
//     caller's input buffer, on the zero-copy decode path.

import (
	"strings"
	"testing"
)

func TestIframeSourceNotHijackedByQuotedTitle(t *testing.T) {
	t.Parallel()

	hasVideo := func(res *Result, substr string) bool {
		for _, v := range res.Videos {
			if strings.Contains(v.URL, substr) {
				return true
			}
		}
		return false
	}

	// Decoy attribute value that reads like an unquoted source; pre-fix, the
	// scanner returned `here"` from inside the title, failed validation, and
	// returned — the real src was never read.
	doc := `<html><body><iframe title="video src=here" src="https://cdn.example.com/embed/real.mp4"></iframe></body></html>`
	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !hasVideo(res, "cdn.example.com/embed/real.mp4") {
		t.Errorf("real iframe src not extracted (videos: %v)", res.Videos)
	}

	// Decoy containing a fully valid-looking video URL inside the title:
	// pre-fix the decoy won the first-match race and real2 was lost entirely.
	doc2 := `<html><body><iframe title="demo src=https://evil.example.com/x.mp4 here" src="https://cdn.example.com/embed/real2.mp4"></iframe></body></html>`
	res2, err := Extract([]byte(doc2))
	if err != nil {
		t.Fatalf("Extract(2): %v", err)
	}
	if !hasVideo(res2, "cdn.example.com/embed/real2.mp4") {
		t.Errorf("real iframe src lost to a quoted-value decoy (videos: %v)", res2.Videos)
	}
}

// mediaAliasingDoc carries every media URL shape that reaches the result:
// an iframe (raw-HTML attribute path), an embed and an object element (both
// raw-HTML), a <video src>, an <audio src>, and an mp3 that appears only as
// untagged text inside a script body (the ScanMediaURLs path).
const mediaAliasingDoc = `<html><body>
<iframe src="https://www.youtube.com/embed/abc123"></iframe>
<embed src="https://cdn.example.com/clip.webm">
<object data="https://cdn.example.com/obj.mp4"></object>
<video src="https://cdn.example.com/movie.mp4"></video>
<audio src="https://cdn.example.com/song.ogg"></audio>
<script>var cfg = {"voice":"https://cdn.example.com/narration.mp3"};</script>
</body></html>`

// scrubInput overwrites every ASCII letter of buf, which is what a caller
// recycling an input buffer does.
func scrubInput(buf []byte) {
	for i := range buf {
		if c := buf[i]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			buf[i] = 'Z'
		}
	}
}

func mediaAliasingProcessor(t *testing.T) *Processor {
	t.Helper()
	// Cache off: with it on, the second extraction could be served from the
	// cache and the assertion would test cloneResult instead of the buffer
	// ownership contract this file is about. ProcessingTimeout off keeps the
	// zero-copy decode path (see detectEncoding); with a timeout the decoded
	// document is already a private copy and the media paths could not alias
	// the caller's buffer, so the clone these tests guard would go unexercised.
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = 0
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestMediaURLsNotAliasedWithInputBuffer pins the ownership contract: mutating
// the input buffer after Extract must not change any URL in the Result.
func TestMediaURLsNotAliasedWithInputBuffer(t *testing.T) {
	t.Parallel()

	p := mediaAliasingProcessor(t)

	buf := []byte(mediaAliasingDoc)
	result, err := p.Extract(buf)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(result.Videos) == 0 || len(result.Audios) == 0 {
		t.Fatalf("test document produced no media: videos=%d audios=%d",
			len(result.Videos), len(result.Audios))
	}

	videosBefore := make([]string, len(result.Videos))
	for i, v := range result.Videos {
		// strings.Clone is required here: copying the string header alone would
		// leave the snapshot pointing at the very bytes the scrub overwrites,
		// so the comparison below would compare a value with itself.
		videosBefore[i] = strings.Clone(v.URL)
	}
	audiosBefore := make([]string, len(result.Audios))
	for i, a := range result.Audios {
		audiosBefore[i] = strings.Clone(a.URL)
	}

	scrubInput(buf)

	for i, v := range result.Videos {
		if v.URL != videosBefore[i] {
			t.Errorf("Video[%d].URL changed after the input buffer was reused: %q -> %q",
				i, videosBefore[i], v.URL)
		}
		if v.Type == "" {
			t.Errorf("Video[%d].Type unexpectedly empty", i)
		}
	}
	for i, a := range result.Audios {
		if a.URL != audiosBefore[i] {
			t.Errorf("Audio[%d].URL changed after the input buffer was reused: %q -> %q",
				i, audiosBefore[i], a.URL)
		}
	}
}

// TestCachedMediaURLsNotAliasedWithInputBuffer covers the second consequence:
// the cached entry must not point into the caller's buffer either. A cache hit
// serves a clone of the cached Result, so if the cached URLs aliased the first
// caller's input, the clone would too.
func TestCachedMediaURLsNotAliasedWithInputBuffer(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 16
	// ProcessingTimeout off for the same reason as mediaAliasingProcessor: the
	// aliasing under test only exists on the zero-copy decode path, and a cache
	// hit would otherwise mask it with cloneResult's fresh strings.
	cfg.ProcessingTimeout = 0
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	buf := []byte(mediaAliasingDoc)
	first, err := p.Extract(buf)
	if err != nil {
		t.Fatalf("first Extract: %v", err)
	}
	if len(first.Videos) == 0 {
		t.Fatal("test document produced no videos")
	}
	// Cloned for the same reason as above: a bare header copy would alias the
	// bytes the next statement overwrites.
	before := strings.Clone(first.Videos[0].URL)

	// The caller reuses its buffer exactly as the API permits.
	scrubInput(buf)

	// Same bytes as the first call, so this is a cache hit.
	second, err := p.Extract(append([]byte(nil), mediaAliasingDoc...))
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}
	if len(second.Videos) == 0 {
		t.Fatal("second extraction produced no videos")
	}
	if got := second.Videos[0].URL; got != before {
		t.Fatalf("cached media URL aliased the first caller's input buffer: %q -> %q", before, got)
	}
}

func TestIframeSourceAfterQuotedGT(t *testing.T) {
	t.Parallel()

	// A '>' inside an earlier quoted attribute value previously ended the
	// tag scan early, so the real src after it was never read.
	doc := `<html><body><iframe title="a > b" src="https://cdn.example.com/embed/q.mp4"></iframe></body></html>`
	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	found := false
	for _, v := range res.Videos {
		if strings.Contains(v.URL, "cdn.example.com/embed/q.mp4") {
			found = true
		}
	}
	if !found {
		t.Errorf("iframe src after a quoted '>' not extracted (videos: %v)", res.Videos)
	}
}

func TestInlineImageHTMLTitle(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "html"

	doc := `<html><body><p><img src="a.png" alt="pic" title="Photo of X"></p></body></html>`
	res, err := Extract([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if want := `<img src="a.png" alt="pic" title="Photo of X">`; !strings.Contains(res.Text, want) {
		t.Errorf("inline html image missing title attribute; Text = %q", res.Text)
	}
}
