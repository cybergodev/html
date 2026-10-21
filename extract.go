package html

import (
	"context"
	"errors"
	"fmt"
	htmlstd "html"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/cybergodev/html/internal"
	"github.com/cybergodev/html/internal/table"
	stdxhtml "golang.org/x/net/html"
)

// markdownEscapeReplacer escapes characters that could break Markdown link/image syntax.
var markdownEscapeReplacer = strings.NewReplacer(
	`\`, `\\`,
	`[`, `\[`,
	`]`, `\]`,
)

// containsASCIIFold reports whether substr is contained in s, case-insensitively
// (ASCII only). Both s and substr are folded to lowercase before comparison.
func containsASCIIFold(s, substr string) bool {
	substrLen := len(substr)
	sLen := len(s)
	if substrLen > sLen {
		return false
	}
	for i := 0; i <= sLen-substrLen; i++ {
		match := true
		for j := 0; j < substrLen; j++ {
			c := s[i+j]
			sc := substr[j]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if c != sc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// escapeMarkdownText escapes characters that could break Markdown link/image syntax.
// Escapes ], [, and \ to prevent injection of arbitrary Markdown content.
func escapeMarkdownText(s string) string {
	// Fast path: strings.(*Replacer).Replace allocates a new string even when
	// nothing matches, so scan first and skip the replacer entirely when the
	// text contains none of the three escapable characters. Link and image
	// text almost never contains them, so this avoids an allocation per call.
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '[', ']':
			return markdownEscapeReplacer.Replace(s)
		}
	}
	return s
}

// markdownURLDestReplacer percent-encodes the bytes that would break a
// Markdown link/image destination.
var markdownURLDestReplacer = strings.NewReplacer(
	"(", "%28",
	")", "%29",
	" ", "%20",
)

// escapeMarkdownURLDest percent-encodes the parentheses and spaces in a
// Markdown link/image destination. An unescaped ')' terminates the
// destination early and spills the rest of the URL into rendered text, and
// per CommonMark unescaped whitespace also terminates it — so a destination
// like "my file.png" rendered as broken text rather than a link.
// Percent-encoding — rather than backslash-escaping — keeps the destination
// a valid URL that survives copy-paste. Fast path: most destinations contain
// none of these bytes.
func escapeMarkdownURLDest(url string) string {
	if !strings.ContainsAny(url, "() ") {
		return url
	}
	return markdownURLDestReplacer.Replace(url)
}

// Timeout goroutine management constants.
// These prevent goroutine leaks when many timeout operations occur.
const (
	// maxTimeoutGoroutines limits concurrent timeout goroutines to prevent
	// resource exhaustion. Goroutine stacks start at a few KB and grow on
	// demand, so 1000 concurrent watchdog goroutines is a modest, bounded cost;
	// the cap exists to keep a burst of timed operations from spawning them
	// without limit. When exceeded, new operations return an error wrapping
	// ErrProcessingTimeout immediately rather than queuing another goroutine.
	maxTimeoutGoroutines = 1000
)

// activeTimeoutGoroutines tracks the number of currently running timeout goroutines.
var activeTimeoutGoroutines atomic.Int64

// recoverPanic wraps a function with defense-in-depth panic recovery.
func recoverPanic[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrInternalPanic, r)
		}
	}()
	return fn()
}

// Extract extracts content from HTML bytes with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// For better performance with multiple extractions, create a Processor directly:
//
//	processor, _ := html.New()
//	defer processor.Close()
//	for _, html := range htmlDocs {
//	    result, _ := processor.Extract(html)  // Uses cache
//	}
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Examples:
//
//	// With default config
//	result, err := html.Extract(htmlBytes)
//
//	// With custom config
//	cfg := html.DefaultConfig()
//	cfg.PreserveImages = false
//	result, err := html.Extract(htmlBytes, cfg)
//
// Returns the same errors as [Processor.Extract], plus ErrMultipleConfigs if
// more than one Config is provided and ErrInvalidConfig (wrapped in *ConfigError)
// if the supplied configuration is invalid.
func Extract(htmlBytes []byte, cfg ...Config) (*Result, error) {
	return withConfig(cfg, func(p *Processor) (*Result, error) {
		return p.Extract(htmlBytes)
	})
}

// ExtractFromFile extracts content from an HTML file with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// In addition to the errors returned by [Extract], file access may fail with a
// *FileError wrapping ErrFileNotFound, ErrInvalidFilePath, or a path-traversal
// rejection (see Config.AllowedBaseDir).
func ExtractFromFile(filePath string, cfg ...Config) (*Result, error) {
	return withConfig(cfg, func(p *Processor) (*Result, error) {
		return p.ExtractFromFile(filePath)
	})
}

// ExtractText extracts plain text from HTML bytes with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// For better performance with multiple extractions, create a Processor directly.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [Extract].
func ExtractText(htmlBytes []byte, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ExtractText(htmlBytes)
	})
}

// ExtractTextFromFile extracts plain text from an HTML file with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractFromFile].
func ExtractTextFromFile(filePath string, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ExtractTextFromFile(filePath)
	})
}

// ExtractTextWithContext extracts plain text from HTML bytes with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractWithContext].
func ExtractTextWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ExtractTextWithContext(ctx, htmlBytes)
	})
}

// ExtractTextFromFileWithContext extracts plain text from an HTML file with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractFromFileWithContext].
func ExtractTextFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (string, error) {
	return withConfig(cfg, func(p *Processor) (string, error) {
		return p.ExtractTextFromFileWithContext(ctx, filePath)
	})
}

// ExtractWithContext extracts content from HTML bytes with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [Extract], plus context.Canceled or
// context.DeadlineExceeded when ctx is cancelled.
func ExtractWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (*Result, error) {
	return withConfig(cfg, func(p *Processor) (*Result, error) {
		return p.ExtractWithContext(ctx, htmlBytes)
	})
}

// ExtractFromFileWithContext extracts content from an HTML file with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractFromFile], plus context.Canceled or
// context.DeadlineExceeded when ctx is cancelled.
func ExtractFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (*Result, error) {
	return withConfig(cfg, func(p *Processor) (*Result, error) {
		return p.ExtractFromFileWithContext(ctx, filePath)
	})
}

// Extract extracts content from HTML bytes with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// This is the primary method for HTML content extraction when the source encoding
// may not be UTF-8, such as content from HTTP responses, databases, or files.
//
// Returns:
//   - ErrProcessorClosed if the processor is nil or has been closed
//   - ErrInputTooLarge (wrapped in *InputError) if len(htmlBytes) exceeds MaxInputSize
//   - a wrapped error if character-encoding detection or conversion fails
//   - ErrInvalidHTML if the bytes cannot be parsed as HTML
//   - ErrMaxDepthExceeded if element nesting exceeds MaxDepth
//   - ErrProcessingTimeout if processing exceeds ProcessingTimeout
//   - ErrInternalPanic if an unexpected internal panic is recovered
func (p *Processor) Extract(htmlBytes []byte) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		return p.extractCoreWithContext(context.Background(), htmlBytes)
	})
}

// ExtractWithContext extracts content from HTML bytes with context support for cancellation.
// This method provides cooperative cancellation, allowing long-running extractions to be
// interrupted when the context is cancelled.
//
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	result, err := processor.ExtractWithContext(ctx, htmlBytes)
//	if errors.Is(err, context.Canceled) {
//	    // Extraction was cancelled
//	}
//
// In addition to the errors returned by [Processor.Extract], this method returns
// context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ExtractWithContext(ctx context.Context, htmlBytes []byte) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		return p.extractCoreWithContext(ctx, htmlBytes)
	})
}

func (p *Processor) extractCoreWithContext(ctx context.Context, htmlBytes []byte) (*Result, error) {
	// Early cancellation check
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate processor state and input size
	if err := p.validateInput(htmlBytes, "Extract"); err != nil {
		return nil, err
	}

	startTime := time.Now()

	// Check the cache before encoding detection. The cache key is derived from
	// the raw input bytes (not the decoded string), so a cache hit skips the
	// O(n) encoding scan entirely — the single most expensive step on the hot
	// (repeated-extraction) path. This is safe because the same raw bytes + the
	// same config (including Encoding setting, which is mixed into the key)
	// deterministically produce the same extraction result.
	//
	// Note: Cache.Get/Set treat the zero key ([16]byte{}) as "no key" and reject
	// it, so in the astronomically unlikely event (2^-128) that this hash comes
	// out all zero, that input is simply a permanent cache miss — reprocessed on
	// every call. This is an accepted tradeoff.
	var cacheKey [16]byte
	hasCacheKey := false
	if p.config.MaxCacheEntries > 0 {
		cacheKey = p.generateCacheKey(htmlBytes)
		hasCacheKey = true
		if cached := p.cache.Get(cacheKey); cached != nil {
			// Count a hit only when the cached value is actually usable.
			if cachedResult, ok := cached.(*Result); ok {
				p.stats.cacheHits.Add(1)
				p.stats.totalProcessed.Add(1)
				// A cache hit is an extraction too: record its (near-zero)
				// elapsed time so AverageProcessTime is a true mean over all
				// extractions rather than a hybrid whose numerator excluded
				// hits while its divisor counted them, trending toward zero
				// as the hit ratio rose.
				p.stats.totalProcessTime.Add(int64(time.Since(startTime)))
				return cloneResult(cachedResult), nil
			}
		}
		p.stats.cacheMisses.Add(1)
	}

	// Check cancellation before encoding detection
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Detect encoding and convert to UTF-8 (skipped entirely on cache hits above)
	utf8String, err := p.detectEncoding(htmlBytes)
	if err != nil {
		return nil, err
	}

	// Check cancellation before content processing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Process content with optional timeout and context support. The timeout is
	// applied by deriving a deadline from ctx and threading it through
	// withTimeout into processContentWithContext, whose cooperative cancellation
	// checks then honor the deadline — so an expired timeout interrupts in-flight
	// work at the next check rather than merely racing the return value while
	// extraction runs to completion.
	var result *Result
	result, err = runWithTimeout(p, ctx, func(deadlineCtx context.Context) (*Result, error) {
		return p.processContentWithContext(deadlineCtx, utf8String)
	})

	if err != nil {
		p.stats.errorCount.Add(1)
		// Depth violations are recorded at the violation site in
		// validateDepthTraversal, which knows the actual offending depth.
		if p.audit != nil && errors.Is(err, ErrProcessingTimeout) {
			p.audit.RecordTimeout(p.config.ProcessingTimeout)
		}
		return nil, err
	}

	processingTime := time.Since(startTime)
	result.ProcessingTime = processingTime
	p.stats.totalProcessTime.Add(int64(processingTime))
	p.stats.totalProcessed.Add(1)

	// Only cache if caching is enabled and we generated a key for this input.
	if hasCacheKey {
		p.cache.Set(cacheKey, result)
		// Return a clone so the caller owns an independent copy. The cached
		// entry and the returned value must not alias: a caller that mutates
		// its result would otherwise race with concurrent cache-hit reads
		// (cloneResult) and silently corrupt the cache. This mirrors the clone
		// taken on the cache-hit path, closing the same ownership contract.
		return cloneResult(result), nil
	}

	return result, nil
}

// processContentWithContext processes HTML content with context cancellation support.
// This method implements cooperative cancellation at key processing stages.
func (p *Processor) processContentWithContext(ctx context.Context, htmlContent string) (*Result, error) {
	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if p.isBlankContent(htmlContent) {
		return &Result{}, nil
	}

	originalHTML := htmlContent

	// Check context before HTML parsing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	doc, err := stdxhtml.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidHTML, err)
	}

	// Check context before depth validation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate DOM depth BEFORE any recursive traversal. The extraction passes
	// below (article-node folding, text extraction, media walks) recurse without
	// an internal depth cap, so a pathologically deep document could exhaust the
	// call stack before those passes rejected it; SanitizeDOM itself is iterative
	// but runs here too, on the same depth-validated tree. Running the O(nodes)
	// iterative depth check first bounds every later recursive pass to MaxDepth.
	// Depth is measured on the raw parsed tree; sanitization only removes nodes,
	// so this is at most marginally stricter than the previous
	// sanitize-then-validate order.
	if err := p.validateDepthTraversal(doc, 0); err != nil {
		return nil, err
	}

	// Sanitize DOM in-place (avoids render + re-parse overhead). Runs only on a
	// depth-validated tree, so its recursion is bounded by MaxDepth.
	if p.config.EnableSanitization {
		if p.audit != nil && p.config.Audit.Enabled {
			internal.SanitizeDOM(doc, p.audit)
		} else {
			internal.SanitizeDOM(doc, internal.NoOpAuditRecorder{})
		}
	}

	// Check context before document extraction
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return p.extractFromDocument(doc, originalHTML)
}

// ExtractFromFile extracts content from an HTML file with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
//
// Returns the same errors as [Processor.Extract], plus a *FileError wrapping
// ErrFileNotFound, ErrInvalidFilePath, or a path-traversal rejection for
// file-access failures.
func (p *Processor) ExtractFromFile(filePath string) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		// Validate processor state (no input size check for file paths)
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}

		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return nil, err
		}

		return p.Extract(data)
	})
}

// ExtractFromFileWithContext extracts content from an HTML file with context support.
// This method provides cooperative cancellation, allowing long-running extractions to be
// interrupted when the context is cancelled.
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	result, err := processor.ExtractFromFileWithContext(ctx, "page.html")
//
// In addition to the errors returned by [Processor.ExtractFromFile], this method
// returns context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ExtractFromFileWithContext(ctx context.Context, filePath string) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		// Early cancellation check
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Validate processor state (no input size check for file paths)
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}

		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return nil, err
		}

		return p.ExtractWithContext(ctx, data)
	})
}

// ExtractText extracts plain text from HTML bytes with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.Extract].
func (p *Processor) ExtractText(htmlBytes []byte) (string, error) {
	result, err := p.Extract(htmlBytes)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ExtractTextFromFile extracts plain text from an HTML file with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.ExtractFromFile].
func (p *Processor) ExtractTextFromFile(filePath string) (string, error) {
	result, err := p.ExtractFromFile(filePath)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ExtractTextWithContext extracts plain text from HTML bytes with context support for cancellation.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.ExtractWithContext].
func (p *Processor) ExtractTextWithContext(ctx context.Context, htmlBytes []byte) (string, error) {
	result, err := p.ExtractWithContext(ctx, htmlBytes)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ExtractTextFromFileWithContext extracts plain text from an HTML file with context support for cancellation.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.ExtractFromFileWithContext].
func (p *Processor) ExtractTextFromFileWithContext(ctx context.Context, filePath string) (string, error) {
	result, err := p.ExtractFromFileWithContext(ctx, filePath)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// withTimeout executes a function under a deadline derived from parent, returning
// its result or an error if the timeout expires. This is a generic helper that
// eliminates code duplication in timeout handling.
//
// The deadline context is passed into fn so that fn's own cooperative
// cancellation checks (e.g. the select-on-ctx.Done() stages in
// processContentWithContext) honor the timeout: when the deadline fires, fn
// returns at its next check rather than running to completion in the background.
// The caller still receives the timeout error promptly via the select below,
// independent of how long fn takes to wind down.
//
// On deadline, withTimeout returns the underlying context error
// (context.DeadlineExceeded); callers normalize it to ErrProcessingTimeout. A
// cancellation propagated through parent surfaces as context.Canceled.
//
// Goroutine Safety: To prevent goroutine leaks under heavy load, this function
// limits the maximum number of concurrent timeout goroutines. If the limit is
// exceeded, an error is returned.
func withTimeout[T any](parent context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	// Atomically check-and-add to prevent TOCTOU race under heavy concurrency.
	// This prevents resource exhaustion when processing many documents with timeouts.
	for {
		current := activeTimeoutGoroutines.Load()
		if current >= maxTimeoutGoroutines {
			var zero T
			return zero, fmt.Errorf("%w: %d watchdogs active", ErrTooManyConcurrentOps, current)
		}
		if activeTimeoutGoroutines.CompareAndSwap(current, current+1) {
			break
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	type result struct {
		res T
		err error
	}

	resultChan := make(chan result, 1)

	// Launch the operation in a goroutine
	// (counter already incremented by CAS loop above)
	go func() {
		// Ensure we decrement the counter when done
		defer activeTimeoutGoroutines.Add(-1)

		// Recover from panics inside fn() to prevent crashing the entire process.
		// Without this, a panic in the spawned goroutine bypasses the caller's
		// recover() (which only catches panics in the same goroutine).
		res, err := func() (res T, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%w: %v", ErrInternalPanic, r)
				}
			}()
			return fn(ctx)
		}()
		// Deliver the result unconditionally: resultChan is buffered with
		// capacity 1 and this goroutine is its only sender, so the send never
		// blocks — and racing it against ctx.Done() could discard a fully
		// computed result that the caller's prefer-result select below was
		// about to accept.
		resultChan <- result{res: res, err: err}
	}()

	// Prefer a completed result when both cases are ready: fn finishing in
	// the same instant the deadline fires made the blocking select below pick
	// either case pseudo-randomly, so a successfully computed extraction
	// could be reported as a timeout, run to run. Try the non-blocking
	// receive first.
	select {
	case res := <-resultChan:
		return res.res, res.err
	default:
	}
	select {
	case res := <-resultChan:
		return res.res, res.err
	case <-ctx.Done():
		var zero T
		// Return the context error so callers can distinguish a deadline from
		// a user cancellation. runWithTimeout normalizes it to
		// ErrProcessingTimeout only when the parent context is still live
		// (the processing timeout fired); a user deadline or cancellation
		// passes through unchanged. The goroutine above receives the same ctx
		// and winds down at its next cooperative check; the buffered
		// resultChan keeps it from blocking.
		return zero, ctx.Err()
	}
}

// runWithTimeout executes fn under the processor's configured ProcessingTimeout
// (if set), deriving the deadline from ctx so cooperative cancellation checks
// inside fn honor both the user's context and the timeout. The derived context
// expires with context.DeadlineExceeded both when the processing timeout
// elapses and when the caller's own (earlier) deadline does; only the former is
// normalized to ErrProcessingTimeout — a user deadline or cancellation is
// returned unchanged, matching the documented contract of the *WithContext
// entry points and keeping the audit log from misattributing a user
// cancellation to the library's timeout. It is a free function (not a method)
// because Go does not permit type parameters on methods.
func runWithTimeout[T any](p *Processor, ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	if p.config.ProcessingTimeout > 0 {
		result, err := withTimeout(ctx, p.config.ProcessingTimeout, fn)
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			// The parent context is still live, so the deadline that fired
			// is the processing timeout itself.
			err = ErrProcessingTimeout
		}
		return result, err
	}
	return fn(ctx)
}

// isBlankContent checks if content is empty or whitespace-only without allocating.
func (p *Processor) isBlankContent(content string) bool {
	n := len(content)
	if n == 0 {
		return true
	}
	for i := range n {
		c := content[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}

// depthStackEntry pairs a node with its depth for iterative depth validation.
type depthStackEntry struct {
	node  *stdxhtml.Node
	depth int
}

// depthStackPool reuses the traversal stack across Extract calls so each
// extraction does not allocate (and repeatedly grow) a fresh slice. The
// profiler attributed ~3.9% of allocations to this stack's growth.
var depthStackPool = sync.Pool{
	New: func() any {
		s := make([]depthStackEntry, 0, 64)
		return &s
	},
}

// validateDepthTraversal validates DOM tree depth using an iterative approach
// to avoid potential stack overflow on deeply nested documents.
// It folds depth validation into a single traversal rather than scanning the
// tree twice (once for depth, once for extraction).
func (p *Processor) validateDepthTraversal(root *stdxhtml.Node, initialDepth int) error {
	// Use iterative approach with explicit stack to avoid stack overflow
	// on deeply nested documents (MaxDepth can be up to 500). The stack is
	// borrowed from a pool and returned when traversal completes, so a
	// typical Extract allocates nothing here.
	stackPtr := depthStackPool.Get().(*[]depthStackEntry)
	stack := (*stackPtr)[:0]
	defer func() {
		*stackPtr = stack
		depthStackPool.Put(stackPtr)
	}()

	stack = append(stack, depthStackEntry{root, initialDepth})

	for len(stack) > 0 {
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if entry.depth > p.config.MaxDepth {
			// Record the actual offending depth (not a MaxDepth+1 stand-in)
			// so audit consumers can gauge how hostile the document was.
			if p.audit != nil {
				p.audit.RecordDepthViolation(entry.depth, p.config.MaxDepth)
			}
			return ErrMaxDepthExceeded
		}

		// Add children to stack in reverse order for correct traversal order
		for c := entry.node.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, depthStackEntry{c, entry.depth + 1})
		}
	}
	return nil
}

func (p *Processor) extractFromDocument(doc *stdxhtml.Node, htmlContent string) (*Result, error) {
	result := &Result{}
	result.Title = p.extractTitle(doc)

	contentNode := doc
	if p.config.ExtractArticle {
		if article := p.extractArticleNode(doc); article != nil {
			contentNode = article
		}
	}
	contentNode = internal.CleanContentNode(contentNode)

	imageFormat := p.imageFormat
	linkFormat := p.linkFormat

	// A "none" format has no formatter, so it needs neither the placeholder
	// tokens nor the resource list behind them.
	needsImageTokens := imageFormat != "none"
	needsLinkTokens := linkFormat != "none"

	// Collect images and links once, in a single traversal. A list is needed
	// when the caller asked for it (PreserveImages/PreserveLinks) or when a
	// formatter must substitute placeholders with it; requesting a list solely
	// because a formatter consumes it would otherwise drop the entries the
	// caller asked for when that format is "none".
	images, links := p.extractImagesAndLinks(contentNode,
		p.config.PreserveImages || needsImageTokens,
		p.config.PreserveLinks || needsLinkTokens)
	if p.config.PreserveImages {
		result.Images = images
	}
	if p.config.PreserveLinks {
		result.Links = links
	}

	if needsImageTokens || needsLinkTokens {
		tb := internal.GetTrackedBuilder()
		var imageCounter, linkCounter int
		var imgCounter, linkCounterPtr *int
		// Only emit the placeholder kinds a formatter will consume: a "none"
		// format has no formatter, so its tokens would survive both formatting
		// passes verbatim and leak into Text as literal [IMAGE:n]/[LINK:n] text
		// when just one of the two formats is enabled. ("placeholder" keeps
		// [IMAGE:n] visible by design and still needs the numbering.) Positions
		// in result.Images/result.Links come from the independent counters in
		// extractImagesAndLinks and are unaffected by this.
		if needsImageTokens {
			imgCounter = &imageCounter
		}
		if needsLinkTokens {
			linkCounterPtr = &linkCounter
		}
		internal.ExtractTextWithStructureAndImages(contentNode, tb, imgCounter, linkCounterPtr, p.config.TableFormat)
		// CleanTextBytes reads the builder's buffer directly, skipping the
		// tb.String() intermediate copy the string entry point would make.
		text := internal.CleanTextBytes(tb.Bytes())
		internal.PutTrackedBuilder(tb)

		// Apply formatters in order: images first, then links. A "none"
		// formatter returns its input unchanged.
		result.Text = p.formatInlineLinks(p.formatInlineImages(text, images, imageFormat), links, linkFormat)
	} else {
		result.Text = p.extractTextContent(contentNode, p.config.TableFormat)
	}

	result.WordCount = p.countWords(result.Text)
	result.ReadingTime = p.calculateReadingTime(result.WordCount)

	// HasMediaReference is a necessary condition for the raw-HTML media scans,
	// and it is the same content-only test for both media kinds, so it is
	// computed once here rather than per extractor.
	if p.config.PreserveVideos || p.config.PreserveAudios {
		canContainMedia := len(htmlContent) > 0 &&
			len(htmlContent) <= maxHTMLForRegex &&
			internal.HasMediaReference(htmlContent)
		// One call handles all three preserve combinations; the want flags keep
		// a disabled kind from costing a walk branch or a raw-HTML scan.
		videos, audios := p.extractMedia(doc, htmlContent, canContainMedia,
			p.config.PreserveVideos, p.config.PreserveAudios)
		result.Videos = videos
		result.Audios = audios
	}
	return result, nil
}

func (p *Processor) extractTitle(doc *stdxhtml.Node) string {
	if doc == nil {
		return ""
	}
	// Collect the first <title>, <h1>, and <h2> in a single document-order
	// traversal. The previous implementation issued up to three separate
	// FindElementByTag calls (one full tree walk each); documents lacking a
	// <title> or <h1> — common — paid for all three. Priority is unchanged:
	// title → h1 → h2, each preferring a non-empty value. The nil guards keep
	// the FIRST occurrence of each tag: without them the walk (which runs
	// until all three candidates are found) kept overwriting the candidates,
	// so the fallback title became the LAST h1/h2 before the stopping point.
	var titleNode, h1Node, h2Node *stdxhtml.Node
	internal.WalkNodes(doc, func(n *stdxhtml.Node) bool {
		if n.Type == stdxhtml.ElementNode {
			switch n.Data {
			case "title":
				if titleNode == nil {
					titleNode = n
				}
			case "h1":
				if h1Node == nil {
					h1Node = n
				}
			case "h2":
				if h2Node == nil {
					h2Node = n
				}
			}
		}
		// Keep walking until all three candidates have been located.
		return titleNode == nil || h1Node == nil || h2Node == nil
	})
	if titleNode != nil {
		if title := internal.GetTextContent(titleNode); title != "" {
			return title
		}
	}
	if h1Node != nil {
		if title := internal.GetTextContent(h1Node); title != "" {
			return title
		}
	}
	if h2Node != nil {
		return internal.GetTextContent(h2Node)
	}
	return ""
}

func (p *Processor) extractArticleNode(doc *stdxhtml.Node) *stdxhtml.Node {
	if doc == nil {
		return nil
	}

	// Fast path: the built-in DefaultScorer can find the best article node from a
	// single bottom-up metrics pass (FindBestArticleNode), avoiding both the O(N²)
	// per-candidate subtree re-walk that Score+collectContentMetrics would repeat
	// for every non-inline element, and the map allocation that
	// ScoreArticleCandidates would require. Custom scorers lack that method and
	// fall back to the per-node Score() loop, whose behavior is unchanged.
	if ds, ok := p.scorer.(*internal.DefaultScorer); ok {
		if bestNode := ds.FindBestArticleNode(doc); bestNode != nil {
			return bestNode
		}
	} else {
		candidates := make(map[*stdxhtml.Node]int, initialMapCap)
		internal.WalkNodes(doc, func(n *stdxhtml.Node) bool {
			if n.Type == stdxhtml.ElementNode && !internal.IsInlineElement(n.Data) {
				if score := p.scorer.Score(n); score > 0 {
					candidates[n] = score
				}
			}
			return true
		})
		if bestNode := internal.SelectBestCandidate(candidates); bestNode != nil {
			return bestNode
		}
	}
	return internal.FindElementByTag(doc, "body")
}

func (p *Processor) extractTextContent(node *stdxhtml.Node, tableFormat string) string {
	// A pooled TrackedBuilder retains its capacity across calls (unlike a pooled
	// *strings.Builder, whose Reset nils the buffer), so the document-length text
	// is appended into a buffer that is already large enough after warmup — no
	// per-call growth allocations or the GC churn they cause.
	tb := internal.GetTrackedBuilder()
	internal.ExtractTextWithStructureAndImages(node, tb, nil, nil, tableFormat)
	// CleanTextBytes reads the builder's buffer directly, skipping the
	// tb.String() intermediate copy the string entry point would make (the
	// returned string is a fresh allocation and never aliases the pooled buffer).
	result := internal.CleanTextBytes(tb.Bytes())
	internal.PutTrackedBuilder(tb)
	return result
}

// writeMarkdownInline writes a Markdown inline link — or, when image is true,
// the image form of the same construct (a leading '!') — escaping the label
// and destination exactly as both inline formatters require. It is the single
// place those rules live: images and links previously assembled the identical
// construct in two different ways, so an escaping fix could land in one and
// miss the other.
func writeMarkdownInline(tb *table.TrackedBuilder, image bool, label, url string) {
	if image {
		_ = tb.WriteByte('!')
	}
	_ = tb.WriteByte('[')
	_, _ = tb.WriteString(escapeMarkdownText(label))
	_, _ = tb.WriteString("](")
	_, _ = tb.WriteString(escapeMarkdownURLDest(url))
	_ = tb.WriteByte(')')
}

// writeHTMLAttr writes a quoted HTML attribute, escaping the value so it
// cannot break out of the attribute. Both inline HTML formatters emit
// attributes this way.
func writeHTMLAttr(tb *table.TrackedBuilder, name, value string) {
	_ = tb.WriteByte(' ')
	_, _ = tb.WriteString(name)
	_, _ = tb.WriteString(`="`)
	_, _ = tb.WriteString(htmlstd.EscapeString(value))
	_ = tb.WriteByte('"')
}

// writeHTMLAnchor writes an inline HTML anchor around text.
func writeHTMLAnchor(tb *table.TrackedBuilder, href, title, text string) {
	_, _ = tb.WriteString("<a")
	writeHTMLAttr(tb, "href", href)
	if title != "" {
		writeHTMLAttr(tb, "title", title)
	}
	_, _ = tb.WriteString(">")
	_, _ = tb.WriteString(htmlstd.EscapeString(text))
	_, _ = tb.WriteString("</a>")
}

func (p *Processor) formatInlineImages(textWithPlaceholders string, images []ImageInfo, format string) string {
	if len(images) == 0 || format == "placeholder" || format == "none" {
		return textWithPlaceholders
	}

	switch format {
	case "markdown":
		// Build replacements and use direct string replacement. The pooled
		// builder holds one rendered image at a time and String() copies it out,
		// so the replacement list owns its strings.
		md := internal.GetTrackedBuilder()
		defer internal.PutTrackedBuilder(md)
		replacements := make([]string, 0, len(images)*2)
		for i := range images {
			if images[i].Position == 0 {
				continue
			}
			altText := images[i].Alt
			if altText == "" {
				altText = "Image " + strconv.Itoa(images[i].Position)
			}
			md.Reset()
			writeMarkdownInline(md, true, altText, images[i].URL)
			replacements = append(replacements,
				"[IMAGE:"+strconv.Itoa(images[i].Position)+"]", md.String())
		}
		if len(replacements) > 0 {
			replacer := strings.NewReplacer(replacements...)
			return replacer.Replace(textWithPlaceholders)
		}

	case "html":
		// Build replacements with a pooled TrackedBuilder: it retains its
		// backing array across the per-image Reset, unlike the *strings.Builder
		// previously pooled here whose Reset() nils the buffer (see pool.go).
		htmlImg := internal.GetTrackedBuilder()
		defer internal.PutTrackedBuilder(htmlImg)
		replacements := make([]string, 0, len(images)*2)
		for i := range images {
			if images[i].Position == 0 {
				continue
			}
			htmlImg.Reset()
			htmlImg.Grow(len(images[i].URL) + len(images[i].Alt) + len(images[i].Width) + len(images[i].Height) + imageHTMLBufExtra)
			_, _ = htmlImg.WriteString("<img")
			writeHTMLAttr(htmlImg, "src", images[i].URL)
			writeHTMLAttr(htmlImg, "alt", images[i].Alt)
			// The advisory title is a native HTML attribute; ImageInfo.Title was
			// collected but never rendered in either inline format.
			if images[i].Title != "" {
				writeHTMLAttr(htmlImg, "title", images[i].Title)
			}
			if images[i].Width != "" {
				writeHTMLAttr(htmlImg, "width", images[i].Width)
			}
			if images[i].Height != "" {
				writeHTMLAttr(htmlImg, "height", images[i].Height)
			}
			_, _ = htmlImg.WriteString(">")
			replacements = append(replacements,
				"[IMAGE:"+strconv.Itoa(images[i].Position)+"]", htmlImg.String())
		}
		if len(replacements) > 0 {
			replacer := strings.NewReplacer(replacements...)
			return replacer.Replace(textWithPlaceholders)
		}
	}

	return textWithPlaceholders
}

func (p *Processor) formatInlineLinks(textWithPlaceholders string, links []LinkInfo, format string) string {
	if len(links) == 0 || format == "none" {
		return textWithPlaceholders
	}

	// Fast check: any [LINK: present?
	if !strings.Contains(textWithPlaceholders, "[LINK:") {
		return textWithPlaceholders
	}

	// Build lookup map
	linkMap := make(map[int]LinkInfo, len(links))
	for _, link := range links {
		if link.Position > 0 {
			linkMap[link.Position] = link
		}
	}

	// Direct string scanning to avoid regex overhead. The TrackedBuilder
	// retains its backing array across pool round-trips, unlike the
	// *strings.Builder previously pooled here whose Reset() nils the buffer
	// (see pool.go).
	tb := internal.GetTrackedBuilder()
	defer internal.PutTrackedBuilder(tb)
	tb.Grow(len(textWithPlaceholders))

	i := 0
	n := len(textWithPlaceholders)
	// noEndTagFrom is the smallest index from which a scan for the closing
	// [/LINK] token is known to have failed: no closing token exists at or
	// after it. Closing-tag scans only ever start further right, so one
	// failed scan settles every later one. Without this floor, text holding
	// many unmatched "[LINK:n]" tokens rescanned the whole tail once per
	// token, making replacement quadratic (measured 128ms at 8k tokens on a
	// 64KB document, ~31s of CPU at 1MB) while pinning the withTimeout
	// goroutine slot for the duration.
	noEndTagFrom := n + 1
	for i < n {
		// Look for [LINK:
		if textWithPlaceholders[i] == '[' && i+6 <= n && textWithPlaceholders[i:i+6] == "[LINK:" {
			// Parse position number
			j := i + 6
			numStart := j
			for j < n && textWithPlaceholders[j] >= '0' && textWithPlaceholders[j] <= '9' {
				j++
			}
			if j > numStart && j < n && textWithPlaceholders[j] == ']' {
				position, err := strconv.Atoi(textWithPlaceholders[numStart:j])
				if err == nil {
					j++ // skip ']'
					// Find [/LINK]. The textStart < noEndTagFrom guard skips
					// scans that a previous failure already proved futile,
					// keeping the whole pass linear.
					textStart := j
					endTag := "[/LINK]"
					found := false
					for textStart < noEndTagFrom && j <= n-len(endTag) {
						if textWithPlaceholders[j] == '[' && textWithPlaceholders[j:j+7] == endTag {
							// Found the closing tag
							linkText := textWithPlaceholders[textStart:j]

							link, ok := linkMap[position]
							if !ok {
								// Unknown position, output just the text
								_, _ = tb.WriteString(linkText)
							} else {
								if linkText == "" {
									linkText = "Link " + strconv.Itoa(position)
								}
								switch format {
								case "markdown":
									writeMarkdownInline(tb, false, linkText, link.URL)
								case "html":
									writeHTMLAnchor(tb, link.URL, link.Title, linkText)
								default:
									_, _ = tb.WriteString(linkText)
								}
							}
							j += len(endTag)
							found = true
							break
						}
						j++
					}
					if !found && textStart < noEndTagFrom {
						// This scan ran to the end without a match: no closing
						// token exists at or after textStart.
						noEndTagFrom = textStart
					}
					if !found {
						// No closing [/LINK] tag present: write the opening tag
						// literally and resume scanning from textStart so that
						// trailing content is preserved rather than dropped.
						_, _ = tb.WriteString(textWithPlaceholders[i:textStart])
						j = textStart
					}
					i = j
					continue
				}
			}
			// Failed to parse, write as-is
			_ = tb.WriteByte(textWithPlaceholders[i])
			i++
		} else {
			_ = tb.WriteByte(textWithPlaceholders[i])
			i++
		}
	}

	return tb.String()
}

// imageScratchPool and linkScratchPool back extractImagesAndLinks' append
// scratch. Results are built in a pooled, capacity-retaining slice and copied
// out once at exact size: the previous fixed-cap preallocations
// (make([]LinkInfo, 0, 128) alone) cost ~9.5 KB per extraction on every
// document — link-sparse ones included — and was a profiled ~8% of allocation
// bytes. The pooled scratch amortizes growth to zero across calls, and the
// exact-size copy gives the returned slice no surplus capacity to carry into
// the Result (and into cloneResult's copies).
var (
	imageScratchPool = internal.NewSlicePool[ImageInfo](initialSliceCap)
	linkScratchPool  = internal.NewSlicePool[LinkInfo](initialSliceCap)
)

// extractImagesAndLinks collects images and/or links from node in a single
// document-order traversal, assigning positions with the same semantics the
// previous separate collectors used:
//   - image positions count only <img> with a valid src (parseImageNode accepts),
//     keeping them contiguous and in lockstep with the [IMAGE:n] placeholders the
//     text pass emits (which guards on imgHasValidSrc).
//   - link positions count every <a> (invalid-href anchors still consume a
//     number but are not appended), matching the [LINK:n] placeholders the text
//     pass emits.
//
// The two counters are independent, so one walk yields the same results as the
// previous two. Pass wantImages/wantLinks false to skip (and avoid allocating)
// a kind; the traversal runs at most once.
func (p *Processor) extractImagesAndLinks(node *stdxhtml.Node, wantImages, wantLinks bool) (images []ImageInfo, links []LinkInfo) {
	if !wantImages && !wantLinks {
		return nil, nil
	}

	var imgBuf *[]ImageInfo
	var linkBuf *[]LinkInfo
	if wantImages {
		imgBuf = imageScratchPool.Get()
		defer imageScratchPool.Put(imgBuf)
	}
	if wantLinks {
		linkBuf = linkScratchPool.Get()
		defer linkScratchPool.Put(linkBuf)
	}

	imgPos, linkPos := 0, 0
	// This collector observes exactly the same node set as the text pass
	// (extractTextWithStructure): unmatched [IMAGE:n]/[LINK:n] tokens would
	// otherwise leak into Text and their URLs would be dropped.
	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if n.Type != stdxhtml.ElementNode {
			return true
		}
		switch n.Data {
		case "img":
			if !wantImages {
				return true
			}
			img := p.parseImageNode(n, imgPos+1)
			if img.URL == "" {
				return true
			}
			imgPos++
			*imgBuf = append(*imgBuf, img)
		case "a":
			if !wantLinks {
				return true
			}
			linkPos++
			link := p.parseLinkNode(n)
			link.Position = linkPos
			if link.URL != "" {
				*linkBuf = append(*linkBuf, link)
			}
		}
		return true
	})

	// Copy out at exact size. make(..., 0) on an empty result keeps the
	// historical non-nil empty slice (marshals as [] rather than null).
	if wantImages {
		images = make([]ImageInfo, len(*imgBuf))
		copy(images, *imgBuf)
	}
	if wantLinks {
		links = make([]LinkInfo, len(*linkBuf))
		copy(links, *linkBuf)
	}
	return images, links
}

func (p *Processor) parseImageNode(n *stdxhtml.Node, position int) ImageInfo {
	img := ImageInfo{Position: position}

	for _, attr := range n.Attr {
		switch attr.Key {
		case "src":
			if !internal.IsValidURL(attr.Val) {
				return ImageInfo{}
			}
			img.URL = attr.Val
		case "alt":
			img.Alt = attr.Val
		case "title":
			img.Title = attr.Val
		case "width":
			img.Width = attr.Val
		case "height":
			img.Height = attr.Val
		}
	}

	if img.URL == "" {
		return ImageInfo{}
	}

	img.IsDecorative = img.Alt == ""
	return img
}

func (p *Processor) parseLinkNode(n *stdxhtml.Node) LinkInfo {
	link := LinkInfo{}

	for _, attr := range n.Attr {
		switch attr.Key {
		case "href":
			if !internal.IsValidURL(attr.Val) {
				return LinkInfo{}
			}
			link.URL = attr.Val
		case "title":
			link.Title = attr.Val
		case "rel":
			// Check for nofollow in rel attribute (case-insensitive, no allocation)
			if containsASCIIFold(attr.Val, "nofollow") {
				link.IsNoFollow = true
			}
		}
	}

	if link.URL == "" {
		return LinkInfo{}
	}

	link.Text = internal.GetTextContent(n)
	link.IsExternal = internal.IsExternalURL(link.URL)
	return link
}

// asciiWordSpace marks the ASCII bytes that end a word for countWords: the
// four whitespace bytes that separate words. A table lookup replaces the
// four-way comparison chain in the per-byte loop (countWords was a profiled
// ~2% of Extract CPU on a realistic document).
var asciiWordSpace = [256]bool{
	' ':  true,
	'\t': true,
	'\n': true,
	'\r': true,
}

// countWords counts whitespace-separated words, additionally counting each
// CJK ideograph, kana, or Hangul syllable as one word. CJK text is
// conventionally written without inter-word spaces, so a whitespace-only
// scan reported a whole paragraph as a single word and skewed WordCount and
// ReadingTime for CJK documents.
func (p *Processor) countWords(text string) int {
	if text == "" {
		return 0
	}
	n := len(text)
	count := 0
	inWord := false
	for i := 0; i < n; {
		c := text[i]
		if c < utf8.RuneSelf {
			if asciiWordSpace[c] {
				inWord = false
			} else if !inWord {
				inWord = true
				count++
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if isCJKRune(r) {
			count++ // each CJK character reads as one word
			inWord = false
		} else if !inWord {
			// Non-CJK multi-byte scripts (Cyrillic, Greek, accented Latin,
			// emoji, ...) follow the same run-of-non-space rule as ASCII.
			inWord = true
			count++
		}
		i += size
	}
	return count
}

// isCJKRune reports whether r is a CJK ideograph, kana, or Hangul syllable —
// the scripts whose text is conventionally written without inter-word spaces.
func isCJKRune(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
		r >= 0x3400 && r <= 0x4DBF, // CJK Extension A
		r >= 0xF900 && r <= 0xFAFF, // CJK Compatibility Ideographs
		r >= 0x3040 && r <= 0x30FF, // Hiragana / Katakana
		r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
		return true
	}
	return false
}

func (p *Processor) calculateReadingTime(wordCount int) time.Duration {
	if wordCount == 0 {
		return 0
	}
	minutes := float64(wordCount) / wordsPerMinute
	return time.Duration(minutes * float64(time.Minute))
}

// cloneResult returns a deep copy of a Result to prevent data races
// when the same cached entry is returned to multiple callers.
func cloneResult(r *Result) *Result {
	clone := *r
	if r.Images != nil {
		clone.Images = make([]ImageInfo, len(r.Images))
		copy(clone.Images, r.Images)
	}
	if r.Links != nil {
		clone.Links = make([]LinkInfo, len(r.Links))
		copy(clone.Links, r.Links)
	}
	if r.Videos != nil {
		clone.Videos = make([]VideoInfo, len(r.Videos))
		copy(clone.Videos, r.Videos)
	}
	if r.Audios != nil {
		clone.Audios = make([]AudioInfo, len(r.Audios))
		copy(clone.Audios, r.Audios)
	}
	return &clone
}
