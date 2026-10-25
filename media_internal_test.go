package html

import (
	"strings"
	"testing"

	stdxhtml "golang.org/x/net/html"
)

// TestExtractAttributeValue exercises the unexported tag-attribute parser
// (media.go) across the branches the public media-extraction path only hits
// incidentally: double/single quotes, unclosed quotes, unquoted values, the
// substring-rejection guard, whitespace-after-= handling, and the empty/EOF
// boundaries. This is a string-in/string-out helper, so it is driven directly.
func TestExtractAttributeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tagContent string
		attrName   string
		want       string
	}{
		// Found, double-quoted.
		{"double quoted value", `<source src="a.mp4" type="video/mp4">`, "src", "a.mp4"},
		// Found, single-quoted.
		{"single quoted value", `<source src='b.mp4'>`, "src", "b.mp4"},
		// Uppercase attr name in content must still match a lower-case query.
		{"uppercase attr in content", `<source SRC="c.mp4">`, "src", "c.mp4"},
		// Uppercase query must still match (search is lower-cased).
		{"uppercase attr query", `<source src="d.mp4">`, "SRC", "d.mp4"},
		// Unclosed quote -> opening quote is consumed, return trimmed remainder.
		{"unclosed quote returns remainder", `<source src="e.mp4>`, "src", "e.mp4>"},
		// Unquoted value terminated by whitespace.
		{"unquoted value space-terminated", `<source src=f.mp4 type=video/mp4>`, "src", "f.mp4"},
		// Unquoted value terminated by '>'.
		{"unquoted value angle-terminated", `<source src=g.mp4>`, "src", "g.mp4"},
		// Whitespace between '=' and the value (no space before '=') is skipped.
		{"whitespace after equals", `<source src= "h.mp4">`, "src", "h.mp4"},
		// attr= with nothing after it.
		{"equals at EOF", `<source src=`, "src", ""},
		// Substring guard: "src" inside "data-src" must NOT match because the
		// preceding char ('-') is not whitespace.
		{"substring rejected data-src", `<source data-src="x.mp4">`, "src", ""},
		// Substring guard: full word at pos 0 has no preceding char, so it matches.
		{"attr at position zero", `src="j.mp4"`, "src", "j.mp4"},
		// Attribute genuinely absent.
		{"attribute absent", `<source href="k.html">`, "src", ""},
		// Empty tag content.
		{"empty content", "", "src", ""},
		// Needle longer than haystack.
		{"needle longer than content", `<a>`, "poster", ""},
		// attr name with no '=' present at all.
		{"no equals anywhere", `<video poster>`, "poster", ""},
		// Multiple attrs, second match wins (first scan stops at first match).
		{"first occurrence wins", `<a src="first" src="second">`, "src", "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAttributeValue(tt.tagContent, tt.attrName)
			if got != tt.want {
				t.Errorf("extractAttributeValue(%q, %q) = %q, want %q",
					tt.tagContent, tt.attrName, got, tt.want)
			}
		})
	}
}

// TestIndexTagEnd drives the quoted-attribute tag scanner directly (media.go:
// the public scanners only reach it per byte of a padded document): the first
// '>' inside a quoted value does not end the tag, the closing quote resumes
// the scan, an unterminated quote falls back to the raw first '>' — the
// pre-existing truncation behavior, so a value is still read from a tag whose
// source was cut inside a quoted value — and no '>' at all yields -1.
func TestIndexTagEnd(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		in   string
		want int
	}{
		{"plain tag end", `<iframe src="a">`, len(`<iframe src="a"`)},
		{"quoted gt is not the end", `<img alt="a>b" src="x">`, len(`<img alt="a>b" src="x"`)},
		{"single quotes too", `<img alt='a>b' src='x'>`, len(`<img alt='a>b' src='x'`)},
		{"unterminated quote falls back to raw gt", `<img alt="a>b`, strings.Index(`<img alt="a>b`, ">")},
		{"no gt at all", `<img src="x"`, -1},
	} {
		if got := indexTagEnd(tt.in); got != tt.want {
			t.Errorf("%s: indexTagEnd(%q) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
}

// TestExtractMediaLinkFallbacks drives the media link-title ladder directly
// (the public path always passes a concrete "video"/"audio" kind and a
// path-bearing URL): an explicit title wins, then the last path segment, and
// the capitalized kind — with the unexported empty-kind "Media" default — is
// the final fallback; an invalid or empty src produces no entry at all.
func TestExtractMediaLinkFallbacks(t *testing.T) {
	t.Parallel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	for _, tt := range []struct {
		name      string
		tag       string
		mediaType string
		wantTitle string
		wantEntry bool
	}{
		{"explicit title", `<video src="https://cdn.example.com/v/clip.mp4" title="Keynote"></video>`, "video", "Keynote", true},
		{"last path segment", `<video src="https://cdn.example.com/v/keynote.mp4"></video>`, "video", "keynote.mp4", true},
		// A trailing slash leaves no last segment, so the capitalized kind
		// (and, for the unexported empty kind, "Media") is the fallback.
		{"capitalized kind fallback", `<audio src="https://stream.example.com/"></audio>`, "audio", "Audio", true},
		{"empty kind defaults to Media", `<video src="https://stream.example.com/"></video>`, "", "Media", true},
		{"scriptable src rejected", `<video src="javascript:alert(1)"></video>`, "video", "", false},
		{"empty src rejected", `<video src=""></video>`, "video", "", false},
	} {
		docNode, err := stdxhtml.Parse(strings.NewReader(tt.tag))
		if err != nil {
			t.Fatalf("%s: parse: %v", tt.name, err)
		}
		var node *stdxhtml.Node
		var find func(*stdxhtml.Node)
		find = func(n *stdxhtml.Node) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if node == nil && c.Type == stdxhtml.ElementNode && (c.Data == "video" || c.Data == "audio") {
					node = c
				}
				find(c)
			}
		}
		find(docNode)
		if node == nil {
			t.Fatalf("%s: fixture error: media element not found", tt.name)
		}
		linkMap := map[string]LinkResource{}
		p.extractMediaLink(node, "", linkMap, tt.mediaType)
		if !tt.wantEntry {
			if len(linkMap) != 0 {
				t.Errorf("%s: got entry %+v, want none", tt.name, linkMap)
			}
			continue
		}
		if len(linkMap) != 1 {
			t.Fatalf("%s: got %d entries, want 1", tt.name, len(linkMap))
		}
		for _, l := range linkMap {
			if l.Title != tt.wantTitle {
				t.Errorf("%s: Title = %q, want %q", tt.name, l.Title, tt.wantTitle)
			}
		}
	}
}
