package html

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/html/internal"
)

// AuditEventType represents the type of security audit event.
type AuditEventType string

const (
	// AuditEventBlockedTag indicates a dangerous HTML tag was blocked.
	AuditEventBlockedTag AuditEventType = "blocked_tag"
	// AuditEventBlockedAttr indicates a dangerous attribute was blocked.
	AuditEventBlockedAttr AuditEventType = "blocked_attr"
	// AuditEventBlockedURL indicates a dangerous URL was blocked.
	AuditEventBlockedURL AuditEventType = "blocked_url"
	// AuditEventInputViolation indicates an input validation failure.
	AuditEventInputViolation AuditEventType = "input_violation"
	// AuditEventDepthViolation indicates a depth limit violation.
	AuditEventDepthViolation AuditEventType = "depth_violation"
	// AuditEventTimeout indicates a processing timeout occurred.
	AuditEventTimeout AuditEventType = "timeout"
	// AuditEventEncodingIssue indicates an encoding detection issue.
	AuditEventEncodingIssue AuditEventType = "encoding_issue"
	// AuditEventPathTraversal indicates a path traversal attempt.
	AuditEventPathTraversal AuditEventType = "path_traversal"
)

// AuditLevel represents the severity level of an audit event.
type AuditLevel string

const (
	// AuditLevelInfo indicates informational events.
	AuditLevelInfo AuditLevel = "info"
	// AuditLevelWarning indicates warning events.
	AuditLevelWarning AuditLevel = "warning"
	// AuditLevelCritical indicates critical security events.
	AuditLevelCritical AuditLevel = "critical"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	// Timestamp is when the event was recorded (UTC).
	Timestamp time.Time `json:"timestamp"`
	// EventType categorizes the audit event (e.g., blocked_tag, timeout).
	EventType AuditEventType `json:"event_type"`
	// Level is the severity of the event (info, warning, or critical).
	Level AuditLevel `json:"level"`
	// Message is a human-readable description of the event.
	Message string `json:"message"`
	// Tag is the HTML tag name that triggered the event, when applicable.
	Tag string `json:"tag,omitempty"`
	// Attribute is the HTML attribute name that triggered the event, when applicable.
	Attribute string `json:"attribute,omitempty"`
	// URL is the resource URL that triggered the event, when applicable.
	URL string `json:"url,omitempty"`
	// InputSize is the actual input size for input-violation events.
	InputSize int `json:"input_size,omitempty"`
	// MaxSize is the configured maximum input size for input-violation events.
	MaxSize int `json:"max_size,omitempty"`
	// Depth is the actual nesting depth for depth-violation events.
	Depth int `json:"depth,omitempty"`
	// MaxDepth is the configured maximum depth for depth-violation events.
	MaxDepth int `json:"max_depth,omitempty"`
	// Path is the file path for path-traversal events, sanitized to the base
	// filename so filesystem layouts never reach audit sinks. The full raw
	// path remains available via RawValue when AuditConfig.IncludeRawValues
	// is enabled (centrally sanitized and length-capped by Record).
	Path string `json:"path,omitempty"`
	// RawValue is the raw attribute or URL value that triggered the event.
	// Only populated when AuditConfig.IncludeRawValues is true.
	RawValue string `json:"raw_value,omitempty"`
	// Metadata holds additional event-specific key-value pairs.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// AuditSink defines the interface for audit log destinations.
// Implementations can write audit entries to various destinations
// such as files, databases, message queues, or external logging services.
type AuditSink interface {
	// Write writes an audit entry to the sink.
	// It should be non-blocking and thread-safe.
	Write(entry AuditEntry)
	// Close releases any resources used by the sink.
	Close() error
}

// AuditConfig configures the audit logging behavior.
type AuditConfig struct {
	// Enabled determines whether audit logging is active.
	Enabled bool `json:"enabled"`

	// LogBlockedTags logs when dangerous HTML tags are removed.
	LogBlockedTags bool `json:"log_blocked_tags"`

	// LogBlockedAttrs logs when dangerous attributes are removed.
	LogBlockedAttrs bool `json:"log_blocked_attrs"`

	// LogBlockedURLs logs when dangerous URLs are blocked.
	LogBlockedURLs bool `json:"log_blocked_urls"`

	// LogInputViolations logs input size and validation violations.
	LogInputViolations bool `json:"log_input_violations"`

	// LogDepthViolations logs depth limit violations.
	LogDepthViolations bool `json:"log_depth_violations"`

	// LogTimeouts logs processing timeout events.
	LogTimeouts bool `json:"log_timeouts"`

	// LogEncodingIssues logs encoding detection issues.
	LogEncodingIssues bool `json:"log_encoding_issues"`

	// LogPathTraversal logs path traversal attempts.
	LogPathTraversal bool `json:"log_path_traversal"`

	// Sink is the destination for audit entries.
	// If nil and audit is enabled, a default logger is used.
	Sink AuditSink `json:"-"`

	// IncludeRawValues includes raw attribute/URL values in logs.
	// Warning: This may log potentially malicious content.
	IncludeRawValues bool `json:"include_raw_values"`

	// MaxRawValueLength limits the length of raw values logged.
	// 0 uses DefaultMaxRawValueLength (200). Negative values are rejected by
	// Config.Validate.
	MaxRawValueLength int `json:"max_raw_value_length"`

	// MaxEntries caps how many audit entries GetAuditLog retains in memory.
	// When the cap is reached, the oldest half of the retained entries are
	// dropped so the log always keeps the most recent events. A hostile
	// document can generate thousands of blocked-tag/attr/URL events; without
	// a cap these accumulated without bound on a long-lived processor.
	// 0 uses DefaultMaxAuditEntries. Negative values and values above 100000
	// are rejected by Config.Validate. Retention is always bounded — feed an
	// AuditSink instead for unbounded external logging.
	MaxEntries int `json:"max_entries"`
}

// DefaultAuditConfig returns the default audit configuration.
// By default, audit logging is disabled.
func DefaultAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:            false,
		LogBlockedTags:     true,
		LogBlockedAttrs:    true,
		LogBlockedURLs:     true,
		LogInputViolations: true,
		LogDepthViolations: true,
		LogTimeouts:        true,
		LogEncodingIssues:  true,
		LogPathTraversal:   true,
		IncludeRawValues:   false,
		MaxRawValueLength:  DefaultMaxRawValueLength,
		MaxEntries:         DefaultMaxAuditEntries,
	}
}

// HighSecurityAuditConfig returns an audit configuration for high-security environments.
// All logging is enabled with raw value inclusion for forensic analysis.
func HighSecurityAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:            true,
		LogBlockedTags:     true,
		LogBlockedAttrs:    true,
		LogBlockedURLs:     true,
		LogInputViolations: true,
		LogDepthViolations: true,
		LogTimeouts:        true,
		LogEncodingIssues:  true,
		LogPathTraversal:   true,
		IncludeRawValues:   true,
		MaxRawValueLength:  500,
		MaxEntries:         DefaultMaxAuditEntries,
	}
}

// auditCollector collects audit entries during processing.
// It is designed to be thread-safe for concurrent use.
type auditCollector struct {
	mu                sync.Mutex
	entries           []AuditEntry
	config            AuditConfig
	sink              AuditSink
	maxEntries        int // clamped (>= 0) copy of config.MaxEntries used for eviction
	maxRawValueLength int // resolved (non-zero) copy of config.MaxRawValueLength used for truncation
}

// newAuditCollector creates a new audit collector with the given configuration.
func newAuditCollector(config AuditConfig) *auditCollector {
	sink := config.Sink
	if sink == nil && config.Enabled {
		sink = NewLoggerAuditSink()
	}
	// A zero MaxEntries (hand-built AuditConfig literals, the zero Config)
	// falls back to the default cap rather than disabling retention: silent
	// retention loss on upgrade would be worse than the cap.
	maxEntries := config.MaxEntries
	if maxEntries == 0 {
		maxEntries = DefaultMaxAuditEntries
	}
	// A zero MaxRawValueLength mirrors the MaxEntries policy: fall back to
	// the default cap rather than silently disabling raw-value truncation.
	// A hand-built AuditConfig{Enabled: true, IncludeRawValues: true} literal
	// (exactly the pattern the tests use) would otherwise pass every length
	// check at zero and retain attacker-sized raw values in full.
	maxRawValueLength := config.MaxRawValueLength
	if maxRawValueLength == 0 {
		maxRawValueLength = DefaultMaxRawValueLength
	}
	// entries is allocated lazily on the first Record: a disabled or quiet
	// collector (the common case) never appends and carries no slice at all.
	return &auditCollector{
		config:            config,
		sink:              sink,
		maxEntries:        maxEntries,
		maxRawValueLength: maxRawValueLength,
	}
}

// maxAuditFieldLength caps the attacker-controlled non-URL string fields of
// an audit entry, mirroring the URL cap's order of magnitude.
const maxAuditFieldLength = 256

// truncateAuditField caps s at maxAuditFieldLength bytes with a rune-safe
// cut (never splitting a multi-byte UTF-8 sequence), appending "..." when it
// truncates. See Record.
func truncateAuditField(s string) string {
	if len(s) <= maxAuditFieldLength {
		return s
	}
	cut := maxAuditFieldLength
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "..."
}

// Record adds an audit entry to the collector.
func (c *auditCollector) Record(entry AuditEntry) {
	if c == nil || !c.config.Enabled {
		return
	}

	entry.Timestamp = time.Now().UTC()

	// SECURITY: entry.URL is attacker-controlled (blocked href/src values) and
	// can be arbitrarily large — data URLs are exempt from MaxURLLength
	// upstream, and a recording site that forgets internal.TruncateAuditURL
	// would otherwise flood the in-memory log and every sink with up to
	// MaxInputSize bytes per entry. Truncate here as the last line of defense,
	// using the same policy as the sanitizer call sites.
	entry.URL = internal.TruncateAuditURL(entry.URL)

	// SECURITY: the same flooding concern applies to the other attacker-
	// controlled fields. The sanitizer blocks on*-prefixed attribute names of
	// unbounded length, and every Record* site formats the raw value into
	// Message as well — one hostile attribute could previously retain
	// megabytes twice over (Attribute + Message) per entry, across the
	// in-memory log and every sink. Cap them here with the same last-line-of-
	// defense posture as the URL truncation above.
	entry.Attribute = truncateAuditField(entry.Attribute)
	entry.Tag = truncateAuditField(entry.Tag)
	entry.Path = truncateAuditField(entry.Path)
	entry.Message = truncateAuditField(entry.Message)

	// Remove raw values if not configured to include them
	if !c.config.IncludeRawValues {
		entry.RawValue = ""
	} else if entry.RawValue != "" {
		// Sanitize before truncating so the length cap applies to the final
		// logged bytes, then back the cut point off so it neither splits a
		// multi-byte UTF-8 rune nor lands inside an HTML escape sequence
		// sanitizeRawValue introduced (a cut inside "&amp;" would persist a
		// broken entity). maxRawValueLength is resolved non-zero by
		// newAuditCollector (0 falls back to the default cap).
		entry.RawValue = sanitizeRawValue(entry.RawValue)
		if maxLen := c.maxRawValueLength; len(entry.RawValue) > maxLen {
			cut := maxLen
			// A cut at index cut is rune-safe when the byte there starts a
			// rune rather than a UTF-8 continuation byte (0b10xxxxxx).
			for cut > 0 && entry.RawValue[cut]&0xC0 == 0x80 {
				cut--
			}
			// Post-sanitize, every '&' starts one of the known &...; escapes;
			// when the last one before cut has not yet seen its ';', cutting
			// there would split it — cut in front of the '&' instead.
			if a := strings.LastIndexByte(entry.RawValue[:cut], '&'); a >= 0 &&
				strings.IndexByte(entry.RawValue[a:cut], ';') < 0 {
				cut = a
			}
			entry.RawValue = entry.RawValue[:cut] + "..."
		}
	}

	c.mu.Lock()
	// Retention is capped so a hostile document (thousands of blocked
	// attributes/URLs) cannot grow the in-memory log without bound on a
	// long-lived processor. When full, drop the oldest half in one move —
	// evicting one per append would make every post-cap Record an O(n) copy,
	// exactly the workload an attacker controls. maxEntries <= 0 disables
	// retention entirely (entries still go to the sink); through New it is
	// unreachable, since newAuditCollector maps 0 to the default cap and
	// Config.Validate rejects negatives.
	if c.maxEntries > 0 {
		if len(c.entries) >= c.maxEntries {
			keep := c.maxEntries / 2
			copy(c.entries, c.entries[len(c.entries)-keep:])
			c.entries = c.entries[:keep]
		}
		c.entries = append(c.entries, entry)
	}
	c.mu.Unlock()

	// Write to the sink synchronously. AuditSink.Write is contractually
	// non-blocking and thread-safe, so invoking it inline on the recording
	// goroutine is correct and avoids spawning an unbounded number of
	// goroutines — one per entry — which an adversarial document (thousands of
	// blocked attributes) could amplify under an enabled audit config.
	//
	// The sink is a user-supplied extension point (custom AuditSink, or a
	// filtered/multi-sink wrapping one), so Write may panic. Recover so a
	// misbehaving sink cannot crash the process: the audit subsystem is
	// best-effort and must never propagate a panic to the public-API caller
	// (SEC-003). The recovered value is intentionally discarded — there is no
	// safe channel to report a panic originating inside the audit path itself.
	if c.sink != nil {
		func() {
			defer func() {
				_ = recover() // swallow sink panic; see comment above
			}()
			c.sink.Write(entry)
		}()
	}
}

// Wait is retained as a no-op safety hook.
//
// Sink writes are now synchronous (see Record), so there are no pending async
// writes to drain: once a Record* method returns, the entry has already been
// handed to the sink. Callers (notably putPooledProcessor) and tests still call
// Wait() to mark the "all audit work for this use is done" point; it remains
// nil-safe and safe to call any number of times.
func (c *auditCollector) Wait() {
	if c == nil {
		return
	}
}

// want reports whether the collector is enabled and the per-event flag admits
// an entry, so the eight Record* methods share one gate instead of a
// hand-copied three-clause guard (a future clause — rate limiting, sampling —
// lands here once).
func (c *auditCollector) want(logged bool) bool {
	return c.config.Enabled && logged
}

// RecordBlockedTag records a blocked tag event.
func (c *auditCollector) RecordBlockedTag(tag string) {
	if c == nil || !c.want(c.config.LogBlockedTags) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventBlockedTag,
		Level:     AuditLevelWarning,
		Message:   fmt.Sprintf("Blocked dangerous HTML tag: %s", tag),
		Tag:       tag,
	})
}

// RecordBlockedAttr records a blocked attribute event.
func (c *auditCollector) RecordBlockedAttr(attr, value string) {
	if c == nil || !c.want(c.config.LogBlockedAttrs) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventBlockedAttr,
		Level:     AuditLevelWarning,
		Message:   fmt.Sprintf("Blocked dangerous attribute: %s", attr),
		Attribute: attr,
		RawValue:  value,
	})
}

// RecordBlockedURL records a blocked URL event.
func (c *auditCollector) RecordBlockedURL(url, reason string) {
	if c == nil || !c.want(c.config.LogBlockedURLs) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventBlockedURL,
		Level:     AuditLevelWarning,
		Message:   fmt.Sprintf("Blocked dangerous URL: %s", reason),
		URL:       url,
		RawValue:  url,
	})
}

// RecordInputViolation records an input validation violation.
func (c *auditCollector) RecordInputViolation(size, maxSize int, violationType string) {
	if c == nil || !c.want(c.config.LogInputViolations) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventInputViolation,
		Level:     AuditLevelCritical,
		Message:   fmt.Sprintf("Input validation violation: %s", violationType),
		InputSize: size,
		MaxSize:   maxSize,
	})
}

// RecordDepthViolation records a depth limit violation.
func (c *auditCollector) RecordDepthViolation(depth, maxDepth int) {
	if c == nil || !c.want(c.config.LogDepthViolations) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventDepthViolation,
		Level:     AuditLevelWarning,
		Message:   fmt.Sprintf("Depth limit exceeded: %d > %d", depth, maxDepth),
		Depth:     depth,
		MaxDepth:  maxDepth,
	})
}

// RecordTimeout records a processing timeout event.
func (c *auditCollector) RecordTimeout(timeout time.Duration) {
	if c == nil || !c.want(c.config.LogTimeouts) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventTimeout,
		Level:     AuditLevelWarning,
		Message:   fmt.Sprintf("Processing timeout exceeded: %v", timeout),
	})
}

// RecordEncodingIssue records an encoding detection issue.
func (c *auditCollector) RecordEncodingIssue(encoding, message string) {
	if c == nil || !c.want(c.config.LogEncodingIssues) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventEncodingIssue,
		Level:     AuditLevelInfo,
		Message:   message,
		Metadata:  map[string]any{"encoding": encoding},
	})
}

// RecordPathTraversal records a path traversal attempt.
//
// SECURITY: Path is stored as the base filename only (per AuditEntry.Path's
// documented contract) — full filesystem paths from caller input must not
// flow into audit sinks that applications forward to logs and SIEMs. The
// unmodified path is preserved in RawValue for forensics, subject to
// AuditConfig.IncludeRawValues and Record's central sanitization.
func (c *auditCollector) RecordPathTraversal(path string) {
	if c == nil || !c.want(c.config.LogPathTraversal) {
		return
	}
	c.Record(AuditEntry{
		EventType: AuditEventPathTraversal,
		Level:     AuditLevelCritical,
		Message:   "Path traversal attempt detected",
		Path:      safeBaseName(path),
		RawValue:  path,
	})
}

// GetEntries returns all collected audit entries.
func (c *auditCollector) GetEntries() []AuditEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]AuditEntry, len(c.entries))
	copy(result, c.entries)
	// Metadata maps are reference types: without a per-entry copy, a caller
	// mutating result[i].Metadata would write through to the retained
	// entries and to every later GetEntries call.
	for i := range result {
		if md := result[i].Metadata; md != nil {
			cp := make(map[string]any, len(md))
			maps.Copy(cp, md)
			result[i].Metadata = cp
		}
	}
	return result
}

// Clear removes all collected entries.
func (c *auditCollector) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil // drop the backing array entirely, not just its length
}

// Close closes the audit collector and its sink.
// Sink writes are synchronous, so no draining is needed before closing.
//
// Like Record, this recovers a panic from a user-supplied AuditSink.Close so a
// misbehaving sink cannot crash the process via the canonical
// `defer processor.Close()` pattern (SEC-003: the audit subsystem is best-effort
// and must never propagate a panic to the public-API caller). Close has an error
// return, so the recovered value is surfaced there rather than discarded.
func (c *auditCollector) Close() (err error) {
	if c == nil {
		return nil
	}
	if c.sink != nil {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%w: audit sink close panic: %v", ErrInternalPanic, r)
			}
		}()
		return c.sink.Close()
	}
	return nil
}

// ---- Built-in Audit Sinks ----

// LoggerAuditSink writes audit entries to a standard logger.
type LoggerAuditSink struct {
	logger *log.Logger
}

// NewLoggerAuditSink creates a new sink that writes to the default logger.
func NewLoggerAuditSink() *LoggerAuditSink {
	return &LoggerAuditSink{
		logger: log.New(os.Stderr, "[AUDIT] ", log.LstdFlags),
	}
}

// NewLoggerAuditSinkWithWriter creates a new sink that writes to the specified
// writer. A nil w falls back to the default stderr logger (the
// NewLoggerAuditSink destination): log.Logger would otherwise dereference the
// nil writer on every Write, and this constructor is public API, which must not
// hand back an object that panics on use.
func NewLoggerAuditSinkWithWriter(w io.Writer) *LoggerAuditSink {
	if w == nil {
		w = os.Stderr
	}
	return &LoggerAuditSink{
		logger: log.New(w, "[AUDIT] ", log.LstdFlags),
	}
}

// Write writes an audit entry to the logger.
//
// A nil logger is treated as "no destination" rather than dereferenced: the
// exported type can be built as a zero value from outside the package
// (`var s html.LoggerAuditSink`), and a logging call in that state must not
// panic the caller's process (SEC-003).
func (s *LoggerAuditSink) Write(entry AuditEntry) {
	if s == nil || s.logger == nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		s.logger.Printf("Failed to marshal audit entry: %v", err)
		return
	}
	s.logger.Printf("%s", data)
}

// Close is a no-op for the logger sink.
func (s *LoggerAuditSink) Close() error {
	return nil
}

// ChannelAuditSink sends audit entries to a channel.
// Useful for integrating with external logging systems.
// Use DroppedCount() to check if any entries were discarded due to a full buffer.
type ChannelAuditSink struct {
	ch       chan AuditEntry
	closeMu  sync.RWMutex
	isClosed bool
	dropped  atomic.Int64
}

// maxChannelAuditSinkBuffer bounds the channel buffer NewChannelAuditSink will
// allocate. A channel is charged its full capacity up front (AuditEntry is
// ~192 B), so an unchecked size is a one-line out-of-memory abort:
// make(chan, 1<<40) asks for ~200 TB and the runtime aborts the process, which
// recover cannot intercept (a fatal allocation failure is not a panic).
// The ceiling is the audit subsystem's own retention cap (maxConfigAuditEntries):
// a backlog larger than the number of entries the library itself will ever
// retain is not a buffer, it is an allocation accident.
const maxChannelAuditSinkBuffer = maxConfigAuditEntries

// NewChannelAuditSink creates a new sink that sends entries to a channel.
// The channel should be consumed promptly: when the buffer is full, further
// entries are dropped and counted (see DroppedCount) rather than blocking.
//
// A negative bufferSize is clamped to 0 (an unbuffered channel), and a size
// above maxChannelAuditSinkBuffer is clamped to that ceiling: make would panic
// on a negative size and abort the process on an allocation that cannot be
// satisfied, and this constructor is public API, which must not panic on
// caller input.
func NewChannelAuditSink(bufferSize int) *ChannelAuditSink {
	if bufferSize < 0 {
		bufferSize = 0
	}
	if bufferSize > maxChannelAuditSinkBuffer {
		bufferSize = maxChannelAuditSinkBuffer
	}
	return &ChannelAuditSink{
		ch: make(chan AuditEntry, bufferSize),
	}
}

// Write sends an audit entry to the channel.
// If the channel is full, the entry is dropped and the drop counter is incremented.
func (s *ChannelAuditSink) Write(entry AuditEntry) {
	if s == nil {
		return
	}
	s.closeMu.RLock()
	defer s.closeMu.RUnlock()
	if s.isClosed {
		return
	}
	select {
	case s.ch <- entry:
	default:
		s.dropped.Add(1)
	}
}

// Channel returns the channel for receiving audit entries. It returns nil for
// a nil sink, matching the nil-safety of Write, Close, and DroppedCount.
func (s *ChannelAuditSink) Channel() <-chan AuditEntry {
	if s == nil {
		return nil
	}
	return s.ch
}

// DroppedCount returns the number of audit entries that were discarded
// because the channel buffer was full.
func (s *ChannelAuditSink) DroppedCount() int64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

// Close closes the channel.
// Safe to call multiple times - only the first call will close the channel.
//
// A zero-value sink has no channel to close, so Close is a no-op there rather
// than a panic ("close of nil channel"): the exported type may be built as a
// zero value from outside the package (SEC-003).
func (s *ChannelAuditSink) Close() error {
	if s == nil || s.ch == nil {
		return nil
	}
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.isClosed {
		return nil
	}
	s.isClosed = true
	close(s.ch)
	return nil
}

// WriterAuditSink writes audit entries to an io.Writer as JSON lines.
type WriterAuditSink struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewWriterAuditSink creates a new sink that writes to the specified writer.
func NewWriterAuditSink(w io.Writer) *WriterAuditSink {
	return &WriterAuditSink{writer: w}
}

// Write writes an audit entry to the writer.
// Marshal errors are logged to the standard logger for diagnosability,
// consistent with LoggerAuditSink.Write.
func (s *WriterAuditSink) Write(entry AuditEntry) {
	if s == nil || s.writer == nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal audit entry: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.writer.Write(data); err != nil {
		// A failed data write drops the entry entirely; say so, mirroring the
		// marshal branch above — a silent return hid real delivery failures
		// (disk full, closed file, broken pipe).
		log.Printf("Failed to write audit entry: %v", err)
		return
	}
	_, _ = s.writer.Write([]byte("\n")) // best-effort newline after successful data write
}

// Close is a no-op for the writer sink.
func (s *WriterAuditSink) Close() error {
	return nil
}

// MultiSink wraps multiple sinks into a single sink.
type MultiSink struct {
	sinks []AuditSink
}

// NewMultiSink creates a new sink that writes to all provided sinks.
func NewMultiSink(sinks ...AuditSink) *MultiSink {
	return &MultiSink{sinks: sinks}
}

// Write writes an audit entry to all sinks.
func (s *MultiSink) Write(entry AuditEntry) {
	if s == nil {
		return
	}
	for _, sink := range s.sinks {
		if sink != nil {
			sink.Write(entry)
		}
	}
}

// Close closes all sinks and joins every error returned (Go 1.20+ errors.Join)
// rather than silently dropping all but the last. For an audit subsystem, losing
// close errors from earlier sinks hides real delivery failures.
func (s *MultiSink) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	for _, sink := range s.sinks {
		if sink != nil {
			if err := sink.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// FilteredSink filters audit entries before writing to the underlying sink.
type FilteredSink struct {
	sink   AuditSink
	filter func(AuditEntry) bool
}

// NewFilteredSink creates a new sink that filters entries.
func NewFilteredSink(sink AuditSink, filter func(AuditEntry) bool) *FilteredSink {
	return &FilteredSink{sink: sink, filter: filter}
}

// Write writes an audit entry if it passes the filter.
func (s *FilteredSink) Write(entry AuditEntry) {
	if s == nil || s.sink == nil {
		return
	}
	if s.filter == nil || s.filter(entry) {
		s.sink.Write(entry)
	}
}

// Close closes the underlying sink.
func (s *FilteredSink) Close() error {
	if s == nil || s.sink == nil {
		return nil
	}
	return s.sink.Close()
}

// LevelFilteredSink only writes entries at or above the specified level.
type LevelFilteredSink struct {
	sink     AuditSink
	minLevel AuditLevel
}

// NewLevelFilteredSink creates a new sink that filters by level.
// NewLevelFilteredSink wraps sink and forwards only entries at minLevel or
// above. minLevel is lowercased so a hand-written "Critical" ranks the same
// as the AuditLevelCritical constant instead of falling into the unknown-level
// rank 0, which passes every entry.
func NewLevelFilteredSink(sink AuditSink, minLevel AuditLevel) *LevelFilteredSink {
	return &LevelFilteredSink{sink: sink, minLevel: AuditLevel(strings.ToLower(string(minLevel)))}
}

// Write writes an audit entry if it meets the minimum level.
func (s *LevelFilteredSink) Write(entry AuditEntry) {
	if s == nil || s.sink == nil {
		return
	}
	if s.meetsLevel(entry.Level) {
		s.sink.Write(entry)
	}
}

// Close closes the underlying sink.
func (s *LevelFilteredSink) Close() error {
	if s == nil || s.sink == nil {
		return nil
	}
	return s.sink.Close()
}

// auditLevelRank orders audit levels by severity. Hoisted to package scope so
// LevelFilteredSink.meetsLevel does not allocate a map on every Write call.
var auditLevelRank = map[AuditLevel]int{
	AuditLevelInfo:     0,
	AuditLevelWarning:  1,
	AuditLevelCritical: 2,
}

func (s *LevelFilteredSink) meetsLevel(level AuditLevel) bool {
	return auditLevelRank[level] >= auditLevelRank[s.minLevel]
}

// sanitizeRawValue escapes HTML-special characters in audit raw values
// to prevent XSS when logs are rendered in HTML contexts (browsers, SIEM dashboards).
func sanitizeRawValue(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// *auditCollector implements internal.AuditRecorder directly: its methods are
// nil-receiver safe and match the interface's signatures one for one. A former
// 28-line forwarding adapter added only a redundant nil check on top.
var _ internal.AuditRecorder = (*auditCollector)(nil)
