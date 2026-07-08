package internal

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestSanitizeHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
		avoid []string
	}{
		{
			name:  "remove script tags",
			input: `<div>Keep<script>alert('remove')</script>Keep</div>`,
			want:  []string{"Keep", "<div>"},
			avoid: []string{"script", "alert"},
		},
		{
			name:  "remove style tags",
			input: `<div>Keep<style>body{color:red}</style>Keep</div>`,
			want:  []string{"Keep", "<div>"},
			avoid: []string{"style", "color:red"},
		},
		{
			name:  "remove noscript tags",
			input: `<div>Keep<noscript>No JS</noscript>Keep</div>`,
			want:  []string{"Keep", "<div>"},
			avoid: []string{"noscript", "No JS"},
		},
		{
			name:  "remove all three",
			input: `<div>Keep<script>js</script><style>css</style><noscript>nojs</noscript>Keep</div>`,
			want:  []string{"Keep", "<div>"},
			avoid: []string{"script", "style", "noscript", "js", "css", "nojs"},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string{""},
		},
		{
			name:  "no tags to remove",
			input: `<div><p>Normal content</p></div>`,
			want:  []string{"<div>", "<p>", "Normal content"},
		},
		{
			name:  "multiple script tags",
			input: `<div><script>1</script>Keep<script>2</script></div>`,
			want:  []string{"Keep", "<div>"},
			avoid: []string{"script", "1", "2"},
		},
		{
			name:  "nested tags",
			input: `<div><script><span>nested</span></script>Keep</div>`,
			want:  []string{"Keep", "<div>"},
			avoid: []string{"script", "nested"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHTML(tt.input)

			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("SanitizeHTML() result should contain %q, got %q", want, result)
				}
			}

			for _, avoid := range tt.avoid {
				if strings.Contains(result, avoid) {
					t.Errorf("SanitizeHTML() result should not contain %q, got %q", avoid, result)
				}
			}
		})
	}
}

func TestRemoveTagContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		tag     string
		want    []string
		avoid   []string
	}{
		{
			name:    "remove script",
			content: `<div>Before<script>remove</script>After</div>`,
			tag:     "script",
			want:    []string{"Before", "After", "<div>"},
			avoid:   []string{"<script>", "remove", "</script>"},
		},
		{
			name:    "remove style",
			content: `<p>Text<style>css</style>More</p>`,
			tag:     "style",
			want:    []string{"Text", "More", "<p>"},
			avoid:   []string{"<style>", "css", "</style>"},
		},
		{
			name:    "tag not present",
			content: `<div>Content</div>`,
			tag:     "script",
			want:    []string{"<div>", "Content", "</div>"},
		},
		{
			name:    "empty content",
			content: "",
			tag:     "script",
			want:    []string{""},
		},
		{
			name:    "empty tag",
			content: `<div>Content</div>`,
			tag:     "",
			want:    []string{"<div>", "Content", "</div>"},
		},
		{
			name:    "multiple tags",
			content: `<div><script>1</script>Middle<script>2</script>End</div>`,
			tag:     "script",
			want:    []string{"Middle", "End", "<div>"},
			avoid:   []string{"<script>", "1", "2", "</script>"},
		},
		{
			name:    "tag with attributes",
			content: `<div><script type="text/javascript">code</script>Text</div>`,
			tag:     "script",
			want:    []string{"Text", "<div>"},
			avoid:   []string{"<script", "code", "</script>"},
		},
		{
			name:    "unclosed tag",
			content: `<div><script>unclosed</div>`,
			tag:     "script",
			want:    []string{"<div>", "unclosed", "</div>"},
		},
		{
			name:    "tag without closing bracket",
			content: `<div><script`,
			tag:     "script",
			want:    []string{"<div>", "<script"},
		},
		{
			name:    "case insensitive",
			content: `<div><SCRIPT>code</SCRIPT>Text</div>`,
			tag:     "script",
			want:    []string{"Text", "<div>"},
			avoid:   []string{"SCRIPT", "code"},
		},
		{
			name:    "mixed case",
			content: `<div><ScRiPt>code</sCrIpT>Text</div>`,
			tag:     "script",
			want:    []string{"Text", "<div>"},
			avoid:   []string{"code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveTagContent(tt.content, tt.tag)

			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("RemoveTagContent() result should contain %q, got %q", want, result)
				}
			}

			for _, avoid := range tt.avoid {
				if strings.Contains(result, avoid) {
					t.Errorf("RemoveTagContent() result should not contain %q, got %q", avoid, result)
				}
			}
		})
	}
}

func TestRemoveTagContentEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nested same tags", func(t *testing.T) {
		content := `<div><script><script>nested</script></script>Text</div>`
		result := RemoveTagContent(content, "script")

		if !strings.Contains(result, "Text") {
			t.Error("Should keep text after nested tags")
		}
	})

	t.Run("tag in attribute", func(t *testing.T) {
		content := `<div data-script="value">Text</div>`
		result := RemoveTagContent(content, "script")

		if !strings.Contains(result, "Text") {
			t.Error("Should not remove tag name in attributes")
		}
	})

	t.Run("very long content", func(t *testing.T) {
		longText := strings.Repeat("word ", 10000)
		content := `<div>` + longText + `<script>remove</script>` + longText + `</div>`
		result := RemoveTagContent(content, "script")

		if strings.Contains(result, "remove") {
			t.Error("Should remove script even in long content")
		}
		if !strings.Contains(result, "word") {
			t.Error("Should keep long text content")
		}
	})
}

func BenchmarkSanitizeHTML(b *testing.B) {
	htmlContent := `<html><body><div>Content<script>alert('test')</script><style>body{}</style><noscript>No JS</noscript>More</div></body></html>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SanitizeHTML(htmlContent)
	}
}

func BenchmarkRemoveTagContent(b *testing.B) {
	content := `<div>Before<script>remove this content</script>After</div>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoveTagContent(content, "script")
	}
}

func BenchmarkRemoveTagContentLarge(b *testing.B) {
	longText := strings.Repeat("word ", 1000)
	content := `<div>` + longText + `<script>remove</script>` + longText + `</div>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoveTagContent(content, "script")
	}
}

// ============================================================================
// SECURITY TESTS
// ============================================================================

func TestSanitizeHTML_PreventsJavascriptProtocol(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		shouldFail bool
	}{
		{
			name:       "javascript in href",
			input:      `<a href="javascript:alert('xss')">click</a>`,
			shouldFail: false,
		},
		{
			name:       "javascript with mixed case in href",
			input:      `<a href="JavasCript:alert('xss')">click</a>`,
			shouldFail: false,
		},
		{
			name:       "javascript with spaces in href",
			input:      `<a href=" javascript:alert('xss')">click</a>`,
			shouldFail: false,
		},
		{
			name:       "javascript in src",
			input:      `<img src="javascript:alert('xss')">`,
			shouldFail: false,
		},
		{
			name:       "vbscript in href",
			input:      `<a href="vbscript:msgbox('xss')">click</a>`,
			shouldFail: false,
		},
		{
			name:       "file protocol in href",
			input:      `<a href="file:///etc/passwd">click</a>`,
			shouldFail: false,
		},
		{
			name:       "data URL with javascript",
			input:      `<a href="data:text/html,<script>alert('xss')</script>">click</a>`,
			shouldFail: false,
		},
		{
			name:       "valid http link",
			input:      `<a href="https://example.com">click</a>`,
			shouldFail: true,
		},
		{
			name:       "valid relative link",
			input:      `<a href="/path/to/page">click</a>`,
			shouldFail: true,
		},
		{
			name:       "onclick handler",
			input:      `<a href="https://example.com" onclick="alert('xss')">click</a>`,
			shouldFail: false,
		},
		{
			name:       "onerror handler",
			input:      `<img src="invalid.jpg" onerror="alert('xss')">`,
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := SanitizeHTML(tt.input)
			hasJavascript := strings.Contains(strings.ToLower(output), "javascript:")
			hasVbscript := strings.Contains(strings.ToLower(output), "vbscript:")
			hasFile := strings.Contains(strings.ToLower(output), "file:")
			hasOnclick := strings.Contains(strings.ToLower(output), "onclick")
			hasOnerror := strings.Contains(strings.ToLower(output), "onerror")

			hasAnyDangerous := hasJavascript || hasVbscript || hasFile || hasOnclick || hasOnerror

			if tt.shouldFail {
				if hasAnyDangerous {
					t.Errorf("expected safe output, but found dangerous content in: %s", output)
				}
			} else {
				if hasAnyDangerous {
					t.Errorf("dangerous content should be removed, but found in: %s", output)
				}
			}
		})
	}
}

func TestSanitizeHTML_ReturnsEmptyOnError(t *testing.T) {
	invalidInputs := []string{
		"<<<>>>",
		"<div>",
		"<a href='test'>link</a",
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			output := SanitizeHTML(input)
			if output == input {
				t.Errorf("should not return original content on parse error, got: %s", output)
			}
		})
	}
}

func TestSanitizeHTML_MalformedDataURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{
			name:  "valid data URL",
			input: `<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==">`,
			valid: true,
		},
		{
			name:  "data URL with control characters",
			input: "<img src=\"data:image/png;base,\x01\x02\">",
			valid: false,
		},
		{
			name:  "data URL with invalid base64",
			input: `<img src="data:image/png;base64,!!!invalid!!!">`,
			valid: true,
		},
		{
			name:  "data URL with unsafe script content",
			input: `<img src="data:text/html,<script>alert(1)</script>">`,
			valid: false, // Now rejected due to whitelist
		},
		{
			name:  "data URL with unsafe text content",
			input: `<img src="data:text/plain,hello">`,
			valid: false, // Now rejected due to whitelist
		},
		{
			name:  "data URL with safe font content",
			input: `<img src="data:font/woff2;base64,ABC123">`,
			valid: true, // Font types are allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := SanitizeHTML(tt.input)
			if !tt.valid {
				if strings.Contains(output, "data:") {
					t.Errorf("malformed data URL should be removed, but found in: %s", output)
				}
			}
		})
	}
}

// captureAudit is an AuditRecorder that records the reasons URLs were blocked,
// so data-URL validation can assert both the boolean result and the audit event.
type captureAudit struct {
	blockedReasons []string
}

func (c *captureAudit) RecordBlockedTag(string)          {}
func (c *captureAudit) RecordBlockedAttr(string, string) {}
func (c *captureAudit) RecordBlockedURL(_ string, reason string) {
	c.blockedReasons = append(c.blockedReasons, reason)
}

// TestIsValidDataURL_ControlChars exercises isValidDataURLWithAudit at the only
// layer where raw control bytes (including NUL) can be observed. Through
// SanitizeHTML the HTML tokenizer rewrites NUL (\x00) to U+FFFD first, so a
// data URL whose only "dangerous" byte is a NUL survives sanitization; that
// case cannot be asserted via the HTML path and belongs here instead.
func TestIsValidDataURL_ControlChars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantOK  bool
		wantHas bool // whether any block reason is recorded
	}{
		// Non-base64 payload: a single raw control byte (< 0x20) must reject.
		{name: "nul byte in payload", url: "data:image/png;base,abc\x00def", wantOK: false, wantHas: true},
		{name: "x01 in payload", url: "data:image/png;base,abc\x01def", wantOK: false, wantHas: true},
		{name: "del 0x7f in payload", url: "data:image/png;base,abc\x7fdef", wantOK: false, wantHas: true},
		{name: "vertical tab 0x0b in payload", url: "data:image/png;base,ab\x0bcd", wantOK: false, wantHas: true},
		// Per the validator, tab (0x09), LF (0x0a) and CR (0x0d) are allowed in
		// non-base64 payloads, so a payload carrying only those is accepted.
		{name: "tab allowed in payload", url: "data:image/png;base,ab\tcd", wantOK: true, wantHas: false},
		// Base64 payload: any byte outside [A-Za-z0-9+/=] plus CR/LF must reject.
		{name: "nul byte in base64", url: "data:image/png;base64,ABCD\x00EFG=", wantOK: false, wantHas: true},
		{name: "space in base64", url: "data:image/png;base64,ABCD EFG=", wantOK: false, wantHas: true},
		// Valid reference cases.
		{name: "valid base64 png", url: "data:image/png;base64,iVBORw0KGgo=", wantOK: true, wantHas: false},
		{name: "valid font woff2", url: "data:font/woff2;base64,ABC123", wantOK: true, wantHas: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var audit captureAudit
			got := isValidDataURLWithAudit(tt.url, &audit)
			if got != tt.wantOK {
				t.Errorf("isValidDataURLWithAudit(%q) = %v, want %v", tt.url, got, tt.wantOK)
			}
			blocked := len(audit.blockedReasons) > 0
			if blocked != tt.wantHas {
				t.Errorf("isValidDataURLWithAudit(%q) recorded block=%v, want %v (reasons=%v)",
					tt.url, blocked, tt.wantHas, audit.blockedReasons)
			}
		})
	}
}

// TestIsValidDataURL_Structure covers the structural rejects that precede the
// payload scan: missing comma, comma immediately after "data:", and oversized
// URLs. These branches of isValidDataURLWithAudit were previously unexercised.
func TestIsValidDataURL_Structure(t *testing.T) {
	t.Parallel()

	oversize := "data:image/png;base64," + strings.Repeat("A", MaxDataURILength)
	tests := []struct {
		name   string
		url    string
		wantOK bool
	}{
		{name: "not a data url", url: "https://example.com/x.png", wantOK: false},
		{name: "no comma", url: "data:image/pngbase64AAAA", wantOK: false},
		{name: "comma right after scheme", url: "data:,hello", wantOK: false},
		{name: "empty media type base64", url: "data:;base64,AAAA", wantOK: false},
		{name: "empty media type plain", url: "data:,text", wantOK: false},
		{name: "unsafe media type", url: "data:text/html,<b>", wantOK: false},
		{name: "exceeds size limit", url: oversize, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var audit captureAudit
			if got := isValidDataURLWithAudit(tt.url, &audit); got != tt.wantOK {
				t.Errorf("isValidDataURLWithAudit(%q) = %v, want %v", tt.url, got, tt.wantOK)
			}
		})
	}
}

func TestSanitizeHTML_EventHandlers(t *testing.T) {
	eventHandlers := []string{
		"onclick", "onerror", "onload", "onmouseover", "onmouseout",
		"onfocus", "onblur", "onchange", "onsubmit", "onreset", "ondblclick",
	}

	for _, handler := range eventHandlers {
		t.Run(handler, func(t *testing.T) {
			input := `<a href="https://example.com" ` + handler + `="alert('xss')">click</a>`
			output := SanitizeHTML(input)
			if strings.Contains(strings.ToLower(output), strings.ToLower(handler)) {
				t.Errorf("%s handler should be removed, but found in: %s", handler, output)
			}
		})
	}
}

func TestSanitizeHTML_DangerousTags(t *testing.T) {
	dangerousTags := []string{
		"script", "style", "noscript", "iframe", "embed", "object", "input", "button",
		"svg", "math", // SVG and MathML can contain scripts
	}

	for _, tag := range dangerousTags {
		t.Run(tag, func(t *testing.T) {
			input := `<` + tag + `>content</` + tag + `>`
			output := SanitizeHTML(input)
			if strings.Contains(strings.ToLower(output), "<"+tag) {
				t.Errorf("%s tag should be removed, but found in: %s", tag, output)
			}
		})
	}
}

// TestSanitizeHTML_PreservesFormContent ensures <form> and its text content are
// retained during sanitization. Server-side frameworks (ASP.NET WebForms, JSF,
// JSP) wrap the entire page body in a single <form>, so stripping <form> would
// discard all visible content. Form controls (<input>/<button>) are still removed.
func TestSanitizeHTML_PreservesFormContent(t *testing.T) {
	input := `<form action="/submit" method="post"><p>Account summary</p><input type="text"></form>`
	output := SanitizeHTML(input)

	if !strings.Contains(output, "Account summary") {
		t.Errorf("text inside <form> should be preserved, got: %s", output)
	}
	if strings.Contains(strings.ToLower(output), "<input") {
		t.Errorf("<input> control should still be removed, got: %s", output)
	}
}

func TestSanitizeHTML_SVGAndMathMLXSSPrevention(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		forbiddenTag string // dangerous container tag that must be stripped
	}{
		// SVG vectors
		{
			name:         "svg with onload",
			input:        `<svg onload="alert('xss')"><circle cx="50" cy="50" r="50"/></svg>`,
			forbiddenTag: "<svg",
		},
		{
			name:         "svg with script",
			input:        `<svg><script>alert('xss')</script></svg>`,
			forbiddenTag: "<svg",
		},
		{
			name:         "svg with foreignObject",
			input:        `<svg><foreignObject><body onload="alert('xss')"></foreignObject></svg>`,
			forbiddenTag: "<svg",
		},
		{
			name:         "svg with animate",
			input:        `<svg><animate onbegin="alert('xss')"/></svg>`,
			forbiddenTag: "<svg",
		},
		{
			name:         "svg inline event",
			input:        `<svg><set onbegin="alert('xss')"/></svg>`,
			forbiddenTag: "<svg",
		},
		// MathML vectors
		{
			name:         "math with annotation-xml",
			input:        `<math><annotation-xml encoding="application/xhtml+xml"><script>alert('xss')</script></annotation-xml></math>`,
			forbiddenTag: "<math",
		},
		{
			name:         "math with href",
			input:        `<math href="javascript:alert('xss')"><mtext>click</mtext></math>`,
			forbiddenTag: "<math",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := SanitizeHTML(tt.input)
			lower := strings.ToLower(output)
			if strings.Contains(lower, tt.forbiddenTag) {
				t.Errorf("%s tag should be removed, but found in: %s", tt.forbiddenTag, output)
			}
			if strings.Contains(lower, "alert") {
				t.Errorf("Script content should be removed, but found in: %s", output)
			}
		})
	}
}

func TestSanitizeHTML_URIAttributes(t *testing.T) {
	uriAttributes := []string{
		"href", "src", "cite", "action", "data", "formaction", "poster", "background", "longdesc",
	}

	for _, attr := range uriAttributes {
		t.Run(attr, func(t *testing.T) {
			input := `<a ` + attr + `="javascript:alert('xss')">click</a>`
			output := SanitizeHTML(input)
			if strings.Contains(strings.ToLower(output), "javascript:") {
				t.Errorf("javascript: should be removed from %s attribute in: %s", attr, output)
			}
		})
	}
}

func TestSanitizeStyleValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "safe style preserved",
			input: "text-align: center",
			want:  "text-align: center",
		},
		{
			name:  "expression blocked",
			input: "width: expression(alert('xss'))",
			want:  "",
		},
		{
			name:  "behavior blocked",
			input: "color: red; behavior: url(evil.htc)",
			want:  "",
		},
		{
			name:  "moz-binding blocked",
			input: "width: 100px; -moz-binding: url(evil.xml#xss)",
			want:  "",
		},
		{
			name:  "javascript in style blocked",
			input: "background: javascript:alert(1)",
			want:  "",
		},
		{
			name:  "vbscript in style blocked",
			input: "background: vbscript:msgbox(1)",
			want:  "",
		},
		{
			name:  "empty style preserved",
			input: "",
			want:  "",
		},
		{
			name:  "multiple safe properties",
			input: "color: red; font-size: 14px; margin: 10px",
			want:  "color: red; font-size: 14px; margin: 10px",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeStyleValue(tt.input)
			if result != tt.want {
				t.Errorf("sanitizeStyleValue(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}

func TestSanitizeDOM(t *testing.T) {
	t.Parallel()

	t.Run("removes script from DOM tree", func(t *testing.T) {
		doc := mustParseHTML(t, `<div>Keep<script>alert('xss')</script>Keep</div>`)
		SanitizeDOM(doc, NoOpAuditRecorder{})

		result := mustRenderBody(t, doc)
		if strings.Contains(result, "script") {
			t.Errorf("script tag should be removed, got: %s", result)
		}
		if !strings.Contains(result, "Keep") {
			t.Errorf("text should be preserved, got: %s", result)
		}
	})

	t.Run("removes event handlers from DOM", func(t *testing.T) {
		doc := mustParseHTML(t, `<a href="https://example.com" onclick="alert('xss')">click</a>`)
		SanitizeDOM(doc, NoOpAuditRecorder{})

		result := mustRenderBody(t, doc)
		if strings.Contains(strings.ToLower(result), "onclick") {
			t.Errorf("onclick should be removed, got: %s", result)
		}
	})

	t.Run("removes dangerous URIs from DOM", func(t *testing.T) {
		doc := mustParseHTML(t, `<a href="javascript:alert('xss')">click</a>`)
		SanitizeDOM(doc, NoOpAuditRecorder{})

		result := mustRenderBody(t, doc)
		if strings.Contains(strings.ToLower(result), "javascript:") {
			t.Errorf("javascript: URI should be removed, got: %s", result)
		}
	})

	t.Run("preserves safe content in DOM", func(t *testing.T) {
		doc := mustParseHTML(t, `<p>Hello <strong>world</strong></p>`)
		SanitizeDOM(doc, NoOpAuditRecorder{})

		result := mustRenderBody(t, doc)
		if !strings.Contains(result, "Hello") || !strings.Contains(result, "world") {
			t.Errorf("safe content should be preserved, got: %s", result)
		}
	})

	t.Run("nil document is safe", func(t *testing.T) {
		SanitizeDOM(nil, NoOpAuditRecorder{})
	})

	t.Run("sanitizes style attribute", func(t *testing.T) {
		doc := mustParseHTML(t, `<div style="color: red; expression(evil)">text</div>`)
		SanitizeDOM(doc, NoOpAuditRecorder{})

		result := mustRenderBody(t, doc)
		if strings.Contains(result, "expression") {
			t.Errorf("dangerous CSS should be removed, got: %s", result)
		}
	})
}

// mustParseHTML is a test helper that parses an HTML string into a DOM tree.
func mustParseHTML(t *testing.T, content string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}
	return doc
}

// mustRenderBody is a test helper that renders the body content of a DOM tree.
// It handles both full documents (<html><head></head><body>...) and fragment bodies.
func mustRenderBody(t *testing.T, doc *html.Node) string {
	t.Helper()
	body := findBody(t, doc)
	if body == nil {
		return ""
	}
	var buf bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		html.Render(&buf, child)
	}
	return buf.String()
}

// findBody recursively searches for the body element in a DOM tree.
func findBody(t *testing.T, n *html.Node) *html.Node {
	t.Helper()
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findBody(t, child); found != nil {
			return found
		}
	}
	return nil
}

func TestIsSafeURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		safe bool
	}{
		{"empty", "", true},
		{"http", "https://example.com", true},
		{"relative", "/path/to/page", true},
		{"javascript", "javascript:alert(1)", false},
		{"javascript with spaces", " javascript:alert(1)", false},
		{"javascript uppercase", "JAVASCRIPT:alert(1)", false},
		{"vbscript", "vbscript:msgbox(1)", false},
		{"file", "file:///etc/passwd", false},
		{"data valid image", "data:image/png;base64,abc123", true},
		{"data valid font", "data:font/woff2;base64,abc", true},
		{"data unsafe image svg", "data:image/svg+xml;base64,abc", false},
		{"data unsafe script", "data:text/html,<script>alert(1)</script>", false},
		{"data unsafe invalid media", "data:invalid/media,abc", false},
		{"data unsafe text", "data:text/plain,abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSafeURIWithAudit(tt.uri, NoOpAuditRecorder{})
			if result != tt.safe {
				t.Errorf("isSafeURIWithAudit(%q) = %v, want %v", tt.uri, result, tt.safe)
			}
		})
	}
}

func TestIsValidDataURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{"not data URL", "https://example.com", false},
		{"no comma", "data:text/html", false},
		{"just data:", "data:,", false},
		{"valid image png", "data:image/png;base64,iVBORw0KG", true},
		{"valid image gif", "data:image/gif;base64,R0lGODlh", true},
		{"valid font", "data:font/woff2;base64,ABC123", true},
		{"control characters", "data:text/html,\x00\x01", false},
		{"del character", "data:text/html,\x7f", false},
		{"valid base64", "data:image/png;base64,ABC123", true},
		{"invalid base64 chars with control", "data:image/png;base64,\x01\x02", false},
		{"unsafe text html", "data:text/html,<script>", false},
		{"unsafe text plain", "data:text/plain,abc", false},
		{"too large", "data:image/png;base64," + string(make([]byte, 100001)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidDataURLWithAudit(tt.url, NoOpAuditRecorder{})
			if result != tt.valid {
				t.Errorf("isValidDataURLWithAudit(%q) = %v, want %v", tt.url, result, tt.valid)
			}
		})
	}
}

// TestIndexASCIIFold pins the boundary behavior of the unexported indexASCIIFold
// helper used by the case-insensitive tag stripper. The needle's first byte is
// scanned in both cases, so a lower-case needle matches a mixed-case haystack
// (the real callers always pass "<" + lower-case tag).
func TestIndexASCIIFold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		target string
		want   int
	}{
		{"exact match", "hello world", "world", 6},
		{"lowercase needle in uppercase haystack", "HELLO WORLD", "world", 6},
		{"real use: opening script tag", "<SCRIPT>alert</SCRIPT>", "<script", 0},
		{"tag later in content", "abc<Script>", "<script", 3},
		{"first occurrence wins", "<script<script>", "<script", 0},
		{"no match", "nothing here", "world", -1},
		{"target longer than haystack", "abc", "abcd", -1},
		{"empty target returns 0", "anything", "", 0},
		{"empty haystack empty target", "", "", 0},
		{"empty haystack non-empty target", "", "a", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := indexASCIIFold(tt.s, tt.target); got != tt.want {
				t.Errorf("indexASCIIFold(%q, %q) = %d, want %d", tt.s, tt.target, got, tt.want)
			}
		})
	}
}

// TestNormalizeFullwidthToASCII covers the fullwidth->ASCII normalization used
// to defeat scheme/keyword obfuscation. This guard previously had zero direct
// coverage. Ranges used: U+FF01..U+FF5E shift by -0xFEE0.
func TestNormalizeFullwidthToASCII(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ascii passthrough no fullwidth", "hello world", "hello world"},
		{"single fullwidth letter", "Ａ", "A"}, // Ａ -> A
		{"fullwidth digit", "１", "1"},         // １ -> 1
		{"fullwidth word", "Ｊａｖａ", "Java"},    // Ｊａｖａ -> Java
		{"fullwidth scheme bypass", "ｊａｖａｓｃｒｉｐｔ：", "javascript:"},
		{"mixed fullwidth and ascii", "ＡＢＣ" + "123", "ABC123"},
		{"range lower bound converts", "！", "!"},   // ！-> !
		{"range upper bound converts", "～", "~"},   // ～ -> ~
		{"just below range passthrough", "＀", "＀"}, // ￀ unchanged
		{"just above range passthrough", "｟", "｟"}, // ￟ unchanged
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeFullwidthToASCII(tt.in); got != tt.want {
				t.Errorf("normalizeFullwidthToASCII(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFindBodyElement covers the success path and both nil-return paths of
// findBodyElement (non-DocumentNode input, and a DocumentNode with no body).
func TestFindBodyElement(t *testing.T) {
	t.Parallel()

	t.Run("document with direct body child returns body node", func(t *testing.T) {
		t.Parallel()
		// findBodyElement only walks the DocumentNode's *direct* children, so the
		// success branch is only reachable when <body> is a top-level child. A full
		// document parsed via html.Parse nests <body> under <html> (covered by the
		// no-body case below), so construct this structure directly.
		doc := &html.Node{Type: html.DocumentNode}
		body := &html.Node{Type: html.ElementNode, Data: "body"}
		doc.AppendChild(body)
		got := findBodyElement(doc)
		if got == nil {
			t.Fatal("findBodyElement returned nil for a document with a direct <body> child")
		}
		if got != body {
			t.Error("findBodyElement returned a node other than the body child")
		}
		if got.Data != "body" {
			t.Errorf("returned node Data = %q, want %q", got.Data, "body")
		}
	})

	t.Run("non document node returns nil", func(t *testing.T) {
		t.Parallel()
		// An element node named "body" must still be rejected: the function
		// only walks a DocumentNode.
		elem := &html.Node{Type: html.ElementNode, Data: "body"}
		if got := findBodyElement(elem); got != nil {
			t.Errorf("findBodyElement on an ElementNode returned %v, want nil", got)
		}
	})

	t.Run("document without body returns nil", func(t *testing.T) {
		t.Parallel()
		// Build a DocumentNode whose only child is <html> with no <body>.
		doc := &html.Node{Type: html.DocumentNode}
		htmlNode := &html.Node{Type: html.ElementNode, Data: "html"}
		doc.AppendChild(htmlNode)
		if got := findBodyElement(doc); got != nil {
			t.Errorf("findBodyElement on a body-less document returned %v, want nil", got)
		}
	})
}
