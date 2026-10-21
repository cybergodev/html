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

		// json.Marshal reaches MarshalJSON for a typed-nil pointer.
		mustNotPanic(t, "json.Marshal(typed-nil *FileError)", func() {
			if _, err := json.Marshal(err); err != nil {
				t.Errorf("json.Marshal: %v", err)
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

// TestSEC003NilFileErrorJSONMatchesEncodingJSON keeps the FileError nil contract
// aligned with encoding/json: a typed-nil pointer passed through an interface
// still marshals to null rather than panicking or emitting an object.
func TestSEC003NilFileErrorJSONMatchesEncodingJSON(t *testing.T) {
	t.Parallel()

	var e *FileError
	var err error = e
	var got []byte
	mustNotPanic(t, "json.Marshal(typed-nil *FileError)", func() {
		b, jerr := json.Marshal(err)
		if jerr != nil {
			t.Fatalf("json.Marshal: %v", jerr)
		}
		got = b
	})
	if string(got) != "null" {
		t.Errorf("typed-nil *FileError marshals to %q, want \"null\"", got)
	}
}
