// html_links_test.go - link and media extraction tests (collection, URL validation, inline formatting).

package html_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

func TestLinkExtraction(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer func() { _ = p.Close() }()

	t.Run("links extracted with details", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="https://example.com" title="Example">External Link</a>
				<a href="/internal" rel="nofollow">Internal Link</a>
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Links) != 2 {
			t.Errorf("Got %d links, want 2", len(result.Links))
		}
		if !result.Links[0].IsExternal {
			t.Error("First link should be external")
		}
		if !result.Links[1].IsNoFollow {
			t.Error("Second link should have nofollow")
		}
	})

	t.Run("empty link handled", func(t *testing.T) {
		htmlContent := `<html><body><a href=""></a></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Links) != 0 {
			t.Errorf("Got %d links, want 0", len(result.Links))
		}
	})
}

func TestExtractAllLinks(t *testing.T) {
	t.Parallel()

	t.Run("comprehensive link extraction", func(t *testing.T) {
		htmlContent := `
			<html>
			<head>
				<link rel="stylesheet" href="styles.css">
				<script src="app.js"></script>
			</head>
			<body>
				<a href="https://example.com">External Link</a>
				<a href="/internal">Internal Link</a>
				<img src="image.jpg" alt="Test Image">
				<video src="video.mp4"></video>
			</body>
			</html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) == 0 {
			t.Fatal("ExtractAllLinks() returned no links")
		}

		types := make(map[string]bool)
		for _, link := range links {
			types[link.Type] = true
		}

		expectedTypes := []string{"css", "js", "link", "image", "video"}
		for _, expectedType := range expectedTypes {
			if !types[expectedType] {
				t.Errorf("Expected link type %q not found", expectedType)
			}
		}
	})

	t.Run("link deduplication", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="page.html">Link 1</a>
				<a href="page.html">Link 2</a>
				<img src="img.jpg">
				<img src="img.jpg" alt="Same">
			</body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		urlCounts := make(map[string]int)
		for _, link := range links {
			urlCounts[link.URL]++
		}

		for url, count := range urlCounts {
			if count > 1 {
				t.Errorf("URL %q appears %d times (should be deduplicated)", url, count)
			}
		}
	})

	t.Run("relative URL resolution with manual base URL", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="/about">About</a>
				<a href="contact.html">Contact</a>
				<img src="images/logo.jpg">
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.BaseURL = "https://mysite.com/"
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()

		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		expectedURLs := map[string]bool{
			"https://mysite.com/about":           false,
			"https://mysite.com/contact.html":    false,
			"https://mysite.com/images/logo.jpg": false,
		}

		for _, link := range links {
			if _, exists := expectedURLs[link.URL]; exists {
				expectedURLs[link.URL] = true
			}
		}

		for url, found := range expectedURLs {
			if !found {
				t.Errorf("Expected URL %q not found", url)
			}
		}
	})

	t.Run("selective extraction by type", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<img src="image.jpg">
				<video src="video.mp4"></video>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeImages = false
		cfg.IncludeVideos = false

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		for _, link := range links {
			if link.Type == "image" {
				t.Error("Image links should not be included")
			}
			if link.Type == "video" {
				t.Error("Video links should not be included")
			}
		}
	})
}

func TestImageFormatting(t *testing.T) {
	t.Parallel()

	t.Run("markdown inline images", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<p>Text before</p>
				<img src="image1.jpg" alt="First Image">
				<p>Text middle</p>
				<img src="image2.png" alt="Second Image">
				<p>Text after</p>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "markdown"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "![First Image](image1.jpg)") {
			t.Error("Should contain markdown inline image 1")
		}
		if !strings.Contains(result.Text, "![Second Image](image2.png)") {
			t.Error("Should contain markdown inline image 2")
		}
	})

	t.Run("HTML inline images", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<p>Text before</p>
				<img src="img.jpg" alt="Test" width="100" height="50">
				<p>Text after</p>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, `<img src="img.jpg"`) {
			t.Error("Should contain HTML img tag")
		}
		if !strings.Contains(result.Text, `alt="Test"`) {
			t.Error("Should preserve alt attribute")
		}
		if !strings.Contains(result.Text, `width="100"`) {
			t.Error("Should preserve width attribute")
		}
		if !strings.Contains(result.Text, `height="50"`) {
			t.Error("Should preserve height attribute")
		}
	})

	t.Run("no inline format", func(t *testing.T) {
		htmlContent := `<html><body><img src="image.jpg"><p>Text</p></body></html>`

		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "none"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if strings.Contains(result.Text, "[IMAGE:") {
			t.Error("Should not contain image placeholders")
		}
		if len(result.Images) == 0 {
			t.Error("Should still extract images to Images array")
		}
	})
}

func TestLinkFormatting(t *testing.T) {
	t.Parallel()

	// Each case builds its own processor (so subtests run in parallel) and
	// asserts inline-link formatting via required/forbidden substrings and an
	// optional minimum extracted-links count.
	tests := []struct {
		name            string
		html            string
		modify          func(*html.Config)
		wantContains    []string
		wantNotContains []string
		wantLinksMin    int
	}{
		{
			name: "markdown inline links",
			html: `
			<html><body>
				<p>Text before</p>
				<a href="https://go.dev/tour/">Go Tour</a>
				<p>Text middle</p>
				<a href="https://golang.org">Golang</a>
				<p>Text after</p>
			</body></html>
		`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{"[Go Tour](https://go.dev/tour/)", "[Golang](https://golang.org)"},
		},
		{
			name: "html inline links",
			html: `
			<html><body>
				<p>Text before</p>
				<a href="https://go.dev/tour/" title="Go Tour">Go Tour</a>
				<p>Text after</p>
			</body></html>
		`,
			modify: func(c *html.Config) { c.InlineLinkFormat = "html" },
			wantContains: []string{
				`<a href="https://go.dev/tour/"`,
				`title="Go Tour"`,
				`>Go Tour</a>`,
			},
		},
		{
			name:            "none format default",
			html:            `<html><body><a href="https://go.dev">Go</a><p>Text</p></body></html>`,
			modify:          func(c *html.Config) { c.InlineLinkFormat = "none" },
			wantNotContains: []string{"[LINK:"},
			wantContains:    []string{"Go"},
			wantLinksMin:    1,
		},
		{
			name:         "empty href handling",
			html:         `<html><body><a href="">Empty Link</a><a href="https://go.dev">Valid Link</a></body></html>`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{"[Valid Link](https://go.dev)"},
		},
		{
			name:         "empty text handling",
			html:         `<html><body><a href="https://go.dev"></a></body></html>`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{"[Link 1](https://go.dev)"}, // fallback text for empty link
		},
		{
			name: "combined with image format",
			html: `
			<html><body>
				<p>Text before</p>
				<img src="image.jpg" alt="Test Image">
				<a href="https://go.dev">Go</a>
				<p>Text after</p>
			</body></html>
		`,
			modify: func(c *html.Config) {
				c.InlineImageFormat = "markdown"
				c.InlineLinkFormat = "markdown"
			},
			wantContains: []string{"![Test Image](image.jpg)", "[Go](https://go.dev)"},
		},
		{
			name:         "html escapes special characters",
			html:         `<html><body><a href="https://go.dev?a=1&b=2">Link with "quotes"</a></body></html>`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "html" },
			wantContains: []string{`href="https://go.dev?a=1&amp;b=2"`, `>Link with &#34;quotes&#34;</a>`},
		},
		{
			// A literal, unpaired "[LINK:n]" token in the source text must not
			// cause all following content to be dropped during link formatting.
			name: "unclosed link placeholder preserves trailing text",
			html: `<html><body><article>
<p>Intro text here.</p>
<a href="https://go.dev">Go</a>
<p>See [LINK:5] for notes. Trailing content must remain.</p>
</article></body></html>`,
			modify: func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{
				"[Go](https://go.dev)",
				"[LINK:5]",                      // unclosed placeholder preserved verbatim
				"Trailing content must remain.", // trailing text not dropped
			},
		},
		{
			// Many literal, unpaired "[LINK:n]" tokens exercise the
			// noEndTagFrom fast path in formatInlineLinks: after the first
			// failed closing-tag scan, every later token's scan is skipped
			// entirely. All tokens must still be preserved verbatim — both
			// those whose number matches a real link and those that do not —
			// and no surrounding content may be dropped.
			name: "multiple unclosed link placeholders preserved verbatim",
			html: `<html><body><article>
<a href="https://go.dev">Go</a>
<p>See [LINK:1] and [LINK:2] and [LINK:3] for notes.</p>
</article></body></html>`,
			modify: func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{
				"[Go](https://go.dev)",
				"[LINK:1]", // position matches the real link; still verbatim when unclosed
				"[LINK:2]", "[LINK:3]",
				"for notes.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := html.DefaultConfig()
			if tt.modify != nil {
				tt.modify(&cfg)
			}
			p, err := html.New(cfg)
			if err != nil {
				t.Fatalf("html.New() failed: %v", err)
			}
			defer func() { _ = p.Close() }()

			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result.Text, want) {
					t.Errorf("expected text to contain %q, got: %s", want, result.Text)
				}
			}
			for _, avoid := range tt.wantNotContains {
				if strings.Contains(result.Text, avoid) {
					t.Errorf("expected text to NOT contain %q, got: %s", avoid, result.Text)
				}
			}
			if tt.wantLinksMin > 0 && len(result.Links) < tt.wantLinksMin {
				t.Errorf("expected at least %d extracted links, got %d", tt.wantLinksMin, len(result.Links))
			}
		})
	}
}

func TestExtractAllLinksComprehensive(t *testing.T) {
	t.Parallel()

	t.Run("extract source links from video/audio tags", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video>
					<source src="video1.mp4" type="video/mp4">
					<source src="video2.webm" type="video/webm">
				</video>
				<audio>
					<source src="audio1.mp3" type="audio/mpeg">
					<source src="audio2.ogg" type="audio/ogg">
				</audio>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true
		cfg.IncludeAudios = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) < 4 {
			t.Errorf("Got %d links, want at least 4", len(links))
		}

		// Check that video sources are extracted
		hasVideo1 := false
		hasVideo2 := false
		for _, link := range links {
			if link.Type == "video" {
				if link.URL == "video1.mp4" {
					hasVideo1 = true
				}
				if link.URL == "video2.webm" {
					hasVideo2 = true
				}
			}
		}
		if !hasVideo1 {
			t.Error("Should extract video1.mp4 source")
		}
		if !hasVideo2 {
			t.Error("Should extract video2.webm source")
		}
	})

	t.Run("extract embed video links", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<iframe src="https://www.youtube.com/embed/test123"></iframe>
				<embed src="video.mp4" type="video/mp4"></embed>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) == 0 {
			t.Error("Should extract embed video links")
		}

		// Check that embed links are extracted
		hasEmbed := false
		for _, link := range links {
			if link.Type == "video" {
				hasEmbed = true
			}
		}
		if !hasEmbed {
			t.Error("Should extract embed as video")
		}
	})
}

func TestURLValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty URL is invalid", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		htmlContent := `<html><body><img src=""></body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		// Empty src must not produce any image entry at all.
		if len(result.Images) != 0 {
			t.Errorf("empty src should produce no images, got %d", len(result.Images))
		}
	})

	t.Run("valid absolute URLs", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		htmlContent := `<html><body>
			<img src="http://example.com/image.jpg">
			<img src="https://example.com/image.png">
			<a href="https://example.com/page">Link</a>
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 2 {
			t.Errorf("Got %d images, want 2", len(result.Images))
		}
		if len(result.Links) != 1 {
			t.Errorf("Got %d links, want 1", len(result.Links))
		}
	})

	t.Run("valid relative URLs", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		htmlContent := `<html><body>
			<img src="/images/photo.jpg">
			<img src="./relative.png">
			<img src="image.gif">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 3 {
			t.Errorf("Got %d images, want 3", len(result.Images))
		}
	})

	t.Run("protocol-relative URLs", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		htmlContent := `<html><body>
			<img src="//example.com/image.jpg">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 1 {
			t.Errorf("Got %d images, want 1", len(result.Images))
		}
	})

	t.Run("alphanumeric path URLs", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		htmlContent := `<html><body>
			<img src="img1.jpg">
			<img src="photo123.png">
			<img src="Image.GIF">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 3 {
			t.Errorf("Got %d images, want 3", len(result.Images))
		}
	})

	t.Run("data URL with special characters is rejected", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		// Data URLs with control characters should be rejected
		htmlContent := `<html><body>
			<img src="data:image/png;base64,invalid<>chars">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		// Should not extract images with invalid data URLs
		for _, img := range result.Images {
			if strings.Contains(img.URL, "<>") {
				t.Error("Should reject data URL with special characters")
			}
		}
	})

	t.Run("URLs with dangerous characters are rejected", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()
		htmlContent := `<html><body>
			<img src="image.jpg<script>">
			<img src="photo.png onclick="attack()">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		// Should not extract URLs with dangerous characters
		for _, img := range result.Images {
			if strings.ContainsAny(img.URL, "<>\"'") {
				t.Errorf("Should reject URL with dangerous characters: %s", img.URL)
			}
		}
	})
}

func TestLinkTagExtraction(t *testing.T) {
	t.Parallel()

	t.Run("extract all link rel types", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="style.css">
				<link rel="icon" href="favicon.ico">
				<link rel="shortcut icon" href="favicon2.ico">
				<link rel="apple-touch-icon" href="apple.png">
				<link rel="apple-touch-icon-precomposed" href="apple2.png">
				<link rel="canonical" href="https://example.com">
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeCSS = true
		cfg.IncludeIcons = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should have CSS and icons
		hasCSS := false
		hasIcon := false
		for _, link := range links {
			if link.Type == "css" {
				hasCSS = true
			}
			if link.Type == "icon" {
				hasIcon = true
			}
		}
		if !hasCSS {
			t.Error("Should extract stylesheet link")
		}
		if !hasIcon {
			t.Error("Should extract icon links")
		}
	})

	t.Run("extract preload links by resource type", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="preload" as="style" href="preload.css">
				<link rel="preload" as="script" href="preload.js">
				<link rel="preload" as="image" href="preload.jpg">
				<link rel="preload" as="video" href="preload.mp4">
				<link rel="preload" as="audio" href="preload.mp3">
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeCSS = true
		cfg.IncludeJS = true
		cfg.IncludeImages = true
		cfg.IncludeVideos = true
		cfg.IncludeAudios = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should have all types
		typeCounts := make(map[string]int)
		for _, link := range links {
			typeCounts[link.Type]++
		}

		if typeCounts["css"] != 1 {
			t.Errorf("Got %d CSS links, want 1", typeCounts["css"])
		}
		if typeCounts["js"] != 1 {
			t.Errorf("Got %d JS links, want 1", typeCounts["js"])
		}
		if typeCounts["image"] != 1 {
			t.Errorf("Got %d image links, want 1", typeCounts["image"])
		}
		if typeCounts["video"] != 1 {
			t.Errorf("Got %d video links, want 1", typeCounts["video"])
		}
		if typeCounts["audio"] != 1 {
			t.Errorf("Got %d audio links, want 1", typeCounts["audio"])
		}
	})

	t.Run("extract prefetch/preconnect/dns-prefetch links", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="prefetch" as="style" href="prefetch.css">
				<link rel="preconnect" as="script" href="preconnect.js">
				<link rel="dns-prefetch" as="image" href="dns.jpg">
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeCSS = true
		cfg.IncludeJS = true
		cfg.IncludeImages = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should have all prefetch types
		if len(links) != 3 {
			t.Errorf("Got %d links, want 3", len(links))
		}
	})

	t.Run("type-based detection for link tags", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="alternate" type="text/css" href="alt-style.css">
				<link rel="alternate" type="text/javascript" href="alt-script.js">
			</head><body></body></html>
		`

		config := html.DefaultConfig()
		config.IncludeCSS = true
		config.IncludeJS = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should detect by type attribute
		typeCounts := make(map[string]int)
		for _, link := range links {
			typeCounts[link.Type]++
		}

		if typeCounts["css"] != 1 {
			t.Errorf("Got %d CSS links by type, want 1", typeCounts["css"])
		}
		if typeCounts["js"] != 1 {
			t.Errorf("Got %d JS links by type, want 1", typeCounts["js"])
		}
	})

	t.Run("link tag without href is ignored", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet">
			</head><body></body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) != 0 {
			t.Errorf("Got %d links, want 0", len(links))
		}
	})

	t.Run("link tag with data URL", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="data:image/png;base64,iVBORw0KGgo=">
				<link rel="stylesheet" href="data:text/css;base64,Ym9keSB7YmFja2dyb3VuZDogcmVkO30=">
			</head><body></body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Whitelisted data URLs are extracted; non-whitelisted media types
		// (text/css) are rejected by IsValidURL, which now enforces the same
		// safeMediaTypes policy as the DOM sanitizer.
		foundSafe, foundUnsafe := false, false
		for _, link := range links {
			if strings.HasPrefix(link.URL, "data:image/png") {
				foundSafe = true
			}
			if strings.HasPrefix(link.URL, "data:text/css") {
				foundUnsafe = true
			}
		}
		if !foundSafe {
			t.Error("Should extract data URL links with a whitelisted media type")
		}
		if foundUnsafe {
			t.Error("Should reject data URL links with a non-whitelisted media type")
		}
	})

	t.Run("link tag with title uses title", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="style.css" title="My Stylesheet">
			</head><body></body></html>
		`

		config := html.DefaultConfig()
		config.IncludeCSS = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) != 1 {
			t.Fatalf("Got %d links, want 1", len(links))
		}

		if links[0].Title != "My Stylesheet" {
			t.Errorf("Title = %q, want 'My Stylesheet'", links[0].Title)
		}
	})

	t.Run("link tag without title uses filename", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="/path/to/style.css">
			</head><body></body></html>
		`

		config := html.DefaultConfig()
		config.IncludeCSS = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) != 1 {
			t.Fatalf("Got %d links, want 1", len(links))
		}

		if links[0].Title != "style.css" {
			t.Errorf("Title = %q, want 'style.css'", links[0].Title)
		}
	})
}

func TestURLAndDomainExtraction(t *testing.T) {
	t.Parallel()

	t.Run("protocol-relative URL resolution", func(t *testing.T) {
		// This tests resolveURL with protocol-relative URLs
		htmlContent := `
			<html><head>
				<base href="https://example.com/path/">
			</head>
			<body>
				<a href="//cdn.example.com/resource">CDN Link</a>
				<a href="/absolute/path">Absolute Link</a>
				<a href="relative.html">Relative Link</a>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.ResolveRelativeURLs = true
		cfg.IncludeContentLinks = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Each href shape must resolve as observed: root- and document-relative
		// hrefs resolve against the default base (the <base> tag does not feed
		// ExtractAllLinks), while the protocol-relative href stays as-is.
		wantURLs := map[string]bool{
			"//cdn.example.com/resource":        false,
			"https://example.com/absolute/path": false,
			"https://example.com/relative.html": false,
		}
		for _, link := range links {
			if _, ok := wantURLs[link.URL]; ok {
				wantURLs[link.URL] = true
			}
		}
		for url, found := range wantURLs {
			if !found {
				t.Errorf("expected resolved URL %q not found in %d links", url, len(links))
			}
		}
	})

	t.Run("base URL detection from meta tags", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<meta property="og:url" content="https://example.com/canonical">
				<link rel="canonical" href="https://example.com/canonical-link">
			</head>
			<body><p>Content</p></body>
		</html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Meta/link tags must not break extraction; body content survives.
		if !strings.Contains(result.Text, "Content") {
			t.Errorf("expected body content to survive meta-tag extraction, got %q", result.Text)
		}
	})

	t.Run("base URL normalization", func(t *testing.T) {
		// Test various base URL formats
		testCases := []string{
			`<base href="https://example.com">`,
			`<base href="https://example.com/">`,
			`<base href="https://example.com/path">`,
			`<base href="https://example.com/path/">`,
		}

		for _, baseTag := range testCases {
			htmlContent := fmt.Sprintf(`
				<html><head>%s</head>
				<body><p>Content</p></body>
				</html>
			`, baseTag)

			result, err := html.Extract([]byte(htmlContent))
			if err != nil {
				t.Errorf("Failed with base tag %s: %v", baseTag, err)
				continue
			}
			if !strings.Contains(result.Text, "Content") {
				t.Errorf("base tag %s: expected body content, got %q", baseTag, result.Text)
			}
		}
	})
}

func TestScriptAndEmbedLinkExtraction(t *testing.T) {
	t.Parallel()

	t.Run("extract script links", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<script src="script1.js"></script>
				<script src="https://cdn.example.com/script2.js"></script>
				<script src="/path/to/script3.js" title="Main Script"></script>
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeJS = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract script links
		if len(links) != 3 {
			t.Errorf("Got %d script links, want 3", len(links))
		}

		// Check that types are correct
		for _, link := range links {
			if link.Type != "js" {
				t.Errorf("Script link should have type 'js', got '%s'", link.Type)
			}
		}
	})

	t.Run("script tag without src is ignored", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<script>console.log("inline script");</script>
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeJS = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Inline scripts should not produce links
		if len(links) != 0 {
			t.Errorf("Got %d links, want 0 (inline script should not produce link)", len(links))
		}
	})

	t.Run("embed and object link extraction", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<embed src="content.swf" type="application/x-shockwave-flash">
				<object data="video.flv" type="video/x-flv"></object>
				<iframe src="https://example.com/frame"></iframe>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// The iframe src is a valid absolute URL and must be captured even when
		// the surrounding embed/object media are unrecognized.
		if len(links) == 0 {
			t.Error("expected at least the iframe link to be extracted")
		}
	})

	t.Run("source link extraction with various attributes", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video>
					<source src="video1.mp4" type="video/mp4">
					<source src="video2.webm" type="video/webm" media="screen and (min-width:800px)">
					<source src="video3.mobile.mp4">
				</video>
				<audio>
					<source src="audio1.mp3" type="audio/mpeg">
					<source src="audio2.ogg" type="audio/ogg">
				</audio>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true
		cfg.IncludeAudios = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract all source links
		if len(links) < 1 {
			t.Error("Should extract source links")
		}
	})
}

func TestContentLinkExtraction(t *testing.T) {
	t.Parallel()

	t.Run("extract internal vs external links", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="https://example.com/page1">Internal</a>
				<a href="https://other.com/page2">External</a>
				<a href="/absolute">Absolute</a>
				<a href="relative.html">Relative</a>
			</body></html>
		`

		config := html.DefaultConfig()
		config.IncludeContentLinks = true
		config.IncludeExternalLinks = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract all link types
		if len(links) != 4 {
			t.Fatalf("Got %d links, want 4", len(links))
		}
		// Every href, in document order, must appear exactly once. Relative hrefs
		// are resolved against the default base URL by ExtractAllLinks.
		wantURLs := []string{
			"https://example.com/page1",
			"https://other.com/page2",
			"https://example.com/absolute",
			"https://example.com/relative.html",
		}
		gotURLs := make(map[string]int, len(links))
		for _, link := range links {
			gotURLs[link.URL]++
		}
		for _, want := range wantURLs {
			if gotURLs[want] != 1 {
				t.Errorf("link %q found %d times, want exactly 1", want, gotURLs[want])
			}
		}
	})

	t.Run("image link extraction", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<img src="image1.jpg" alt="Image 1">
				<img src="https://example.com/image2.png" alt="Image 2">
				<img src="/path/to/image3.gif" alt="Image 3">
			</body></html>
		`

		config := html.DefaultConfig()
		config.IncludeImages = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract image links
		if len(links) != 3 {
			t.Errorf("Got %d image links, want 3", len(links))
		}

		// Check that types are correct
		for _, link := range links {
			if link.Type != "image" {
				t.Errorf("Image link should have type 'image', got '%s'", link.Type)
			}
		}
	})

	t.Run("link without href is ignored", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a name="anchor"></a>
				<a id="link2"></a>
			</body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Links without href should not be extracted
		if len(links) != 0 {
			t.Errorf("Got %d links, want 0 (links without href)", len(links))
		}
	})
}
