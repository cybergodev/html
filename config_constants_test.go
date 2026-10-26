package html_test

// config_constants_test.go - D-005 API unification regression tests.
// Locks the exported Format* constants to the values accepted by
// Config.Validate and used by DefaultConfig()/presets, and covers
// AuditConfig.Validate() (extracted from Config.Validate without
// changing error identity).

import (
	"errors"
	"testing"

	"github.com/cybergodev/html"
)

// TestFormatConstantsAcceptedByValidate verifies every exported Format*
// constant passes validation on each field it is documented to accept.
func TestFormatConstantsAcceptedByValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*html.Config)
	}{
		{"InlineImageFormat none", func(c *html.Config) { c.InlineImageFormat = html.FormatNone }},
		{"InlineImageFormat markdown", func(c *html.Config) { c.InlineImageFormat = html.FormatMarkdown }},
		{"InlineImageFormat html", func(c *html.Config) { c.InlineImageFormat = html.FormatHTML }},
		{"InlineImageFormat placeholder", func(c *html.Config) { c.InlineImageFormat = html.FormatPlaceholder }},
		{"InlineLinkFormat none", func(c *html.Config) { c.InlineLinkFormat = html.FormatNone }},
		{"InlineLinkFormat markdown", func(c *html.Config) { c.InlineLinkFormat = html.FormatMarkdown }},
		{"InlineLinkFormat html", func(c *html.Config) { c.InlineLinkFormat = html.FormatHTML }},
		{"TableFormat markdown", func(c *html.Config) { c.TableFormat = html.FormatMarkdown }},
		{"TableFormat html", func(c *html.Config) { c.TableFormat = html.FormatHTML }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() rejected constant: %v", err)
			}
		})
	}
}

// TestFormatPlaceholderRejectedOutsideInlineImageFormat verifies
// FormatPlaceholder stays an InlineImageFormat-only value.
func TestFormatPlaceholderRejectedOutsideInlineImageFormat(t *testing.T) {
	t.Parallel()

	fields := []struct {
		name   string
		mutate func(*html.Config)
	}{
		{"InlineLinkFormat", func(c *html.Config) { c.InlineLinkFormat = html.FormatPlaceholder }},
		{"TableFormat", func(c *html.Config) { c.TableFormat = html.FormatPlaceholder }},
	}
	for _, tt := range fields {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() accepted FormatPlaceholder, want rejection")
			}
			if !errors.Is(err, html.ErrInvalidConfig) {
				t.Errorf("Validate() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

// TestFormatConstantsMatchDefaultAndPresets verifies the constants equal the
// values DefaultConfig() and MarkdownConfig() install, and that every preset
// still validates.
func TestFormatConstantsMatchDefaultAndPresets(t *testing.T) {
	t.Parallel()

	if got := html.DefaultConfig(); got.InlineImageFormat != html.FormatNone ||
		got.InlineLinkFormat != html.FormatNone || got.TableFormat != html.FormatMarkdown {
		t.Errorf("DefaultConfig() formats = (%q, %q, %q), want (%q, %q, %q)",
			got.InlineImageFormat, got.InlineLinkFormat, got.TableFormat,
			html.FormatNone, html.FormatNone, html.FormatMarkdown)
	}
	if got := html.MarkdownConfig(); got.InlineImageFormat != html.FormatMarkdown ||
		got.InlineLinkFormat != html.FormatMarkdown {
		t.Errorf("MarkdownConfig() formats = (%q, %q), want (%q, %q)",
			got.InlineImageFormat, got.InlineLinkFormat,
			html.FormatMarkdown, html.FormatMarkdown)
	}
	presets := []struct {
		name string
		cfg  html.Config
	}{
		{"DefaultConfig", html.DefaultConfig()},
		{"HighSecurityConfig", html.HighSecurityConfig()},
		{"TextOnlyConfig", html.TextOnlyConfig()},
		{"MarkdownConfig", html.MarkdownConfig()},
	}
	for _, p := range presets {
		if err := p.cfg.Validate(); err != nil {
			t.Errorf("%s.Validate() = %v, want nil", p.name, err)
		}
	}
}

// TestAuditConfigValidate covers the AuditConfig.Validate method extracted
// from Config.Validate: same ranges, same ErrInvalidConfig sentinel.
func TestAuditConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("zero value and default pass", func(t *testing.T) {
		t.Parallel()
		if err := (html.AuditConfig{}).Validate(); err != nil {
			t.Errorf("AuditConfig{}.Validate() = %v, want nil", err)
		}
		if err := html.DefaultAuditConfig().Validate(); err != nil {
			t.Errorf("DefaultAuditConfig().Validate() = %v, want nil", err)
		}
	})

	invalid := []struct {
		name   string
		mutate func(*html.AuditConfig)
	}{
		{"negative MaxRawValueLength", func(c *html.AuditConfig) { c.MaxRawValueLength = -1 }},
		{"negative MaxEntries", func(c *html.AuditConfig) { c.MaxEntries = -1 }},
		{"MaxEntries over limit", func(c *html.AuditConfig) { c.MaxEntries = 100001 }},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := html.DefaultAuditConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !errors.Is(err, html.ErrInvalidConfig) {
				t.Errorf("Validate() error = %v, want ErrInvalidConfig", err)
			}
		})
	}

	t.Run("reached through Config.Validate with same sentinel", func(t *testing.T) {
		t.Parallel()
		cfg := html.DefaultConfig()
		cfg.Audit.MaxEntries = -1
		if err := cfg.Validate(); !errors.Is(err, html.ErrInvalidConfig) {
			t.Errorf("Config.Validate() error = %v, want ErrInvalidConfig", err)
		}
	})
}
