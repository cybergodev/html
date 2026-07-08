package html

import (
	"context"
	"encoding/json"

	"github.com/cybergodev/html/internal"
)

// ExtractToMarkdown extracts content from HTML and returns it in Markdown format.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// This method configures the extractor to use markdown format for inline images and links.
// Thread-safe: creates a config copy to avoid modifying shared state.
func (p *Processor) ExtractToMarkdown(htmlBytes []byte) (string, error) {
	return recoverPanic(func() (string, error) {
		result, err := p.extractWithFormats(htmlBytes, "markdown", "markdown")
		if err != nil {
			return "", err
		}
		return result.Text, nil
	})
}

// ExtractToMarkdownFromFile extracts content from an HTML file and returns it in Markdown format.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// This method configures the extractor to use markdown format for inline images and links.
// Thread-safe: creates a config copy to avoid modifying shared state.
func (p *Processor) ExtractToMarkdownFromFile(filePath string) (string, error) {
	return recoverPanic(func() (string, error) {
		result, err := p.extractFromFileWithFormats(filePath, "markdown", "markdown")
		if err != nil {
			return "", err
		}
		return result.Text, nil
	})
}

// ExtractToJSON extracts content from HTML and returns it as JSON.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// This method uses the processor's configuration (cache, timeout, etc.) for extraction.
func (p *Processor) ExtractToJSON(htmlBytes []byte) ([]byte, error) {
	return recoverPanic(func() ([]byte, error) {
		result, err := p.Extract(htmlBytes)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
}

// ExtractToJSONFromFile extracts content from an HTML file and returns it as JSON.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// This method uses the processor's configuration (cache, timeout, etc.) for extraction.
func (p *Processor) ExtractToJSONFromFile(filePath string) ([]byte, error) {
	return recoverPanic(func() ([]byte, error) {
		result, err := p.ExtractFromFile(filePath)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
}

// ExtractToMarkdownWithContext extracts content from HTML and returns it in Markdown format with context support.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// Thread-safe: creates a config copy to avoid modifying shared state.
func (p *Processor) ExtractToMarkdownWithContext(ctx context.Context, htmlBytes []byte) (string, error) {
	return recoverPanic(func() (string, error) {
		result, err := p.extractWithFormatsWithContext(ctx, htmlBytes, "markdown", "markdown")
		if err != nil {
			return "", err
		}
		return result.Text, nil
	})
}

// ExtractToMarkdownFromFileWithContext extracts content from an HTML file and returns it in Markdown format with context support.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Thread-safe: creates a config copy to avoid modifying shared state.
func (p *Processor) ExtractToMarkdownFromFileWithContext(ctx context.Context, filePath string) (string, error) {
	return recoverPanic(func() (string, error) {
		result, err := p.extractFromFileWithFormatsWithContext(ctx, filePath, "markdown", "markdown")
		if err != nil {
			return "", err
		}
		return result.Text, nil
	})
}

// ExtractToJSONWithContext extracts content from HTML and returns it as JSON with context support.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
func (p *Processor) ExtractToJSONWithContext(ctx context.Context, htmlBytes []byte) ([]byte, error) {
	return recoverPanic(func() ([]byte, error) {
		result, err := p.ExtractWithContext(ctx, htmlBytes)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
}

// ExtractToJSONFromFileWithContext extracts content from an HTML file and returns it as JSON with context support.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
func (p *Processor) ExtractToJSONFromFileWithContext(ctx context.Context, filePath string) ([]byte, error) {
	return recoverPanic(func() ([]byte, error) {
		result, err := p.ExtractFromFileWithContext(ctx, filePath)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
}

// ============================================================================
// Package-level Convenience Functions
// ============================================================================

// ExtractToMarkdown extracts content from HTML and returns it in Markdown format.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToMarkdown(htmlBytes []byte, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractToMarkdown(htmlBytes)
	})
}

// ExtractToMarkdownFromFile extracts content from an HTML file and returns it in Markdown format.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToMarkdownFromFile(filePath string, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractToMarkdownFromFile(filePath)
	})
}

// ExtractToJSON extracts content from HTML and returns it as JSON.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToJSON(htmlBytes []byte, cfg ...Config) ([]byte, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) ([]byte, error) {
		return p.ExtractToJSON(htmlBytes)
	})
}

// ExtractToJSONFromFile extracts content from an HTML file and returns it as JSON.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToJSONFromFile(filePath string, cfg ...Config) ([]byte, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) ([]byte, error) {
		return p.ExtractToJSONFromFile(filePath)
	})
}

// ExtractToMarkdownWithContext extracts content from HTML and returns it in Markdown format with context support.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToMarkdownWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractToMarkdownWithContext(ctx, htmlBytes)
	})
}

// ExtractToMarkdownFromFileWithContext extracts content from an HTML file and returns it in Markdown format with context support.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToMarkdownFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractToMarkdownFromFileWithContext(ctx, filePath)
	})
}

// ExtractToJSONWithContext extracts content from HTML and returns it as JSON with context support.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToJSONWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) ([]byte, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) ([]byte, error) {
		return p.ExtractToJSONWithContext(ctx, htmlBytes)
	})
}

// ExtractToJSONFromFileWithContext extracts content from an HTML file and returns it as JSON with context support.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractToJSONFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) ([]byte, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) ([]byte, error) {
		return p.ExtractToJSONFromFileWithContext(ctx, filePath)
	})
}

// jsonResult wraps Result for custom JSON marshaling with duration formatting.
type jsonResult struct {
	Text             string      `json:"text"`
	Title            string      `json:"title"`
	Images           []ImageInfo `json:"images,omitempty"`
	Links            []LinkInfo  `json:"links,omitempty"`
	Videos           []VideoInfo `json:"videos,omitempty"`
	Audios           []AudioInfo `json:"audios,omitempty"`
	ProcessingTimeMS int64       `json:"processing_time_ms"`
	WordCount        int         `json:"word_count"`
	ReadingTimeMS    int64       `json:"reading_time_ms"`
}

// MarshalJSON implements custom JSON marshaling for Result.
// It converts time.Duration fields to milliseconds for better JSON interoperability.
// Note: Result does not implement UnmarshalJSON. Deserializing JSON output back into
// Result will lose duration fields (ProcessingTime, ReadingTime) because the JSON keys
// differ from the struct field names. This is intentional — the JSON format is designed
// for external consumption, not round-tripping.
func (r *Result) MarshalJSON() ([]byte, error) {
	jr := jsonResult{
		Text:             r.Text,
		Title:            r.Title,
		Images:           r.Images,
		Links:            r.Links,
		Videos:           r.Videos,
		Audios:           r.Audios,
		ProcessingTimeMS: r.ProcessingTime.Milliseconds(),
		WordCount:        r.WordCount,
		ReadingTimeMS:    r.ReadingTime.Milliseconds(),
	}
	return json.Marshal(jr)
}

// buildFormatProcessor returns a transient processor that reuses p's scorer but
// applies the given inline image/link formats. It snapshots p's config under the
// config lock so the format overrides never mutate the shared config, uses a
// disabled cache to avoid polluting p's cache with format-specific results, and
// an independent disabled audit collector so a parent Close() cannot race with
// an in-flight extraction on the temporary processor.
func (p *Processor) buildFormatProcessor(imageFormat, linkFormat string) *Processor {
	p.configMu.Lock()
	cfg := *p.config // Value copy to avoid race conditions
	p.configMu.Unlock()

	cfg.InlineImageFormat = imageFormat
	cfg.InlineLinkFormat = linkFormat
	// The transient processor uses a disabled cache (NewCache[[16]byte](0, 0) below), so
	// zero the entry budget too: this short-circuits the cache-key generation
	// and Get/Set calls in Extract, which would otherwise run as no-ops while
	// still paying the cost of hashing the input on every format conversion.
	cfg.MaxCacheEntries = 0

	return &Processor{
		config:       &cfg,
		cache:        internal.NewCache[[16]byte](0, 0),
		scorer:       p.scorer,
		audit:        newAuditCollector(AuditConfig{Enabled: false}),
		auditAdapter: &auditRecorderAdapter{collector: nil},
		stats:        &processorStats{},
		imageFormat:  normalizeInlineFormat(imageFormat),
		linkFormat:   normalizeInlineFormat(linkFormat),
	}
}

// extractWithFormats extracts content using temporary format settings.
// This creates a config copy to avoid race conditions when modifying format settings.
func (p *Processor) extractWithFormats(htmlBytes []byte, imageFormat, linkFormat string) (*Result, error) {
	if p == nil || p.closed.Load() {
		return nil, ErrProcessorClosed
	}
	return p.buildFormatProcessor(imageFormat, linkFormat).Extract(htmlBytes)
}

// extractFromFileWithFormats extracts content from file using temporary format settings.
func (p *Processor) extractFromFileWithFormats(filePath, imageFormat, linkFormat string) (*Result, error) {
	if p == nil || p.closed.Load() {
		return nil, ErrProcessorClosed
	}
	data, err := p.validateAndReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return p.extractWithFormats(data, imageFormat, linkFormat)
}

// extractWithFormatsWithContext extracts content using temporary format settings with context support.
func (p *Processor) extractWithFormatsWithContext(ctx context.Context, htmlBytes []byte, imageFormat, linkFormat string) (*Result, error) {
	if p == nil || p.closed.Load() {
		return nil, ErrProcessorClosed
	}
	return p.buildFormatProcessor(imageFormat, linkFormat).ExtractWithContext(ctx, htmlBytes)
}

// extractFromFileWithFormatsWithContext extracts content from file using temporary format settings with context support.
func (p *Processor) extractFromFileWithFormatsWithContext(ctx context.Context, filePath, imageFormat, linkFormat string) (*Result, error) {
	if p == nil || p.closed.Load() {
		return nil, ErrProcessorClosed
	}
	data, err := p.validateAndReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return p.extractWithFormatsWithContext(ctx, data, imageFormat, linkFormat)
}
