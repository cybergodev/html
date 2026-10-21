package html_test

// media_type_test.go — pins VideoInfo.Type / AudioInfo.Type. They used to be
// populated only when the markup declared a type (<source type>, <embed type>)
// or for iframe embeds, so the common "<video src=movie.mp4>" shape and every
// video found by the raw-HTML scan reported an empty Type.

import (
	"testing"

	"github.com/cybergodev/html"
)

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
