package html

// panic_api_regression_test.go — SEC-003 regressions for public surfaces that
// could still panic.
//
// The panic-protection suite in panic_protection_test.go covers panic *recovery*
// on the extraction path (a misbehaving Scorer, AuditSink, or a hand-built
// zero-value Processor). This file covers the remaining class: exported
// functions, methods, and constructors that panicked *before* any recover could
// run, because the panic happened in caller-visible code or in allocation that
// recover cannot intercept.
//
// Each test drives the public API exactly as a hostile or careless caller
// would — huge sizes, zero values, typed-nil pointers inside non-nil
// interfaces — and fails if the call panics instead of returning.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// mustNotPanic runs fn and fails the test if it panics.
func mustNotPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", name, r)
		}
	}()
	fn()
}

// TestSEC003ChannelAuditSinkBufferClamped pins the constructor's size clamp.
//
// make(chan, n) charges n*sizeof(AuditEntry) up front, and an allocation the
// runtime cannot satisfy aborts the process with a fatal error — recover cannot
// catch it, so a recover-based guard would not save a caller who passed a
// hostile size. The bound is enforced before make.
func TestSEC003ChannelAuditSinkBufferClamped(t *testing.T) {
	t.Parallel()

	mustNotPanic(t, "NewChannelAuditSink(maxConfigAuditEntries+1)", func() {
		sink := NewChannelAuditSink(maxConfigAuditEntries + 1)
		defer func() { _ = sink.Close() }() // best-effort cleanup
		// The request was clamped to the ceiling rather than honored.
		if got := cap(sink.Channel()); got != maxChannelAuditSinkBuffer {
			t.Errorf("Channel() capacity = %d, want clamped %d", got, maxChannelAuditSinkBuffer)
		}
	})

	// A size that would have asked the runtime for hundreds of terabytes; the
	// pre-fix code aborted the process here.
	mustNotPanic(t, "NewChannelAuditSink(math.MaxInt-ish)", func() {
		sink := NewChannelAuditSink(1 << 62)
		defer func() { _ = sink.Close() }()
		if got := cap(sink.Channel()); got != maxChannelAuditSinkBuffer {
			t.Errorf("Channel() capacity = %d, want clamped %d", got, maxChannelAuditSinkBuffer)
		}
	})

	// Sizes at or below the ceiling keep the caller's value (no over-clamping).
	for _, size := range []int{0, 1, 64, maxChannelAuditSinkBuffer} {
		mustNotPanic(t, fmt.Sprintf("NewChannelAuditSink(%d)", size), func() {
			sink := NewChannelAuditSink(size)
			defer func() { _ = sink.Close() }()
			if got := cap(sink.Channel()); got != size {
				t.Errorf("Channel() capacity = %d, want %d", got, size)
			}
		})
	}
}

// TestSEC003ZeroValueSinksNoPanic covers sinks built as zero values from
// outside the package: the types are exported, their fields are not, so
// `var s html.LoggerAuditSink` is legal caller code and must not crash on use.
func TestSEC003ZeroValueSinksNoPanic(t *testing.T) {
	t.Parallel()

	entry := AuditEntry{EventType: AuditEventBlockedTag, Message: "x"}

	t.Run("zero LoggerAuditSink", func(t *testing.T) {
		t.Parallel()
		var sink LoggerAuditSink
		mustNotPanic(t, "LoggerAuditSink.Write", func() { sink.Write(entry) })
		mustNotPanic(t, "LoggerAuditSink.Close", func() { _ = sink.Close() })
	})

	t.Run("zero ChannelAuditSink", func(t *testing.T) {
		t.Parallel()
		var sink ChannelAuditSink
		// Write on a nil channel: the shielded select drops (and counts) the
		// entry instead of blocking or panicking.
		mustNotPanic(t, "ChannelAuditSink.Write", func() { sink.Write(entry) })
		mustNotPanic(t, "ChannelAuditSink.Close", func() { _ = sink.Close() })
		if got := sink.DroppedCount(); got != 1 {
			t.Errorf("DroppedCount() = %d, want 1 (nil channel drops)", got)
		}
		if ch := sink.Channel(); ch != nil {
			t.Error("Channel() on a zero-value sink = non-nil, want nil")
		}
	})

	t.Run("zero WriterAuditSink", func(t *testing.T) {
		t.Parallel()
		var sink WriterAuditSink
		mustNotPanic(t, "WriterAuditSink.Write", func() { sink.Write(entry) })
		mustNotPanic(t, "WriterAuditSink.Close", func() { _ = sink.Close() })
	})

	t.Run("zero composite sinks", func(t *testing.T) {
		t.Parallel()
		var multi MultiSink
		mustNotPanic(t, "MultiSink.Write", func() { multi.Write(entry) })
		mustNotPanic(t, "MultiSink.Close", func() { _ = multi.Close() })

		var filtered FilteredSink
		mustNotPanic(t, "FilteredSink.Write", func() { filtered.Write(entry) })
		mustNotPanic(t, "FilteredSink.Close", func() { _ = filtered.Close() })

		var level LevelFilteredSink
		mustNotPanic(t, "LevelFilteredSink.Write", func() { level.Write(entry) })
		mustNotPanic(t, "LevelFilteredSink.Close", func() { _ = level.Close() })
	})
}

// TestSEC003TypedNilErrorMethods pins the nil-receiver contract of the exported
// error types.
//
// These methods are reachable with a nil receiver through ordinary code: a
// typed-nil pointer stored in an `error` variable is non-nil as an interface,
// so `log.Printf("%v", err)`, errors.Is, and json.Marshal all invoke the
// method. Before the guard they dereferenced the nil pointer and panicked.
func TestSEC003TypedNilErrorMethods(t *testing.T) {
	t.Parallel()

	t.Run("InputError", func(t *testing.T) {
		t.Parallel()
		var e *InputError
		mustNotPanic(t, "(*InputError).Error", func() { _ = e.Error() })
		mustNotPanic(t, "(*InputError).Unwrap", func() { _ = e.Unwrap() })

		var err error = e
		if !errors.Is(err, ErrInputTooLarge) {
			t.Error("errors.Is(typed-nil *InputError, ErrInputTooLarge) = false, want true")
		}
		mustNotPanic(t, "fmt %v of typed-nil *InputError", func() { _ = fmt.Sprintf("%v", err) })
	})

	t.Run("ConfigError", func(t *testing.T) {
		t.Parallel()
		var e *ConfigError
		mustNotPanic(t, "(*ConfigError).Error", func() { _ = e.Error() })
		mustNotPanic(t, "(*ConfigError).Unwrap", func() { _ = e.Unwrap() })

		var err error = e
		if !errors.Is(err, ErrInvalidConfig) {
			t.Error("errors.Is(typed-nil *ConfigError, ErrInvalidConfig) = false, want true")
		}
	})

	t.Run("FileError", func(t *testing.T) {
		t.Parallel()
		var e *FileError
		mustNotPanic(t, "(*FileError).Error", func() { _ = e.Error() })
		mustNotPanic(t, "(*FileError).Unwrap", func() { _ = e.Unwrap() })
		mustNotPanic(t, "(*FileError).SafePath", func() { _ = e.SafePath() })
		mustNotPanic(t, "(*FileError).MarshalJSON", func() { _, _ = e.MarshalJSON() })

		var err error = e
		if !errors.Is(err, ErrInvalidFilePath) {
			t.Error("errors.Is(typed-nil *FileError, ErrInvalidFilePath) = false, want true")
		}

		// json.Marshal reaches MarshalJSON for a typed-nil pointer and must
		// produce the same "null" encoding/json would (absorbed from the
		// former TestSEC003NilFileErrorJSONMatchesEncodingJSON, which pinned
		// only this value through the same call).
		mustNotPanic(t, "json.Marshal(typed-nil *FileError)", func() {
			b, err := json.Marshal(err)
			if err != nil {
				t.Errorf("json.Marshal: %v", err)
			}
			if string(b) != "null" {
				t.Errorf("typed-nil *FileError marshals to %q, want \"null\"", b)
			}
		})
	})
}

// TestSEC003NilResultMarshalJSON pins Result.MarshalJSON's nil handling.
// encoding/json short-circuits a nil pointer before calling the method, but the
// method is exported and callers invoke it directly; it must return the same
// `null` json would produce rather than panicking.
func TestSEC003NilResultMarshalJSON(t *testing.T) {
	t.Parallel()

	var r *Result
	var got []byte
	mustNotPanic(t, "(*Result).MarshalJSON", func() {
		var err error
		got, err = r.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
	})
	if string(got) != "null" {
		t.Errorf("nil Result marshals to %q, want \"null\"", got)
	}

	// And through encoding/json, which must stay consistent with the direct call.
	mustNotPanic(t, "json.Marshal((*Result)(nil))", func() {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		if string(b) != "null" {
			t.Errorf("json.Marshal(nil Result) = %q, want \"null\"", b)
		}
	})
}

// TestSEC003NilFileErrorJSONMatchesEncodingJSON was merged into the FileError
// subtest of TestSEC003TypedNilErrorMethods above: both exercised
// json.Marshal on a typed-nil *FileError, and the subtest now pins the
// "null" output value the standalone test existed to check.

// TestSEC003ReadPageAPISurfaceNoPanic covers the reader-mode API family that
// postdates the original SEC-003 sweep: every exported Read* method and the
// ReadResult flatteners must return an error (or a string) rather than panic
// under the same hostile-call patterns the rest of this file drives — nil
// receiver, closed processor, nil context, and empty/binary/oversize input.
// The Read* methods wrap their bodies in recoverPanic, so a panic anywhere in
// the reader pipeline (segmentation, token stripping, section merge) surfaces
// as ErrInternalPanic; this test pins that no input shape bypasses it.
func TestSEC003ReadPageAPISurfaceNoPanic(t *testing.T) {
	t.Parallel()

	const doc = `<html><body><header><nav><a href="/x">x</a></nav></header>` +
		`<main><article><h1>T</h1><p>Hello <a href="/y">y</a>.</p>` +
		`<table><tr><td>c</td></tr></table></article></main>` +
		`<footer>f</footer></body></html>`

	t.Run("nil processor methods", func(t *testing.T) {
		t.Parallel()
		var p *Processor
		ctx := context.Background()
		calls := []struct {
			name string
			call func() error
		}{
			{"ReadPage", func() error { _, err := p.ReadPage([]byte(doc)); return err }},
			{"ReadPageWithContext", func() error { _, err := p.ReadPageWithContext(ctx, []byte(doc)); return err }},
			{"ReadPageFromFile", func() error { _, err := p.ReadPageFromFile("no-such.html"); return err }},
			{"ReadPageFromFileWithContext", func() error { _, err := p.ReadPageFromFileWithContext(ctx, "no-such.html"); return err }},
			{"ReadPageToMarkdown", func() error { _, err := p.ReadPageToMarkdown([]byte(doc)); return err }},
			{"ReadPageToMarkdownWithContext", func() error { _, err := p.ReadPageToMarkdownWithContext(ctx, []byte(doc)); return err }},
			{"ReadPageToMarkdownText", func() error { _, err := p.ReadPageToMarkdownText([]byte(doc)); return err }},
			{"ReadPageToMarkdownTextWithContext", func() error { _, err := p.ReadPageToMarkdownTextWithContext(ctx, []byte(doc)); return err }},
			{"ReadPageFromFileToMarkdown", func() error { _, err := p.ReadPageFromFileToMarkdown("no-such.html"); return err }},
			{"ReadPageFromFileToMarkdownWithContext", func() error { _, err := p.ReadPageFromFileToMarkdownWithContext(ctx, "no-such.html"); return err }},
			{"ReadPageFromFileToMarkdownText", func() error { _, err := p.ReadPageFromFileToMarkdownText("no-such.html"); return err }},
			{"ReadPageFromFileToMarkdownTextWithContext", func() error { _, err := p.ReadPageFromFileToMarkdownTextWithContext(ctx, "no-such.html"); return err }},
		}
		for _, c := range calls {
			mustNotPanic(t, "(*Processor)(nil)."+c.name, func() {
				if err := c.call(); err == nil {
					t.Errorf("%s on nil processor returned nil error, want ErrProcessorClosed", c.name)
				}
			})
		}
	})

	t.Run("package functions on hostile input", func(t *testing.T) {
		t.Parallel()
		oversize := make([]byte, DefaultConfig().MaxInputSize+1)
		inputs := map[string][]byte{
			"nil":     nil,
			"empty":   {},
			"blank":   []byte(" \t\n\r "),
			"binary":  {0xFF, 0xFE, 0x00, 0x01, 0x80},
			"literal": []byte(`[LINK:1] and [IMAGE:999] and [LINK: truncated markers`),
		}
		mustNotPanic(t, "ReadPage(oversize)", func() {
			if _, err := ReadPage(oversize); err == nil {
				t.Error("ReadPage(oversize) returned nil error, want ErrInputTooLarge")
			}
		})
		for name, in := range inputs {
			mustNotPanic(t, "ReadPage("+name+")", func() { _, _ = ReadPage(in) })
			mustNotPanic(t, "ReadPageToMarkdown("+name+")", func() { _, _ = ReadPageToMarkdown(in) })
			mustNotPanic(t, "ReadPageToMarkdownText("+name+")", func() { _, _ = ReadPageToMarkdownText(in) })
		}
		// An already-cancelled context takes the cooperative-exit path, not an
		// error-returning guard, on every WithContext variant.
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		mustNotPanic(t, "ReadPageWithContext(cancelled)", func() { _, _ = ReadPageWithContext(cancelled, []byte(doc)) })
		mustNotPanic(t, "ReadPageToMarkdownWithContext(cancelled)", func() { _, _ = ReadPageToMarkdownWithContext(cancelled, []byte(doc)) })
	})

	t.Run("nil context", func(t *testing.T) {
		t.Parallel()
		p, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = p.Close() }()
		// A nil Context dereferences inside recoverPanic's closure, so the
		// recovered value must surface as ErrInternalPanic, not a crash. The
		// nil ctx is the input under test (SA1012 is the point: callers do
		// pass nil, matching the ExtractBatch precedent in
		// panic_protection_test.go).
		mustNotPanic(t, "ReadPageWithContext(nil ctx)", func() {
			if _, err := p.ReadPageWithContext(nil, []byte(doc)); err == nil { //nolint:staticcheck // deliberately nil
				t.Error("ReadPageWithContext(nil ctx) returned nil error, want ErrInternalPanic")
			}
		})
		mustNotPanic(t, "ReadPageFromFileWithContext(nil ctx)", func() {
			_, _ = p.ReadPageFromFileWithContext(nil, "no-such.html") //nolint:staticcheck // deliberately nil
		})
	})

	t.Run("closed processor", func(t *testing.T) {
		t.Parallel()
		p, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		mustNotPanic(t, "ReadPage on closed processor", func() {
			if _, err := p.ReadPage([]byte(doc)); !errors.Is(err, ErrProcessorClosed) {
				t.Errorf("ReadPage after Close = %v, want ErrProcessorClosed", err)
			}
		})
		mustNotPanic(t, "ReadPageToMarkdownText on closed processor", func() {
			if _, err := p.ReadPageToMarkdownText([]byte(doc)); !errors.Is(err, ErrProcessorClosed) {
				t.Errorf("ReadPageToMarkdownText after Close = %v, want ErrProcessorClosed", err)
			}
		})
	})

	t.Run("ReadResult flatteners", func(t *testing.T) {
		t.Parallel()
		var nilResult *ReadResult
		mustNotPanic(t, "(*ReadResult)(nil).Render", func() {
			if s := nilResult.Render(); s != "" {
				t.Errorf("nil ReadResult.Render() = %q, want \"\"", s)
			}
		})
		mustNotPanic(t, "(*ReadResult)(nil).RenderText", func() {
			if s := nilResult.RenderText(); s != "" {
				t.Errorf("nil ReadResult.RenderText() = %q, want \"\"", s)
			}
		})
		// A zero-value result with unknown section kinds exercises the label
		// fallback (string(sec.Kind)) alongside the empty-text skip.
		zero := &ReadResult{Sections: []PageSection{{}, {Kind: SectionKind("weird")}}}
		mustNotPanic(t, "zero ReadResult.Render", func() { _ = zero.Render() })
		mustNotPanic(t, "zero ReadResult.RenderText", func() { _ = zero.RenderText() })
	})
}
