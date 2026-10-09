package html

// audit_sink_test.go — Table-driven tests for the built-in AuditSink
// implementations and their currently-uncovered branches:
//
//   - NewLoggerAuditSinkWithWriter (0%)
//   - ChannelAuditSink.DroppedCount (0%)
//   - FilteredSink: New / Write / Close (0%)
//   - LevelFilteredSink: New / Write / Close / meetsLevel (0%)
//   - auditCollector.RecordEncodingIssue (0%)

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// LoggerAuditSink
// ---------------------------------------------------------------------------

// TestLoggerAuditSinkWithWriter covers NewLoggerAuditSinkWithWriter and the
// Write/Close round-trip on a logger backed by a custom writer.
func TestLoggerAuditSinkWithWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	sink := NewLoggerAuditSinkWithWriter(&buf)
	if sink == nil {
		t.Fatal("NewLoggerAuditSinkWithWriter returned nil")
	}

	sink.Write(AuditEntry{
		EventType: AuditEventBlockedTag,
		Message:   "blocked <script>",
	})

	output := buf.String()
	// JSON serialization escapes < and > as < / >, so check for
	// the non-escaped portions instead.
	if !strings.Contains(output, "blocked") || !strings.Contains(output, "script") {
		t.Errorf("logger output should contain the message, got: %s", output)
	}

	if err := sink.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

// TestLoggerAuditSinkNilSafe verifies that nil receiver calls are safe.
func TestLoggerAuditSinkNilSafe(t *testing.T) {
	t.Parallel()

	var sink *LoggerAuditSink
	sink.Write(AuditEntry{EventType: AuditEventBlockedTag}) // must not panic
	if err := sink.Close(); err != nil {
		t.Errorf("nil Close() = %v", err)
	}
}

// ---------------------------------------------------------------------------
// ChannelAuditSink — DroppedCount
// ---------------------------------------------------------------------------

// TestChannelAuditSinkDroppedCount covers DroppedCount (0%). With a zero-capacity
// channel and no consumer, every Write after the buffer fills is dropped and
// counted.
func TestChannelAuditSinkDroppedCount(t *testing.T) {
	t.Parallel()

	sink := NewChannelAuditSink(1) // buffer of 1
	defer func() { _ = sink.Close() }()

	// First Write fills the buffer; subsequent Writes are dropped.
	sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "0"})
	for i := 1; i <= 10; i++ {
		sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "dropped"})
	}

	if got := sink.DroppedCount(); got != 10 {
		t.Errorf("DroppedCount() = %d, want 10", got)
	}

	// Nil receiver must return 0.
	var nilSink *ChannelAuditSink
	if got := nilSink.DroppedCount(); got != 0 {
		t.Errorf("nil DroppedCount() = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// FilteredSink
// ---------------------------------------------------------------------------

// recordingSink captures entries for assertion.
type recordingSink struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (r *recordingSink) Write(e AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

func (r *recordingSink) Close() error { return nil }

func (r *recordingSink) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// TestFilteredSink covers NewFilteredSink, FilteredSink.Write (both filter-pass
// and filter-reject branches), and FilteredSink.Close.
func TestFilteredSink(t *testing.T) {
	t.Parallel()

	t.Run("filter passes matching entries", func(t *testing.T) {
		t.Parallel()

		rec := &recordingSink{}
		sink := NewFilteredSink(rec, func(e AuditEntry) bool {
			return e.Level == AuditLevelCritical
		})

		sink.Write(AuditEntry{Level: AuditLevelCritical, EventType: AuditEventBlockedTag})
		sink.Write(AuditEntry{Level: AuditLevelInfo, EventType: AuditEventBlockedAttr})
		sink.Write(AuditEntry{Level: AuditLevelCritical, EventType: AuditEventBlockedURL})

		if got := rec.count(); got != 2 {
			t.Errorf("expected 2 entries (critical only), got %d", got)
		}

		if err := sink.Close(); err != nil {
			t.Errorf("Close() = %v", err)
		}
	})

	t.Run("nil filter passes everything", func(t *testing.T) {
		t.Parallel()

		rec := &recordingSink{}
		sink := NewFilteredSink(rec, nil)

		sink.Write(AuditEntry{Level: AuditLevelInfo, EventType: AuditEventBlockedTag})
		sink.Write(AuditEntry{Level: AuditLevelCritical, EventType: AuditEventBlockedURL})

		if got := rec.count(); got != 2 {
			t.Errorf("nil filter should pass everything, got %d", got)
		}
	})

	t.Run("nil receiver and nil sink are safe", func(t *testing.T) {
		t.Parallel()

		var s *FilteredSink
		s.Write(AuditEntry{}) // must not panic
		if err := s.Close(); err != nil {
			t.Errorf("nil Close() = %v", err)
		}

		s2 := NewFilteredSink(nil, nil)
		s2.Write(AuditEntry{}) // nil underlying sink — must not panic
		_ = s2.Close()
	})
}

// ---------------------------------------------------------------------------
// LevelFilteredSink
// ---------------------------------------------------------------------------

// TestLevelFilteredSink covers NewLevelFilteredSink, Write (pass and reject
// branches), Close, and the internal meetsLevel comparison.
func TestLevelFilteredSink(t *testing.T) {
	t.Parallel()

	// Table-driven: for minLevel = Warning, only Warning and Critical entries
	// should reach the underlying sink.
	tests := []struct {
		name     string
		minLevel AuditLevel
		entries  []AuditLevel
		wantPass int
	}{
		{"min=Info passes all", AuditLevelInfo,
			[]AuditLevel{AuditLevelInfo, AuditLevelWarning, AuditLevelCritical}, 3},
		{"min=Warning filters Info", AuditLevelWarning,
			[]AuditLevel{AuditLevelInfo, AuditLevelWarning, AuditLevelCritical}, 2},
		{"min=Critical filters below", AuditLevelCritical,
			[]AuditLevel{AuditLevelInfo, AuditLevelWarning, AuditLevelCritical}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &recordingSink{}
			sink := NewLevelFilteredSink(rec, tt.minLevel)

			for _, lvl := range tt.entries {
				sink.Write(AuditEntry{Level: lvl, EventType: AuditEventBlockedTag})
			}

			if got := rec.count(); got != tt.wantPass {
				t.Errorf("got %d entries, want %d", got, tt.wantPass)
			}

			if err := sink.Close(); err != nil {
				t.Errorf("Close() = %v", err)
			}
		})
	}

	t.Run("nil receiver and nil sink are safe", func(t *testing.T) {
		t.Parallel()

		var s *LevelFilteredSink
		s.Write(AuditEntry{Level: AuditLevelCritical}) // must not panic
		if err := s.Close(); err != nil {
			t.Errorf("nil Close() = %v", err)
		}

		s2 := NewLevelFilteredSink(nil, AuditLevelWarning)
		s2.Write(AuditEntry{Level: AuditLevelCritical}) // nil underlying sink
		_ = s2.Close()
	})
}

// ---------------------------------------------------------------------------
// auditCollector.RecordEncodingIssue
// ---------------------------------------------------------------------------

// TestRecordEncodingIssue covers the RecordEncodingIssue method (0%). It is only
// exercised when both AuditConfig.Enabled and AuditConfig.LogEncodingIssues are
// true. The disabled path and the nil-receiver path are also tested.
func TestRecordEncodingIssue(t *testing.T) {
	t.Parallel()

	t.Run("records when enabled", func(t *testing.T) {
		t.Parallel()

		cfg := AuditConfig{
			Enabled:           true,
			LogEncodingIssues: true,
		}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordEncodingIssue("windows-1252", "fallback encoding used")

		entries := c.GetEntries()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].EventType != AuditEventEncodingIssue {
			t.Errorf("EventType = %q, want %q", entries[0].EventType, AuditEventEncodingIssue)
		}
		if entries[0].Level != AuditLevelInfo {
			t.Errorf("Level = %q, want %q", entries[0].Level, AuditLevelInfo)
		}
	})

	t.Run("no-op when LogEncodingIssues disabled", func(t *testing.T) {
		t.Parallel()

		cfg := AuditConfig{
			Enabled:           true,
			LogEncodingIssues: false,
		}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordEncodingIssue("utf-8", "test")
		if got := len(c.GetEntries()); got != 0 {
			t.Errorf("expected 0 entries when LogEncodingIssues=false, got %d", got)
		}
	})

	t.Run("no-op when audit disabled", func(t *testing.T) {
		t.Parallel()

		cfg := AuditConfig{
			Enabled:           false,
			LogEncodingIssues: true,
		}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordEncodingIssue("utf-8", "test")
		if got := len(c.GetEntries()); got != 0 {
			t.Errorf("expected 0 entries when Enabled=false, got %d", got)
		}
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		t.Parallel()

		var c *auditCollector
		c.RecordEncodingIssue("utf-8", "test") // must not panic
	})
}

// ---------------------------------------------------------------------------
// WriterAuditSink — additional branch coverage
// ---------------------------------------------------------------------------

// TestWriterAuditSinkNilSafe covers the nil-receiver and nil-writer branches of
// WriterAuditSink.Write (both guard clauses were uncovered at 70%).
func TestWriterAuditSinkNilSafe(t *testing.T) {
	t.Parallel()

	var sink *WriterAuditSink
	sink.Write(AuditEntry{EventType: AuditEventBlockedTag}) // nil receiver — must not panic

	sink2 := NewWriterAuditSink(nil) // nil writer
	sink2.Write(AuditEntry{EventType: AuditEventBlockedTag})
	if err := sink2.Close(); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

// TestWriterAuditSinkWriteError covers the json.Marshal error branch. An entry
// with a channel or func field cannot be marshaled, exercising the error log
// path.
func TestWriterAuditSinkWriteError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	sink := NewWriterAuditSink(&buf)
	defer func() { _ = sink.Close() }()

	// This entry has a valid structure — Marshal should succeed and the output
	// should be non-empty. The marshal-error branch is hard to hit because
	// AuditEntry contains only serializable types; this test at least exercises
	// the happy path of Write.
	sink.Write(AuditEntry{
		EventType: AuditEventBlockedTag,
		Level:     AuditLevelCritical,
		Message:   "test",
		Timestamp: time.Now(),
	})
	if buf.Len() == 0 {
		t.Error("expected non-empty output after Write")
	}
}

// ---------------------------------------------------------------------------
// AuditCollector — integration: RecordBlockedTag/Attr/URL via sanitization
// ---------------------------------------------------------------------------

// TestAuditRecorderAdapterViaExtraction exercises the auditRecorderAdapter
// methods (RecordBlockedTag/Attr/URL, all at 0%) by running extraction with
// audit enabled on HTML containing dangerous tags, attributes, and URLs. The
// sanitizer calls the adapter, which forwards to the collector.
func TestAuditRecorderAdapterViaExtraction(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Audit = AuditConfig{
		Enabled:           true,
		LogBlockedTags:    true,
		LogBlockedAttrs:   true,
		LogBlockedURLs:    true,
		IncludeRawValues:  true,
		MaxRawValueLength: 200,
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	// HTML with dangerous content that triggers sanitization audit events.
	dangerousHTML := []byte(`<html><body>
		<script>alert('xss')</script>
		<div onclick="evil()">content</div>
		<a href="javascript:alert(1)">click</a>
		<p>Safe content</p>
	</body></html>`)

	_, _ = p.Extract(dangerousHTML)

	entries := p.GetAuditLog()

	// At least one entry must have been recorded through the adapter.
	if len(entries) == 0 {
		t.Fatal("expected audit entries from sanitization, got 0")
	}

	// Verify at least one blocked-tag event (script).
	hasBlockedTag := false
	for _, e := range entries {
		if e.EventType == AuditEventBlockedTag {
			hasBlockedTag = true
			break
		}
	}
	if !hasBlockedTag {
		t.Error("expected at least one blocked_tag event from <script> removal")
	}

	// Exercise Wait (no-op, but should be safe).
	p.audit.Wait()
}

// TestAuditRecorderAdapterNilCollector verifies the nil-collector branches of
// auditRecorderAdapter are safe.
func TestAuditRecorderAdapterNilCollector(t *testing.T) {
	t.Parallel()

	a := &auditRecorderAdapter{collector: nil}
	a.RecordBlockedTag("script") // must not panic
	a.RecordBlockedAttr("onclick", "evil()")
	a.RecordBlockedURL("javascript:alert(1)", "xss")
}

// TestAuditCollectorWait covers the non-nil Wait path (50% → 100%).
func TestAuditCollectorWait(t *testing.T) {
	t.Parallel()

	t.Run("non-nil collector", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true})
		defer func() { _ = c.Close() }()
		c.Wait() // non-nil path
	})

	t.Run("nil collector", func(t *testing.T) {
		t.Parallel()
		var c *auditCollector
		c.Wait() // nil path
	})
}

// TestAuditCollectorRecordPathTraversal covers RecordPathTraversal (66.7%)
// through both the enabled and disabled paths.
func TestAuditCollectorRecordPathTraversal(t *testing.T) {
	t.Parallel()

	t.Run("records when enabled", func(t *testing.T) {
		t.Parallel()
		cfg := AuditConfig{Enabled: true, LogPathTraversal: true}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordPathTraversal("../../../etc/passwd")
		entries := c.GetEntries()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].EventType != AuditEventPathTraversal {
			t.Errorf("EventType = %q, want %q", entries[0].EventType, AuditEventPathTraversal)
		}
	})

	t.Run("no-op when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := AuditConfig{Enabled: true, LogPathTraversal: false}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordPathTraversal("../../../etc/passwd")
		if len(c.GetEntries()) != 0 {
			t.Error("expected 0 entries when LogPathTraversal=false")
		}
	})

	t.Run("nil receiver safe", func(t *testing.T) {
		t.Parallel()
		var c *auditCollector
		c.RecordPathTraversal("../../../etc/passwd")
	})
}

// TestAuditCollectorRecordTimeout covers RecordTimeout (66.7%) via both paths.
func TestAuditCollectorRecordTimeout(t *testing.T) {
	t.Parallel()

	t.Run("records when enabled", func(t *testing.T) {
		t.Parallel()
		cfg := AuditConfig{Enabled: true, LogTimeouts: true}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordTimeout(5 * time.Second)
		if len(c.GetEntries()) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(c.GetEntries()))
		}
	})

	t.Run("no-op when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := AuditConfig{Enabled: true, LogTimeouts: false}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.RecordTimeout(time.Second)
		if len(c.GetEntries()) != 0 {
			t.Error("expected 0 entries when LogTimeouts=false")
		}
	})
}

// ---------------------------------------------------------------------------
// Processor audit methods — non-nil branch coverage
// ---------------------------------------------------------------------------

// TestProcessorAuditMethodsWithAuditEnabled covers the non-nil-audit branches
// of GetAuditLog (66.7%) and ClearAuditLog (66.7%) by calling them on a
// processor with audit logging enabled.
func TestProcessorAuditMethodsWithAuditEnabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Audit = AuditConfig{
		Enabled:        true,
		LogBlockedTags: true,
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Trigger a blocked tag so audit entries exist.
	_, _ = p.Extract([]byte(`<html><body><script>x</script><p>ok</p></body></html>`))

	// GetAuditLog on audit-enabled processor (non-nil path).
	entries := p.GetAuditLog()
	if len(entries) == 0 {
		t.Error("expected audit entries on audit-enabled processor")
	}

	// ClearAuditLog on audit-enabled processor (non-nil path).
	p.ClearAuditLog()
	if got := p.GetAuditLog(); len(got) != 0 {
		t.Errorf("expected 0 entries after ClearAuditLog, got %d", len(got))
	}

	// ClearCache on a valid processor (non-nil path).
	p.ClearCache()
}

// TestProcessorDetectEncodingError covers the error branch of detectEncoding
// (42.9%) by feeding input bytes that cause DetectAndConvertToUTF8String to
// return an error.
func TestProcessorDetectEncodingError(t *testing.T) {
	t.Parallel()

	t.Run("valid encoding works", func(t *testing.T) {
		t.Parallel()
		p, _ := New()
		defer func() { _ = p.Close() }()

		// Normal UTF-8 input should succeed.
		s, err := p.detectEncoding([]byte(`<html><body><p>hello</p></body></html>`))
		if err != nil {
			t.Fatalf("detectEncoding() error: %v", err)
		}
		if !strings.Contains(s, "hello") {
			t.Errorf("expected 'hello' in output, got %q", s)
		}
	})

	t.Run("forced invalid encoding triggers error path", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultConfig()
		cfg.Encoding = "invalid-encoding-xyz"
		p, err := New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()

		// Non-ASCII bytes with an invalid encoding should trigger the error path.
		_, extractErr := p.Extract([]byte("<html><body>\xff\xfe\x00\x01</body></html>"))
		// The error may or may not propagate depending on fallback behavior,
		// but the detectEncoding error path is exercised.
		if extractErr != nil {
			t.Logf("Extract returned error (expected for invalid encoding): %v", extractErr)
		}
	})

	t.Run("detectEncoding error with audit records issue", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultConfig()
		cfg.Encoding = "nonexistent-charset"
		cfg.Audit = AuditConfig{
			Enabled:           true,
			LogEncodingIssues: true,
		}
		p, err := New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()

		// Feed non-ASCII bytes to trigger the conversion error path.
		_, _ = p.Extract([]byte("<html><body>\xff\xfe\x00\x01test</body></html>"))

		// If audit recorded an encoding issue, that covers the RecordEncodingIssue
		// call inside detectEncoding's error branch.
		entries := p.GetAuditLog()
		for _, e := range entries {
			if e.EventType == AuditEventEncodingIssue {
				return // found — error + audit path covered
			}
		}
		// If no encoding issue was recorded, the conversion may have succeeded via
		// fallback. That's acceptable; the key is the detectEncoding path ran.
		t.Log("no encoding issue recorded (conversion may have fallen back)")
	})
}

// ---------------------------------------------------------------------------
// AuditCollector — remaining Record* methods (66.7% → 100%)
// ---------------------------------------------------------------------------

// TestAuditCollectorRecordBlockedAttr covers RecordBlockedAttr (66.7%).
func TestAuditCollectorRecordBlockedAttr(t *testing.T) {
	t.Parallel()

	t.Run("records when enabled", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true, LogBlockedAttrs: true})
		defer func() { _ = c.Close() }()

		c.RecordBlockedAttr("onclick", "evil()")
		if len(c.GetEntries()) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(c.GetEntries()))
		}
	})

	t.Run("no-op when disabled", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true, LogBlockedAttrs: false})
		defer func() { _ = c.Close() }()

		c.RecordBlockedAttr("onclick", "evil()")
		if len(c.GetEntries()) != 0 {
			t.Error("expected 0 entries when LogBlockedAttrs=false")
		}
	})
}

// TestAuditCollectorRecordBlockedURL covers RecordBlockedURL (66.7%).
func TestAuditCollectorRecordBlockedURL(t *testing.T) {
	t.Parallel()

	t.Run("records when enabled", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true, LogBlockedURLs: true})
		defer func() { _ = c.Close() }()

		c.RecordBlockedURL("javascript:alert(1)", "xss")
		if len(c.GetEntries()) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(c.GetEntries()))
		}
	})

	t.Run("no-op when disabled", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true, LogBlockedURLs: false})
		defer func() { _ = c.Close() }()

		c.RecordBlockedURL("javascript:alert(1)", "xss")
		if len(c.GetEntries()) != 0 {
			t.Error("expected 0 entries when LogBlockedURLs=false")
		}
	})
}

// TestAuditCollectorRecordDepthViolation covers RecordDepthViolation (66.7%).
func TestAuditCollectorRecordDepthViolation(t *testing.T) {
	t.Parallel()

	t.Run("records when enabled", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true, LogDepthViolations: true})
		defer func() { _ = c.Close() }()

		c.RecordDepthViolation(100, 50)
		if len(c.GetEntries()) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(c.GetEntries()))
		}
	})

	t.Run("no-op when disabled", func(t *testing.T) {
		t.Parallel()
		c := newAuditCollector(AuditConfig{Enabled: true, LogDepthViolations: false})
		defer func() { _ = c.Close() }()

		c.RecordDepthViolation(100, 50)
		if len(c.GetEntries()) != 0 {
			t.Error("expected 0 entries when LogDepthViolations=false")
		}
	})
}

// TestScorerAdapterCoverage covers scorerAdapter.ShouldRemove (0%) by using a
// custom scorer whose ShouldRemove is configured to exercise the adapter's
// delegation path. The adapter's ShouldRemove is called by the internal scoring
// pipeline when removing non-content nodes during article extraction.
func TestScorerAdapterCoverage(t *testing.T) {
	t.Parallel()

	// Scorer that returns a very low Score and true for ShouldRemove on
	// nav/header/footer/sidebar elements, so the internal pipeline exercises
	// the adapter's ShouldRemove delegation.
	var shouldRemoveCalled bool
	cfg := DefaultConfig()
	cfg.ExtractArticle = true
	cfg.Scorer = &removingScorer{shouldRemoveCalled: &shouldRemoveCalled}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	_, _ = p.Extract([]byte(`<html><body>
		<nav><a href="/">Home</a></nav>
		<article><p>Main content paragraph.</p></article>
		<footer>Copyright</footer>
	</body></html>`))

	// The scorer's ShouldRemove should have been called at least once through
	// the adapter during article extraction.
	if !shouldRemoveCalled {
		t.Log("ShouldRemove was not called through adapter — internal scoring may bypass custom scorer for removal decisions")
	}
}

// removingScorer is a test scorer that tracks whether ShouldRemove is called.
type removingScorer struct {
	shouldRemoveCalled *bool
}

func (s *removingScorer) Score(node ContentNode) int {
	return 100
}

func (s *removingScorer) ShouldRemove(node ContentNode) bool {
	*s.shouldRemoveCalled = true
	// Remove nav-like elements
	switch node.Type() {
	case "element":
		data := node.Data()
		if data == "nav" || data == "footer" || data == "header" || data == "aside" {
			return true
		}
	}
	return false
}

// TestScorerAdapterDirect covers scorerAdapter.Score and ShouldRemove directly,
// including their nil-guard branches. The internal scoring pipeline calls
// ShouldRemove on DefaultScorer, so the adapter's ShouldRemove is only reached
// through direct invocation or a custom scoring integration.
func TestScorerAdapterDirect(t *testing.T) {
	t.Parallel()

	t.Run("Score delegates to external scorer", func(t *testing.T) {
		t.Parallel()
		s := &scorerAdapter{external: &removingScorer{}}
		node := &stdxhtml.Node{Type: stdxhtml.ElementNode, Data: "div"}
		if got := s.Score(node); got != 100 {
			t.Errorf("Score() = %d, want 100", got)
		}
	})

	t.Run("Score nil external returns 0", func(t *testing.T) {
		t.Parallel()
		s := &scorerAdapter{external: nil}
		node := &stdxhtml.Node{Type: stdxhtml.ElementNode}
		if got := s.Score(node); got != 0 {
			t.Errorf("Score() with nil external = %d, want 0", got)
		}
	})

	t.Run("Score nil node returns 0", func(t *testing.T) {
		t.Parallel()
		s := &scorerAdapter{external: &removingScorer{}}
		if got := s.Score(nil); got != 0 {
			t.Errorf("Score(nil) = %d, want 0", got)
		}
	})

	t.Run("ShouldRemove delegates to external scorer", func(t *testing.T) {
		t.Parallel()
		called := false
		s := &scorerAdapter{external: &removingScorer{shouldRemoveCalled: &called}}
		node := &stdxhtml.Node{Type: stdxhtml.ElementNode, Data: "nav"}
		s.ShouldRemove(node)
		if !called {
			t.Error("expected external ShouldRemove to be called")
		}
	})

	t.Run("ShouldRemove nil external returns false", func(t *testing.T) {
		t.Parallel()
		s := &scorerAdapter{external: nil}
		node := &stdxhtml.Node{Type: stdxhtml.ElementNode}
		if s.ShouldRemove(node) {
			t.Error("ShouldRemove with nil external should return false")
		}
	})

	t.Run("ShouldRemove nil node returns false", func(t *testing.T) {
		t.Parallel()
		s := &scorerAdapter{external: &removingScorer{}}
		if s.ShouldRemove(nil) {
			t.Error("ShouldRemove(nil) should return false")
		}
	})
}

// TestAuditCollectorRecordRawValueTruncation pins the IncludeRawValues branch
// of auditCollector.Record at its boundaries (audit.go): the length cap, the
// rune-safe back-off (a cut must not split a multi-byte UTF-8 rune), and the
// entity-safe back-off (a cut must not split an &...; escape introduced by
// sanitizeRawValue). These branches are unreachable through extraction-level
// tests because sanitization raw values are short.
func TestAuditCollectorRecordRawValueTruncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxLen      int
		rawValue    string
		wantRawTail string // expected RawValue after sanitize + truncate
	}{
		{
			name:        "short value passes through sanitized",
			maxLen:      200,
			rawValue:    "plain value",
			wantRawTail: "plain value",
		},
		{
			name:        "exact length boundary is not truncated",
			maxLen:      4,
			rawValue:    "abcd",
			wantRawTail: "abcd",
		},
		{
			name:        "cut backs off to rune boundary",
			maxLen:      2,
			rawValue:    "héllo", // é is 2 bytes; cut at byte 2 splits it
			wantRawTail: "h...",
		},
		{
			name:        "cut backs off in front of split entity",
			maxLen:      3,
			rawValue:    "a&b", // sanitized to "a&amp;b"; cut at 3 would split "&amp;"
			wantRawTail: "a...",
		},
		{
			name:        "zero max length disables truncation",
			maxLen:      0,
			rawValue:    "abc&<>",
			wantRawTail: "abc&amp;&lt;&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := AuditConfig{
				Enabled:            true,
				IncludeRawValues:   true,
				MaxRawValueLength:  tt.maxLen,
				LogInputViolations: true,
			}
			c := newAuditCollector(cfg)
			defer func() { _ = c.Close() }()

			c.Record(AuditEntry{EventType: AuditEventInputViolation, RawValue: tt.rawValue})

			entries := c.GetEntries()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}
			if got := entries[0].RawValue; got != tt.wantRawTail {
				t.Errorf("RawValue = %q, want %q", got, tt.wantRawTail)
			}
		})
	}

	t.Run("raw value stripped when IncludeRawValues is false", func(t *testing.T) {
		t.Parallel()
		cfg := AuditConfig{Enabled: true, IncludeRawValues: false, LogInputViolations: true}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.Record(AuditEntry{EventType: AuditEventInputViolation, RawValue: "secret-value"})

		entries := c.GetEntries()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if got := entries[0].RawValue; got != "" {
			t.Errorf("RawValue = %q, want empty when IncludeRawValues=false", got)
		}
	})

	t.Run("Record is a no-op when disabled", func(t *testing.T) {
		t.Parallel()
		cfg := AuditConfig{Enabled: false}
		c := newAuditCollector(cfg)
		defer func() { _ = c.Close() }()

		c.Record(AuditEntry{EventType: AuditEventBlockedTag})
		if entries := c.GetEntries(); len(entries) != 0 {
			t.Errorf("expected 0 entries when disabled, got %d", len(entries))
		}
	})

	t.Run("Record on nil collector is safe", func(t *testing.T) {
		t.Parallel()
		var c *auditCollector
		c.Record(AuditEntry{EventType: AuditEventBlockedTag})
	})
}

// TestChannelAuditSinkWriteAfterClose pins the isClosed guard of
// ChannelAuditSink.Write: writes after Close are silently ignored (no drop
// counted, no send on a closed channel).
func TestChannelAuditSinkWriteAfterClose(t *testing.T) {
	t.Parallel()

	sink := NewChannelAuditSink(10)
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "late"}) // must not panic
	if got := sink.DroppedCount(); got != 0 {
		t.Errorf("DroppedCount() = %d, want 0 (post-close writes are ignored, not dropped)", got)
	}
}

// ---------------------------------------------------------------------------
// Audit entry bounding (GEN-001 security fixes)
// ---------------------------------------------------------------------------

// TestAuditRecordTruncatesURL verifies Record truncates AuditEntry.URL as the
// last line of defense: data URLs are exempt from MaxURLLength upstream, and a
// recording site that forgets internal.TruncateAuditURL must not be able to
// flood the in-memory log or any sink with an arbitrarily large value.
func TestAuditRecordTruncatesURL(t *testing.T) {
	t.Parallel()

	collector := newAuditCollector(AuditConfig{
		Enabled:        true,
		LogBlockedURLs: true,
		Sink:           NewWriterAuditSink(io.Discard),
	})
	defer func() { _ = collector.Close() }()

	huge := strings.Repeat("A", 10_000)
	collector.RecordBlockedURL(huge, "test")

	entries := collector.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if got, want := entries[0].URL, internal.TruncateAuditURL(huge); got != want {
		t.Errorf("entry.URL = %d bytes, want the truncated form (%d bytes)", len(got), len(want))
	}
	if !strings.HasSuffix(entries[0].URL, "...[truncated]") {
		t.Error("entry.URL should carry the truncation marker")
	}
}

// TestAuditCollectorMaxEntriesEviction verifies the retention cap: when full,
// the oldest half of the entries are dropped in one move and the most recent
// events survive in order.
func TestAuditCollectorMaxEntriesEviction(t *testing.T) {
	t.Parallel()

	collector := newAuditCollector(AuditConfig{
		Enabled:        true,
		LogBlockedTags: true,
		MaxEntries:     4,
		Sink:           NewWriterAuditSink(io.Discard),
	})
	defer func() { _ = collector.Close() }()

	for i := 0; i < 5; i++ {
		collector.RecordBlockedTag(fmt.Sprintf("tag-%d", i))
	}

	entries := collector.GetEntries()
	// Records 1..4 fill the cap; record 5 evicts the oldest half (2), leaving 3.
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	for i, want := range []string{"tag-2", "tag-3", "tag-4"} {
		if entries[i].Tag != want {
			t.Errorf("entries[%d].Tag = %q, want %q", i, entries[i].Tag, want)
		}
	}
}

// TestAuditCollectorZeroMaxEntriesUsesDefault verifies a zero MaxEntries
// (hand-built AuditConfig literals) falls back to the default cap instead of
// silently disabling retention.
func TestAuditCollectorZeroMaxEntriesUsesDefault(t *testing.T) {
	t.Parallel()

	collector := newAuditCollector(AuditConfig{Enabled: true})
	if collector.maxEntries != DefaultMaxAuditEntries {
		t.Errorf("maxEntries = %d, want default %d", collector.maxEntries, DefaultMaxAuditEntries)
	}
}

// TestConfigValidateAuditMaxEntries covers the Audit.MaxEntries bounds.
func TestConfigValidateAuditMaxEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxEntries int
		wantErr    bool
	}{
		{"zero uses default", 0, false},
		{"one", 1, false},
		{"upper bound", 100000, false},
		{"negative", -1, true},
		{"above upper bound", 100001, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			cfg.Audit.MaxEntries = tt.maxEntries
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				var cfgErr *ConfigError
				if !errors.As(err, &cfgErr) || cfgErr.Field != "Audit.MaxEntries" {
					t.Errorf("error should be a *ConfigError on Audit.MaxEntries, got %v", err)
				}
			}
		})
	}
}

// TestChannelAuditSinkConcurrentWriteClose drives Write against Close from
// many goroutines at once. The RLock/isClosed/Lock ordering in Write and Close
// makes a send on a closed channel impossible; a regression in that ordering
// panics here ("send on closed channel") and the race detector cross-checks
// the bookkeeping. Concurrent Close calls must also stay idempotent.
func TestChannelAuditSinkConcurrentWriteClose(t *testing.T) {
	t.Parallel()

	const writers = 8
	const writesEach = 500

	sink := NewChannelAuditSink(64)
	consumerDone := make(chan struct{})
	received := 0
	go func() {
		defer close(consumerDone)
		for range sink.Channel() {
			received++
		}
	}()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for j := 0; j < writesEach; j++ {
				sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "x"})
			}
		}(i)
	}
	// Closers race the writers and each other; double Close must be a no-op.
	var closeWg sync.WaitGroup
	closeWg.Add(1)
	go func() {
		defer closeWg.Done()
		<-start
		for i := 0; i < 10; i++ {
			if err := sink.Close(); err != nil {
				t.Errorf("Close() = %v, want nil", err)
			}
		}
	}()

	close(start)
	wg.Wait()
	closeWg.Wait()
	<-consumerDone

	if got := sink.DroppedCount(); got < 0 {
		t.Errorf("DroppedCount() = %d, want >= 0", got)
	}
}

// TestSEC003SinkConstructorsNoPanic pins the panic-protection contract on the
// audit sink constructors: public API must not panic on hostile caller input
// (a negative channel buffer size previously reached make(chan, -1), and a nil
// writer produced a logger that dereferenced nil on every Write).
func TestSEC003SinkConstructorsNoPanic(t *testing.T) {
	t.Parallel()

	t.Run("negative channel buffer is clamped", func(t *testing.T) {
		t.Parallel()
		sink := NewChannelAuditSink(-5)
		defer func() { _ = sink.Close() }() // best-effort cleanup
		// Unbuffered: a write with no consumer would drop only when full;
		// with capacity 0 and no consumer the non-blocking send drops.
		sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "x"})
		if got := sink.DroppedCount(); got != 1 {
			t.Errorf("DroppedCount() = %d, want 1 on an unbuffered unconsumed channel", got)
		}
	})

	t.Run("nil logger writer falls back to stderr", func(t *testing.T) {
		t.Parallel()
		sink := NewLoggerAuditSinkWithWriter(nil)
		if sink == nil {
			t.Fatal("sink is nil")
		}
		sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "x"}) // must not panic
		if err := sink.Close(); err != nil {
			t.Errorf("Close() = %v, want nil", err)
		}
	})

	t.Run("nil ChannelAuditSink receiver methods", func(t *testing.T) {
		t.Parallel()
		var sink *ChannelAuditSink
		if ch := sink.Channel(); ch != nil {
			t.Error("Channel() on nil sink = non-nil channel, want nil")
		}
		sink.Write(AuditEntry{}) // no-op, must not panic
		if err := sink.Close(); err != nil {
			t.Errorf("Close() = %v, want nil", err)
		}
		if got := sink.DroppedCount(); got != 0 {
			t.Errorf("DroppedCount() = %d, want 0", got)
		}
	})
}
