package html_test

// extract_format_matrix_test.go — pins the combined behavior of the inline
// format switches and Preserve* flags. These axes used to be handled by two
// near-duplicate production branches; the table below is the contract that
// matters, and it fails if a refactor shifts which resources are returned or
// which placeholders reach Text.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

const formatMatrixDoc = `<html><head><title>T</title></head><body>
	<article><h1>H</h1>
	<p>Para with <a href="/l" title="tt">link text</a>.</p>
	<img src="/i.png" alt="alt text" title="it" width="10" height="20">
	<p><a href="/l2">second</a></p>
	</article></body></html>`

func TestFormatMatrixPlaceholdersAndResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		imgFmt   string
		lnkFmt   string
		preserve bool
		wantText []string
		denyText []string
		wantImgs int
		wantLnks int
	}{
		{
			name:     "both none: no tokens, resources kept",
			imgFmt:   "none",
			lnkFmt:   "none",
			preserve: true,
			wantImgs: 1,
			wantLnks: 2,
		},
		{
			name:     "markdown images and links substituted",
			imgFmt:   "markdown",
			lnkFmt:   "markdown",
			preserve: true,
			wantText: []string{"![alt text](/i.png)", "[link text](/l)", "[second](/l2)"},
			denyText: []string{"[IMAGE:", "[LINK:", "[/LINK]"},
			wantImgs: 1,
			wantLnks: 2,
		},
		{
			name:     "only images formatted: link tokens must not leak",
			imgFmt:   "markdown",
			lnkFmt:   "none",
			preserve: true,
			wantText: []string{"![alt text](/i.png)", "link text"},
			denyText: []string{"[IMAGE:", "[LINK:", "[/LINK]"},
			wantImgs: 1,
			wantLnks: 2,
		},
		{
			name:     "only links formatted: image tokens must not leak",
			imgFmt:   "none",
			lnkFmt:   "markdown",
			preserve: true,
			wantText: []string{"[link text](/l)"},
			denyText: []string{"[IMAGE:", "[LINK:", "[/LINK]"},
			wantImgs: 1,
			wantLnks: 2,
		},
		{
			name:     "placeholder keeps image markers visible",
			imgFmt:   "placeholder",
			lnkFmt:   "none",
			preserve: true,
			wantText: []string{"[IMAGE:1]"},
			wantImgs: 1,
			wantLnks: 2,
		},
		{
			name:     "html formats escape into markup",
			imgFmt:   "html",
			lnkFmt:   "html",
			preserve: true,
			wantText: []string{
				`<img src="/i.png" alt="alt text" title="it" width="10" height="20">`,
				`<a href="/l" title="tt">link text</a>`,
			},
			denyText: []string{"[IMAGE:", "[LINK:"},
			wantImgs: 1,
			wantLnks: 2,
		},
		{
			name:     "unpreserved resources are still consumed by the formatter",
			imgFmt:   "markdown",
			lnkFmt:   "markdown",
			preserve: false,
			wantText: []string{"![alt text](/i.png)", "[link text](/l)"},
			denyText: []string{"[IMAGE:", "[LINK:"},
			wantImgs: 0,
			wantLnks: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultConfig()
			cfg.MaxCacheEntries = 0
			cfg.InlineImageFormat = tt.imgFmt
			cfg.InlineLinkFormat = tt.lnkFmt
			cfg.PreserveImages = tt.preserve
			cfg.PreserveLinks = tt.preserve

			res, err := html.Extract([]byte(formatMatrixDoc), cfg)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(res.Text, want) {
					t.Errorf("Text missing %q:\n%s", want, res.Text)
				}
			}
			for _, deny := range tt.denyText {
				if strings.Contains(res.Text, deny) {
					t.Errorf("Text contains %q:\n%s", deny, res.Text)
				}
			}
			if len(res.Images) != tt.wantImgs {
				t.Errorf("len(Images) = %d, want %d", len(res.Images), tt.wantImgs)
			}
			if len(res.Links) != tt.wantLnks {
				t.Errorf("len(Links) = %d, want %d", len(res.Links), tt.wantLnks)
			}
		})
	}
}
