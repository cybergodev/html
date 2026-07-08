# Security Policy

## Overview

The `github.com/cybergodev/html` library is designed with security as a core principle. This document outlines the security measures, threat model, and best practices for using this library safely in production environments.

## Security Architecture

### Defense-in-Depth Strategy

The library implements multiple layers of security controls:

1. **Input Validation** - All inputs validated at API boundaries
2. **Resource Limits** - Configurable limits prevent resource exhaustion
3. **Safe Parsing** - Uses battle-tested `golang.org/x/net/html` parser
4. **Content Sanitization** - Removes dangerous content by default
5. **Thread Safety** - Concurrent access protected with proper synchronization
6. **Error Handling** - No panics, all errors returned explicitly

## Threat Model

### Protected Against

#### 1. Denial of Service (DoS)

**Input Size Attacks**
- **Threat**: Malicious actors submit extremely large HTML documents to exhaust memory
- **Mitigation**: `MaxInputSize` limit (default: 50MB)
- **Validation**: Input size checked before processing
- **Error**: Returns `ErrInputTooLarge` when exceeded

```go
cfg := html.DefaultConfig()
cfg.MaxInputSize = 10 * 1024 * 1024 // 10MB limit
processor, err := html.New(cfg)
```

**Deeply Nested HTML (Billion Laughs Attack)**
- **Threat**: Exponentially nested elements cause stack overflow or excessive processing
- **Mitigation**: `MaxDepth` limit (default: 500 levels)
- **Validation**: DOM depth validated using iterative traversal (avoids stack overflow in the validator itself)
- **Error**: Returns `ErrMaxDepthExceeded` when exceeded

```go
cfg := html.DefaultConfig()
cfg.MaxDepth = 50 // Limit nesting to 50 levels
```

**Processing Timeout**
- **Threat**: Complex HTML causes indefinite processing
- **Mitigation**: `ProcessingTimeout` (default: 30 seconds)
- **Validation**: Operations respect context deadlines
- **Error**: Returns `ErrProcessingTimeout` when exceeded
- **Goroutine Safety**: Maximum 1000 concurrent timeout goroutines to prevent resource exhaustion

**Cache Exhaustion**
- **Threat**: Unique inputs fill cache memory
- **Mitigation**: `MaxCacheEntries` limit (default: 2000) with eviction
- **Validation**: Cache size enforced with automatic eviction
- **Cleanup**: Background cleanup of expired entries (configurable interval, default: 5 minutes)

#### 2. Code Injection

**Script Injection (XSS)**
- **Threat**: Malicious `<script>` tags execute in downstream applications
- **Mitigation**: `EnableSanitization` removes dangerous tags and attributes
- **Default**: Enabled by default
- **Scope**: Dangerous content stripped from the parsed DOM tree during a dedicated sanitization pass

**Removed Tags** (10 tags):
- `<script>`, `<style>`, `<noscript>` - Script and style containers
- `<iframe>`, `<embed>`, `<object>` - Embedded content (potential XSS vectors)
- `<input>`, `<button>` - Form controls (potential CSRF/UI redress)
- `<svg>` - Can contain JavaScript and event handlers
- `<math>` - Can be abused for XSS in some browsers

> **Note:** The `<form>` container is intentionally **not** removed. Server frameworks
> (ASP.NET WebForms, JSF, JSP) commonly wrap the entire page body in a single
> `<form>`, so removing it would discard all visible content. Text extraction never
> renders or submits forms, so only the form controls (`<input>`/`<button>`) are
> stripped while `<form>` content is preserved.

```go
cfg := html.DefaultConfig()
cfg.EnableSanitization = true // Default: true
```

**Event Handler Injection**
- **Threat**: Inline event handlers execute JavaScript in downstream applications
- **Mitigation**: All event handler attributes removed during sanitization

**Removed Event Handler Attributes** (45+ attributes):
- **Mouse events**: `onclick`, `ondblclick`, `onmousedown`, `onmouseup`, `onmouseover`, `onmousemove`, `onmouseout`, `onmouseenter`, `onmouseleave`
- **Keyboard events**: `onkeydown`, `onkeypress`, `onkeyup`
- **Focus events**: `onfocus`, `onblur`
- **Form events**: `onsubmit`, `onreset`, `onchange`, `onselect`
- **UI events**: `onload`, `onunload`, `onabort`, `onerror`, `onresize`, `onscroll`, `oncontextmenu`
- **Drag and drop events**: `ondrag`, `ondragstart`, `ondragend`, `ondragenter`, `ondragleave`, `ondragover`, `ondrop`
- **Clipboard events**: `oncopy`, `oncut`, `onpaste`
- **Media events**: `onplay`, `onpause`, `onended`, `onvolumechange`
- **Mutation events**: `onDOMAttrModified`, `onDOMCharacterDataModified`, `onDOMNodeInserted`, `onDOMNodeRemoved`
- **Animation events**: `onanimationstart`, `onanimationend`, `onanimationiteration`
- **Transition events**: `ontransitionend`
- **Touch events**: `ontouchstart`, `ontouchend`, `ontouchmove`, `ontouchcancel`
- **Pointer events**: `onpointerdown`, `onpointerup`, `onpointermove`, `onpointercancel`, `onpointerenter`, `onpointerleave`, `onpointerover`, `onpointerout`
- **Other dangerous attributes**: `formaction` (can override form action), `autofocus` (can be used for phishing)

**Dangerous URI Schemes**
- **Threat**: `javascript:`, `vbscript:`, `data:` URLs execute code
- **Mitigation**: URI validation removes dangerous schemes
- **Blocked Schemes**: `javascript:`, `vbscript:`, `file:`
- **Validated Schemes**: `data:` URLs validated for size (100KB max), safe content, and media type whitelist
- **Additional Protection**: NFC Unicode normalization and fullwidth character normalization to prevent bypass attacks
- **SVG Block**: `image/svg+xml` data URLs explicitly blocked

#### 3. Resource Exhaustion

**Memory Exhaustion**
- **Threat**: Large documents or cache consume all available memory
- **Mitigation**:
  - Input size limits (50MB default)
  - Cache size limits with automatic eviction
  - Efficient string builders with pre-allocated capacity
  - No unbounded allocations

**Regex DoS (ReDoS)**
- **Threat**: Malicious input causes catastrophic backtracking in regex
- **Mitigation**:
  - All regex patterns pre-compiled at initialization
  - Regex only applied to size-limited content (`maxHTMLForRegex = 1MB`)
  - Match limits enforced (`maxRegexMatches = 1000`)
  - Simple, non-backtracking patterns used

```go
// Illustrative — the real path additionally gates on len > 0 and a
// HasMediaReference() pre-scan, so the regex only runs when a media
// signature is actually present (see extractVideos in media.go).
if len(htmlContent) <= maxHTMLForRegex {
    matches := videoRegex.FindAllString(htmlContent, maxRegexMatches)
}
```

**URL Length Attacks**
- **Threat**: Extremely long URLs in attributes cause memory issues
- **Mitigation**: URL length limited to 2000 characters (`MaxURLLength`)
- **Validation**: URLs exceeding limit are silently ignored

**Data URL Attacks**
- **Threat**: Large data URLs exhaust memory
- **Mitigation**: Data URL length limited to 100,000 bytes (`MaxDataURILength`)
- **Validation**: Data URLs exceeding limit are rejected
- **Media type whitelist**: Only safe types allowed (JPEG, PNG, GIF, WebP, BMP, ICO, AVIF, APNG, WOFF/WOFF2, TTF, OTF, PDF)
- **Base64 validation**: Character set validated for base64-encoded data URLs

#### 4. Cache Poisoning

**Hash Collision Attacks**
- **Threat**: Crafted inputs produce same cache key, causing incorrect results
- **Mitigation**: xxHash-style non-cryptographic hash with 128-bit output for cache keys
- **Key generation**: Config flags + format options + content (with 5-point sampling for large documents)
- **Large document handling**: Multi-point sampling (5 regions) for documents exceeding 64KB ensures modifications anywhere in the document are detected

**Cache Timing Attacks**
- **Threat**: Timing differences reveal cached vs. non-cached content
- **Mitigation**: Not applicable - library is not cryptographic
- **Note**: Cache hits are observable through statistics API (by design)

#### 5. Unsafe Package Usage

**Controlled Unsafe Operations**
The library uses `unsafe` in a limited number of locations for performance-critical operations:

1. **`internal/unsafe.go`** - Zero-allocation string/bytes conversions:
   - `StringToBytes(s string) []byte` - Read-only conversion for cache key generation and hashing
   - `BytesToString(b []byte) string` - For encoding detection output
   - Both functions require callers to respect the read-only contract (documented in function comments)
   - Safe memory-isolated encoding conversion is available via `DetectAndConvertToUTF8StringSafe` in `internal/encoding.go`

2. **`cachekey.go`** - Cache key hashing performance:
   - `hashMixBytesInline` uses `unsafe.Pointer` and `unsafe.Add` for 8-byte/32-byte block processing in the xxHash-style hash function
   - Read-only operation on byte slices from `StringToBytes` (which are backed by immutable strings)

3. **`internal/encoding.go`** - Encoding detection:
   - Uses `unsafe.Pointer` for performance-critical byte processing during character encoding detection

**Safety Properties**:
- All unsafe operations are read-only (no mutation of underlying data)
- String-to-bytes conversion: returned slices are never modified (backed by immutable Go strings)
- Full documentation in code comments at each usage site
- Safe memory-isolated alternatives available for encoding conversion

**Security Assessment**:
- All usage sites well-documented with safety comments
- Read-only operations (no mutation)
- Lifetime bounded to original string/slice scope
- No memory safety violations possible

### Not Protected Against

#### 1. Malicious HTML Content

**Phishing Content**
- **Scope**: Library extracts content as-is
- **Responsibility**: Application must validate extracted URLs and text
- **Recommendation**: Implement URL allowlists/blocklists in application layer

**Misleading Links**
- **Scope**: Library preserves link text and URLs without validation
- **Responsibility**: Application must verify link destinations
- **Detection**: Use `LinkInfo.IsExternal` to identify external links

#### 2. Privacy Concerns

**Tracking Pixels**
- **Scope**: Image URLs extracted without filtering
- **Responsibility**: Application must filter tracking domains
- **Detection**: Check `ImageInfo.Width` and `ImageInfo.Height` for 1x1 images

**Third-Party Embeds**
- **Scope**: Video/audio embed URLs extracted without validation
- **Responsibility**: Application must validate embed sources
- **Detection**: Use `VideoInfo.Type == "embed"` to identify embeds

#### 3. Content Validation

**Inappropriate Content**
- **Scope**: Library does not filter offensive or inappropriate text
- **Responsibility**: Application must implement content moderation

**Copyright Violations**
- **Scope**: Library extracts all content without rights verification
- **Responsibility**: Application must respect copyright and licensing

## Security Best Practices

### 1. Configuration Hardening

**Production Configuration**
```go
cfg := html.DefaultConfig()
cfg.MaxInputSize = 5 * 1024 * 1024     // 5MB - adjust based on use case
cfg.ProcessingTimeout = 10 * time.Second // Fail fast
cfg.MaxCacheEntries = 500                // Limit memory usage
cfg.CacheTTL = 30 * time.Minute          // Expire stale entries
cfg.WorkerPoolSize = 4                   // Limit concurrency
cfg.EnableSanitization = true            // Always enable
cfg.MaxDepth = 50                        // Prevent deep nesting
processor, err := html.New(cfg)
```

**Untrusted Input Configuration**
```go
cfg := html.HighSecurityConfig() // Pre-configured for strict security
processor, err := html.New(cfg)
```

Or manually:

```go
cfg := html.DefaultConfig()
cfg.MaxInputSize = 1 * 1024 * 1024    // 1MB - strict limit
cfg.ProcessingTimeout = 5 * time.Second // Very short timeout
cfg.MaxCacheEntries = 100               // Small cache
cfg.CacheTTL = 5 * time.Minute          // Short TTL
cfg.EnableSanitization = true           // Critical
cfg.MaxDepth = 30                       // Conservative depth
processor, err := html.New(cfg)
```

### 2. Input Validation

**Always Validate Before Processing**
```go
func processUserHTML(userInput []byte) (*html.Result, error) {
    // 1. Validate input is not empty
    if len(userInput) == 0 {
        return nil, errors.New("empty input")
    }

    // 2. Check size before passing to processor
    if len(userInput) > 10*1024*1024 {
        return nil, errors.New("input too large")
    }

    // 3. Process with configured limits
    return processor.Extract(userInput)
}
```

### 3. Error Handling

**Never Ignore Errors**
```go
result, err := processor.Extract(htmlBytes)
if err != nil {
    log.Printf("HTML extraction failed: %v", err)

    if errors.Is(err, html.ErrInputTooLarge) {
        return nil, fmt.Errorf("document too large: %w", err)
    }
    if errors.Is(err, html.ErrMaxDepthExceeded) {
        return nil, fmt.Errorf("document too complex: %w", err)
    }
    if errors.Is(err, html.ErrProcessingTimeout) {
        return nil, fmt.Errorf("processing timeout: %w", err)
    }

    return nil, err
}
```

### 4. Resource Management

**Always Close Processor**
```go
processor, err := html.New()
if err != nil {
    log.Fatal(err)
}
defer processor.Close() // Critical: releases cache and background goroutines

result, err := processor.Extract(htmlBytes)
```

**Monitor Resource Usage**
```go
stats := processor.GetStatistics()
if stats.TotalProcessed > 0 {
    log.Printf("Cache hit rate: %.2f%%",
        float64(stats.CacheHits)/float64(stats.TotalProcessed)*100)
}
```

### 5. Output Sanitization

**Sanitize Extracted Content**
```go
result, err := processor.Extract(htmlBytes)
if err != nil {
    return err
}

// Sanitize URLs before use
for _, link := range result.Links {
    if !isAllowedDomain(link.URL) {
        log.Printf("Blocked suspicious URL: %s", link.URL)
        continue
    }
}

// Filter tracking pixels
for _, img := range result.Images {
    if img.Width == "1" && img.Height == "1" {
        log.Printf("Skipping tracking pixel: %s", img.URL)
        continue
    }
}
```

### 6. Concurrent Usage

**Thread-Safe by Design**
```go
processor, err := html.New()
if err != nil {
    log.Fatal(err)
}
defer processor.Close()

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func(content []byte) {
        defer wg.Done()
        result, err := processor.Extract(content)
        // ... handle result
    }(htmlDocs[i])
}
wg.Wait()
```

**Batch Processing**
```go
// Use built-in batch processing for efficiency
br := processor.ExtractBatch(htmlDocs)

// Check individual results
for i, result := range br.Results {
    if result == nil {
        log.Printf("Item %d failed: %v", i, br.Errors[i])
        continue
    }
}
```

### 7. Audit Logging

**Enable Audit Logging for Security Monitoring**
```go
cfg := html.DefaultConfig()
cfg.Audit = html.AuditConfig{
    Enabled:            true,
    LogBlockedTags:     true,
    LogBlockedAttrs:    true,
    LogBlockedURLs:     true,
    LogInputViolations: true,
    LogDepthViolations: true,
    LogTimeouts:        true,
    LogEncodingIssues:  true,
    LogPathTraversal:   true,
    Sink:               html.NewChannelAuditSink(100),
}
processor, err := html.New(cfg)
```

## Dependency Security

### Minimal Attack Surface

**Dependencies**:
- `golang.org/x/net/html` - Official Go supplementary network libraries (HTML5 parser)
- `golang.org/x/text` - Official Go text processing libraries (encoding detection)

Both maintained by the Go team. No transitive dependencies beyond these.

**Dependency Verification**
```bash
# Verify dependencies
go mod verify

# Check for known vulnerabilities
govulncheck ./...

# Update to latest secure version
go get -u golang.org/x/net/html
go get -u golang.org/x/text
go mod tidy
```

### Supply Chain Security

**Module Verification**
```bash
# Enable Go module checksum database
export GOSUMDB=sum.golang.org

# Verify module authenticity
go mod download
go mod verify
```

## Vulnerability Reporting

### Reporting a Vulnerability

If you discover a security vulnerability, please follow responsible disclosure:

1. **Do Not** open a public GitHub issue
2. **Email** security concerns to: [security contact - to be added]
3. **Include**:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

### Response Timeline

- **24 hours**: Initial acknowledgment
- **7 days**: Preliminary assessment
- **30 days**: Fix development and testing
- **Coordinated disclosure**: After fix is released

### Security Updates

Security fixes are released as:
- **Patch versions** (1.0.x) for minor issues
- **Minor versions** (1.x.0) for moderate issues
- **Immediate patches** for critical vulnerabilities

## Security Testing

### Automated Testing

The library includes comprehensive security-focused tests with **85%+ code coverage**:

```bash
# Run all tests including security tests
go test -v ./...

# Run with race detector
go test -race ./...

# Run concurrency stress tests
go test -v -run TestStress

# Run with coverage
go test -cover ./...
```

### Security Test Coverage

- Input validation tests (size limits, depth limits, timeout)
- XSS prevention tests (sanitization effectiveness)
- Resource limit tests (cache, memory, allocations)
- Concurrent access tests (race conditions, thread safety)
- URL validation tests (dangerous schemes, length limits, Unicode bypass)
- Data URL validation tests (size limits, content validation, media type whitelist)
- Path traversal tests (file extraction security)
- Panic recovery tests (defense-in-depth)

### Fuzzing

**Recommended Fuzzing Targets**
```go
func FuzzExtract(f *testing.F) {
    processor, err := html.New()
    if err != nil {
        log.Fatal(err)
    }
    defer processor.Close()

    f.Fuzz(func(t *testing.T, data []byte) {
        processor.Extract(data)
        // Should never panic
    })
}
```

### Security Audit Checklist

- [x] Input validation on all public APIs
- [x] Resource limits enforced
- [x] No panics in production code (panic recovery in all public methods)
- [x] All errors properly handled
- [x] Thread-safe concurrent access
- [x] Controlled `unsafe` package usage (read-only operations with full documentation)
- [x] No arbitrary code execution
- [x] Regex patterns are safe (pre-compiled, size-limited, match-limited)
- [x] Cache keys use collision-resistant hashing
- [x] Dependencies are up to date

## Compliance

### OWASP Top 10 (2021)

| Risk | Status | Mitigation |
|------|--------|------------|
| A01: Broken Access Control | N/A | Library does not handle authentication |
| A02: Cryptographic Failures | N/A | No cryptographic operations |
| A03: Injection | Protected | Content sanitization, no code execution |
| A04: Insecure Design | Protected | Defense-in-depth architecture |
| A05: Security Misconfiguration | Protected | Secure defaults, validation |
| A06: Vulnerable Components | Protected | Minimal dependencies, regular updates |
| A07: Authentication Failures | N/A | Library does not handle authentication |
| A08: Software/Data Integrity | Protected | Module verification, checksums |
| A09: Logging Failures | Configurable | Audit logging available (opt-in) |
| A10: Server-Side Request Forgery | N/A | Library does not make network requests |

### CWE Coverage

- **CWE-20**: Input Validation - Comprehensive validation
- **CWE-79**: XSS - Content sanitization with tag/attribute removal
- **CWE-89**: SQL Injection - N/A (no database access)
- **CWE-119**: Buffer Overflow - Go memory safety
- **CWE-190**: Integer Overflow - Validated limits
- **CWE-400**: Resource Exhaustion - Resource limits (DoS protection)
- **CWE-770**: Allocation without Limits - Size limits enforced
- **CWE-835**: Infinite Loop - Depth limits, timeouts

## Security Changelog

> This section highlights security-relevant changes per release. For the complete
> change history (features, performance, bug fixes), see [CHANGES.md](../CHANGES.md).

### v1.4.4 (2026-06-26)
- `ExtractAllLinks` and link extraction now honor `ProcessingTimeout` via cooperative cancellation; a fired deadline surfaces as `ErrProcessingTimeout` instead of running to completion — DoS hardening for the link path
- Zero-byte cache-key sentinel replaced with an explicit `hasCacheKey` flag — a legitimately all-zero hash was previously treated as "no key" (cache-integrity fix)
- `<form>` content is now preserved (controls `<input>`/`<button>` still dropped) — fixes empty extraction on ASP.NET/JSF/JSP pages whose body is wrapped in `<form>`
- `RestartCleanup` resets `cleanupOnce` under `cleanupMu` and documents the caller-serialization requirement, closing a latent race on the reset path

### v1.4.3 (2026-06-24)
- Background goroutines (audit sink write, cache TTL cleanup) now recover panics, so a panicking user-supplied `AuditSink` or an internal cleanup fault can no longer crash the process
- Audit sink writes are now synchronous on the recording goroutine, removing the unbounded-goroutine amplification an adversarial document could cause (DoS hardening)

### v1.4.2 (2026-06-17)
- Cache miss no longer returns a result aliased with the cached entry — fixes a concurrent-mutation data race that could silently corrupt the cached value
- Depth validation now runs **before** sanitization, bounding every recursive pass to `MaxDepth` (DoS hardening against pathologically deep documents)
- Pooled (package-level) calls no longer stop and respawn the cache cleanup goroutine on every invocation, eliminating goroutine churn

### v1.4.1 (2026-05-07)
- `AllowedBaseDir` config field restricts file operations to paths under a specified directory
- `truncateAuditURL` caps data URLs at 256 chars in audit logs, preventing disk exhaustion
- `FileError.MarshalJSON` uses `SafePath()` to prevent raw filesystem path disclosure in JSON responses
- Fixed `ChannelAuditSink` Write/Close race condition; cache hit returns a deep copy (`cloneResult`) to prevent data races

### v1.4.0 (2026-04-29)
- CSS sanitization stripping `expression()`, `behavior:`, `-moz-binding:`, `javascript:`, `vbscript:`
- `escapeMarkdownText()` prevents Markdown injection via unescaped `]`, `[`, `\`
- `sanitizeRawValue()` HTML-escapes audit `RawValue` fields to prevent XSS in downstream log renderers
- Closed `isDangerousScheme` fullwidth-Unicode bypass via `normalizeFullwidthToASCII`
- `maxBatchSize` (10,000) early rejection prevents OOM on extreme input

### v1.3.2 (2026-03-23)
- Defense-in-depth for fast-path vulnerabilities (S-06 to S-16)
- Enhanced numeric entity validation prevents DoS via long strings
- Improved cache key collision resistance (5-point sampling)
- `maxWalkDepth` (50,000 nodes) prevents memory exhaustion attacks

### v1.3.0 (2026-03-03)
- Library confirmed fully thread-safe (100+ race detection iterations)
- Cache key hash length increased 8 → 16 bytes (better collision resistance)
- Goroutine leak in `withTimeout()` bounded by maximum concurrent-operation limit

### v1.2.0 (2026-02-07)
- Path traversal protection in `ExtractFromFile()` with stricter checks
- CSS injection protection in style attributes
- Enhanced URI protocol validation for dangerous schemes
- ReDoS protection and null byte injection prevention in URLs/paths

### v1.1.1 (2026-02-02)
- URI validation reordered to block dangerous protocols first (`javascript:`, `vbscript:`, `file:`)
- Closed leading/trailing whitespace bypass
- Corrected data URL character validation (base64-encoded content)

### v1.1.0 (2026-02-01)
- Enhanced data URL validation; data URI size limit (100KB max)
- Early input size validation (DoS prevention)
- Improved DoS prevention and safe HTML entity handling
- Table extraction with colspan and alignment support (opt-in via `TableFormat`)

### v1.0.6 (2026-01-19)
- HTML sanitization removes iframe/embed/object; media extracted before sanitization
- `NormalizeBaseURL` skips non-HTTP protocols (`data:`, `javascript:`, `mailto:`)

### v1.0.5 (2026-01-14)
- Enhanced data URL validation (safe ASCII only, blocks injection characters)
- Early input size validation (moved to function entry for DoS prevention)

### v1.0.4 (2026-01-12)
- **CRITICAL**: Fixed thread-safety issues (concurrent map access); eliminated data races
- Fixed XSS vulnerability in HTML output with proper escaping
- Reduced `MaxInputSize` from 1GB to 50MB for DoS protection

### Initial Release (v1.0.0)
- Input validation on all public APIs
- Resource limits with configurable defaults
- Content sanitization enabled by default
- Thread-safe concurrent access
- Comprehensive security testing

## Additional Resources

- [Go Security Best Practices](https://golang.org/doc/security)
- [OWASP HTML Sanitization](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)
- [CWE Top 25](https://cwe.mitre.org/top25/)
- [Go Race Detector](https://golang.org/doc/articles/race_detector.html)

## License

This security policy is part of the `github.com/cybergodev/html` project and is licensed under the same terms as the project.
