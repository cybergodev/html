package html

// errors_test.go - Tests for error types and constructors
// This file tests all error types, their constructors, and errors.Is() support.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestInputError(t *testing.T) {
	t.Parallel()

	t.Run("error message format", func(t *testing.T) {
		err := newInputError("Extract", 10000, 5000, nil)
		msg := err.Error()

		if msg == "" {
			t.Error("Error message should not be empty")
		}
		if !strings.Contains(msg, "Extract") {
			t.Errorf("Error message should contain operation: %s", msg)
		}
		if !strings.Contains(msg, "10000") {
			t.Errorf("Error message should contain size: %s", msg)
		}
		if !strings.Contains(msg, "5000") {
			t.Errorf("Error message should contain max: %s", msg)
		}
	})

	t.Run("error message with underlying error", func(t *testing.T) {
		underlying := errors.New("underlying error")
		err := newInputError("Extract", 10000, 5000, underlying)
		msg := err.Error()

		if !strings.Contains(msg, "underlying error") {
			t.Errorf("Error message should contain underlying error: %s", msg)
		}
	})

	t.Run("field values", func(t *testing.T) {
		err := newInputError("Op", 100, 50, nil)

		if err.Op != "Op" {
			t.Errorf("Op = %q, want 'Op'", err.Op)
		}
		if err.Size != 100 {
			t.Errorf("Size = %d, want 100", err.Size)
		}
		if err.MaxSize != 50 {
			t.Errorf("MaxSize = %d, want 50", err.MaxSize)
		}
	})
}

// TestErrorUnwrap consolidates the errors.Is unwrap contracts for all three
// error types (previously TestInputErrorUnwrap, TestConfigErrorUnwrap,
// TestFileErrorUnwrap and TestErrorIsUsage, which were four overlapping copies
// of the same errors.Is check). The message-format and field-value tests stay
// per-type because the three error types have structurally different fields
// and constructors.
func TestErrorUnwrap(t *testing.T) {
	t.Parallel()

	underlying := errors.New("underlying error")

	tests := []struct {
		name     string
		err      error
		sentinel error
		wantIs   bool
	}{
		{"InputError unwraps to ErrInputTooLarge", newInputError("Extract", 10000, 5000, nil), ErrInputTooLarge, true},
		{"InputError unwraps to underlying error", newInputError("Extract", 10000, 5000, underlying), underlying, true},
		{"ConfigError unwraps to ErrInvalidConfig", newConfigError("MaxDepth", 0, "must be positive"), ErrInvalidConfig, true},
		{"FileError unwraps to underlying error", newFileError("ExtractFromFile", "../traversal", underlying), underlying, true},
		{"FileError with nil underlying unwraps to ErrInvalidFilePath", newFileError("ExtractFromFile", "../traversal", nil), ErrInvalidFilePath, true},
		{"FileError unwraps to ErrFileNotFound", newFileError("ExtractFromFile", "missing.html", ErrFileNotFound), ErrFileNotFound, true},
		// Negative cases: an error type must not match an unrelated sentinel.
		{"ConfigError does not match ErrInputTooLarge", newConfigError("Field", "value", "message"), ErrInputTooLarge, false},
		{"InputError does not match ErrInvalidConfig", newInputError("Extract", 10000, 5000, nil), ErrInvalidConfig, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := errors.Is(tt.err, tt.sentinel); got != tt.wantIs {
				t.Errorf("errors.Is(err, %v) = %v, want %v", tt.sentinel, got, tt.wantIs)
			}
		})
	}
}

func TestConfigError(t *testing.T) {
	t.Parallel()

	t.Run("error message format", func(t *testing.T) {
		err := newConfigError("MaxDepth", 0, "must be positive")
		msg := err.Error()

		if msg == "" {
			t.Error("Error message should not be empty")
		}
		if !strings.Contains(msg, "MaxDepth") {
			t.Errorf("Error message should contain field: %s", msg)
		}
		if !strings.Contains(msg, "must be positive") {
			t.Errorf("Error message should contain message: %s", msg)
		}
	})

	t.Run("field values", func(t *testing.T) {
		err := newConfigError("Field", "value", "test message")

		if err.Field != "Field" {
			t.Errorf("Field = %q, want 'Field'", err.Field)
		}
		if err.Value != "value" {
			t.Errorf("Value = %v, want 'value'", err.Value)
		}
		if err.Message != "test message" {
			t.Errorf("Message = %q, want 'test message'", err.Message)
		}
	})

	t.Run("different value types", func(t *testing.T) {
		tests := []struct {
			name  string
			field string
			value interface{}
		}{
			{"int value", "MaxDepth", 100},
			{"string value", "Encoding", "utf-8"},
			{"negative int", "MaxInputSize", -1},
			{"zero int", "WorkerPoolSize", 0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := newConfigError(tt.field, tt.value, "test")
				if err.Field != tt.field {
					t.Errorf("Field = %q, want %q", err.Field, tt.field)
				}
				if err.Value != tt.value {
					t.Errorf("Value = %v, want %v", err.Value, tt.value)
				}
			})
		}
	})
}

func TestFileError(t *testing.T) {
	t.Parallel()

	t.Run("error message format", func(t *testing.T) {
		err := newFileError("ExtractFromFile", "/path/to/file.html", errors.New("not found"))
		msg := err.Error()

		if msg == "" {
			t.Error("Error message should not be empty")
		}
		if !strings.Contains(msg, "ExtractFromFile") {
			t.Errorf("Error message should contain operation: %s", msg)
		}
		// SECURITY: Error message should NOT contain full path (information disclosure)
		// It should only contain the filename (base component)
		if strings.Contains(msg, "/path/to/") {
			t.Errorf("Error message should not contain directory path: %s", msg)
		}
		// Should contain just the filename
		if !strings.Contains(msg, "file.html") {
			t.Errorf("Error message should contain filename: %s", msg)
		}
	})

	t.Run("error message sanitizes underlying error with path", func(t *testing.T) {
		err := newFileError("Read", "/secret/path/config.yaml", errors.New("permission denied for /secret/path"))
		msg := err.Error()

		// Should not contain the path from underlying error
		if strings.Contains(msg, "/secret/path") {
			t.Errorf("Error message should not contain path from underlying error: %s", msg)
		}
		// Should still indicate the error type
		if !strings.Contains(msg, "permission denied") {
			t.Errorf("Error message should contain error type: %s", msg)
		}
	})

	t.Run("field values", func(t *testing.T) {
		underlying := errors.New("test error")
		err := newFileError("Read", "/test/path", underlying)

		if err.Op != "Read" {
			t.Errorf("Op = %q, want 'Read'", err.Op)
		}
		// Full path should still be available via the Path field for internal use
		if err.Path != "/test/path" {
			t.Errorf("Path = %q, want '/test/path'", err.Path)
		}
		if err.FileErr != underlying {
			t.Error("FileErr should be the underlying error")
		}
	})

	t.Run("SafePath returns filename only", func(t *testing.T) {
		err := newFileError("Read", "/path/to/file.html", nil)
		if err.SafePath() != "file.html" {
			t.Errorf("SafePath() = %q, want 'file.html'", err.SafePath())
		}
	})

	// Path variants: the constructor must carry the path verbatim in the Path
	// field (full path retained for internal use; only Error()/JSON redact it).
	t.Run("path field carries variants verbatim", func(t *testing.T) {
		paths := []struct {
			name string
			path string
		}{
			{"empty path", ""},
			{"relative path", "relative/path.html"},
			{"absolute path", "/absolute/path.html"},
			{"path with spaces", "/path with spaces/file.html"},
			{"windows path", `C:\Users\test\file.html`},
			{"traversal attempt", "../../../etc/passwd"},
		}
		for _, tt := range paths {
			t.Run(tt.name, func(t *testing.T) {
				err := newFileError("Test", tt.path, errors.New("test"))
				if err.Path != tt.path {
					t.Errorf("Path = %q, want %q", err.Path, tt.path)
				}
			})
		}
	})
}

// TestSentinelErrors verifies all sentinel errors exist and have meaningful messages
func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	sentinelErrors := []struct {
		name string
		err  error
		msg  string // Expected substring in error message
	}{
		{"ErrInputTooLarge", ErrInputTooLarge, "input"},
		{"ErrInvalidHTML", ErrInvalidHTML, "invalid"},
		{"ErrProcessorClosed", ErrProcessorClosed, "closed"},
		{"ErrMaxDepthExceeded", ErrMaxDepthExceeded, "depth"},
		{"ErrInvalidConfig", ErrInvalidConfig, "config"},
		{"ErrProcessingTimeout", ErrProcessingTimeout, "timeout"},
		{"ErrFileNotFound", ErrFileNotFound, "not found"},
		{"ErrInvalidFilePath", ErrInvalidFilePath, "path"},
	}

	for _, tt := range sentinelErrors {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s should have a message", tt.name)
			}
			if !strings.Contains(tt.err.Error(), tt.msg) {
				t.Errorf("%s message should contain %q, got %q", tt.name, tt.msg, tt.err.Error())
			}
		})
	}
}
func TestErrorIsUsage(t *testing.T) {
	t.Parallel()

	t.Run("errors.Is with InputError", func(t *testing.T) {
		err := newInputError("Extract", 10000, 5000, nil)
		if !errors.Is(err, ErrInputTooLarge) {
			t.Error("errors.Is should match ErrInputTooLarge")
		}
	})

	t.Run("errors.Is with ConfigError", func(t *testing.T) {
		err := newConfigError("Field", "value", "message")
		if !errors.Is(err, ErrInvalidConfig) {
			t.Error("errors.Is should match ErrInvalidConfig")
		}
	})

	t.Run("errors.Is with FileError and nil inner error", func(t *testing.T) {
		err := newFileError("Op", "path", nil)
		if !errors.Is(err, ErrInvalidFilePath) {
			t.Error("errors.Is should match ErrInvalidFilePath for nil inner error")
		}
	})

	t.Run("errors.Is with FileError unwraps to inner error", func(t *testing.T) {
		innerErr := errors.New("test")
		err := newFileError("Op", "path", innerErr)
		if !errors.Is(err, innerErr) {
			t.Error("errors.Is should match inner error")
		}
	})

	t.Run("errors.Is negative case", func(t *testing.T) {
		err := newConfigError("Field", "value", "message")
		if errors.Is(err, ErrInputTooLarge) {
			t.Error("ConfigError should not match ErrInputTooLarge")
		}
	})
}

func TestErrInternalPanic(t *testing.T) {
	t.Parallel()

	t.Run("sentinel error exists", func(t *testing.T) {
		if ErrInternalPanic == nil {
			t.Error("ErrInternalPanic should not be nil")
		}
		if ErrInternalPanic.Error() == "" {
			t.Error("ErrInternalPanic should have a message")
		}
	})

	t.Run("error message contains expected text", func(t *testing.T) {
		msg := ErrInternalPanic.Error()
		if !strings.Contains(msg, "panic") {
			t.Errorf("ErrInternalPanic message should contain 'panic': %s", msg)
		}
	})
}

// TestFileErrorMarshalJSON covers FileError.MarshalJSON (previously 0% coverage)
// and asserts the security-relevant behavior: when a FileError is serialized to
// JSON (e.g. for an API response), only the basename of Path is exposed and the
// underlying error message is sanitized of path detail.
func TestFileErrorMarshalJSON(t *testing.T) {
	t.Parallel()

	type jsonForm struct {
		Op      string `json:"op"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}

	t.Run("path is reduced to basename", func(t *testing.T) {
		err := newFileError("ExtractFromFile", "/var/secrets/../../etc/passwd", errors.New("open failed"))
		data, mErr := json.Marshal(err)
		if mErr != nil {
			t.Fatalf("MarshalJSON returned error: %v", mErr)
		}

		var got jsonForm
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("output is not valid JSON: %v\nraw: %s", err, data)
		}
		if got.Op != "ExtractFromFile" {
			t.Errorf("op = %q, want %q", got.Op, "ExtractFromFile")
		}
		// SECURITY: only the basename may be exposed; the directory chain and
		// traversal segments must never reach the serialized form.
		if got.Path != "passwd" {
			t.Errorf("path = %q, want basename %q", got.Path, "passwd")
		}
		raw := string(data)
		if strings.Contains(raw, "/var/secrets") || strings.Contains(raw, "..") {
			t.Errorf("serialized JSON leaked sensitive path components: %s", raw)
		}
	})

	t.Run("path-traversal message is sanitized", func(t *testing.T) {
		err := newFileError("ReadFile", "/safe/dir/file.txt", fmt.Errorf("path traversal detected near /root/.ssh"))
		data, _ := json.Marshal(err)
		var got jsonForm
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.Message != "path traversal detected" {
			t.Errorf("message = %q, want %q", got.Message, "path traversal detected")
		}
		if strings.Contains(got.Message, "/root") {
			t.Errorf("message leaked filesystem path: %q", got.Message)
		}
	})

	t.Run("nil underlying error yields empty message", func(t *testing.T) {
		err := newFileError("ReadFile", "file.txt", nil)
		data, _ := json.Marshal(err)
		var got jsonForm
		_ = json.Unmarshal(data, &got)
		if got.Message != "" {
			t.Errorf("message = %q, want empty when FileErr is nil", got.Message)
		}
	})

	t.Run("empty path serializes as empty", func(t *testing.T) {
		err := newFileError("ReadFile", "", errors.New("not found"))
		data, _ := json.Marshal(err)
		var got jsonForm
		_ = json.Unmarshal(data, &got)
		if got.Path != "" {
			t.Errorf("path = %q, want empty", got.Path)
		}
	})
}

// TestFileErrorEmptyPathPlaceholder covers the SafePath fallback in
// FileError.Error(): an empty path renders as "[file]" rather than empty
// quotes.
func TestFileErrorEmptyPathPlaceholder(t *testing.T) {
	t.Parallel()

	err := newFileError("ExtractFromFile", "", errors.New("boom"))
	msg := err.Error()
	if !strings.Contains(msg, `"[file]"`) {
		t.Errorf("Error() = %q, want placeholder [file] for empty path", msg)
	}
}
