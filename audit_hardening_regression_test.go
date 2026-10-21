package html

// audit_hardening_regression_test.go — regressions for the D-002 P2 audit
// hardening fixes: Record caps every attacker-controlled string field (a
// hostile multi-megabyte on*-attribute name used to be retained in full in
// Attribute AND duplicated into Message per entry), a zero
// MaxRawValueLength now falls back to the default cap instead of silently
// disabling raw-value truncation, Config.Validate rejects negative
// MaxRawValueLength, and Processor.Close surfaces the audit sink's Close
// error instead of discarding it.

import (
	"errors"
	"strings"
	"testing"
)

func TestAuditFieldsCapped(t *testing.T) {
	t.Parallel()

	c := newAuditCollector(AuditConfig{
		Enabled:         true,
		LogBlockedAttrs: true,
	})

	hugeAttr := "on" + strings.Repeat("A", 1_000_000)
	c.RecordBlockedAttr(hugeAttr, "x")

	entries := c.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("GetEntries() = %d entries, want 1", len(entries))
	}
	e := entries[0]

	const capLimit = maxAuditFieldLength + len("...") + 64 // cap + marker + message prose headroom
	if len(e.Attribute) > maxAuditFieldLength+len("...") {
		t.Errorf("Attribute not capped: %d bytes", len(e.Attribute))
	}
	if len(e.Message) > capLimit {
		t.Errorf("Message not capped: %d bytes", len(e.Message))
	}
	if !strings.HasPrefix(e.Attribute, "onAAA") {
		t.Errorf("capped Attribute lost its head: %q", e.Attribute[:20])
	}
}

func TestMaxRawValueLengthZeroFallsBackToDefault(t *testing.T) {
	t.Parallel()

	// Hand-built literal with zero MaxRawValueLength — exactly the pattern
	// that previously disabled raw-value truncation entirely.
	c := newAuditCollector(AuditConfig{
		Enabled:          true,
		LogBlockedAttrs:  true,
		IncludeRawValues: true,
	})

	c.RecordBlockedAttr("onclick", strings.Repeat("v", 10_000))

	entries := c.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("GetEntries() = %d entries, want 1", len(entries))
	}
	max := DefaultMaxRawValueLength + len("...")
	if len(entries[0].RawValue) > max {
		t.Errorf("RawValue not truncated under the default cap: %d bytes (cap %d)", len(entries[0].RawValue), max)
	}
}

func TestConfigValidateRejectsNegativeMaxRawValueLength(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Audit.MaxRawValueLength = -1
	if err := cfg.Validate(); err == nil {
		t.Error("Validate accepted a negative Audit.MaxRawValueLength")
	}
}

// closeFailingSink is an AuditSink whose Close always fails, to pin that
// Processor.Close propagates sink close errors.
type closeFailingSink struct{}

func (closeFailingSink) Write(entry AuditEntry) {}
func (closeFailingSink) Close() error           { return errors.New("sink close failed") }

func TestCloseReturnsAuditSinkError(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Audit.Enabled = true
	cfg.Audit.Sink = closeFailingSink{}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := p.Close(); err == nil {
		t.Error("Close() = nil, want the audit sink's close error")
	} else if !strings.Contains(err.Error(), "sink close failed") {
		t.Errorf("Close() = %v, want the sink's close error", err)
	}
}
