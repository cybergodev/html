package html

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for the `cybergodev/html` package.
// These errors can be used with errors.Is() for type checking.
var (
	// ErrInputTooLarge is returned when input exceeds MaxInputSize.
	ErrInputTooLarge = errors.New("html: input size exceeds maximum")

	// ErrInvalidHTML is returned when HTML parsing fails.
	ErrInvalidHTML = errors.New("html: invalid HTML")

	// ErrProcessorClosed is returned when operations are attempted on a closed processor.
	ErrProcessorClosed = errors.New("html: processor closed")

	// ErrMaxDepthExceeded is returned when HTML nesting exceeds MaxDepth.
	ErrMaxDepthExceeded = errors.New("html: max depth exceeded")

	// ErrInvalidConfig is returned when configuration validation fails.
	ErrInvalidConfig = errors.New("html: invalid config")

	// ErrProcessingTimeout is returned when processing exceeds ProcessingTimeout.
	ErrProcessingTimeout = errors.New("html: processing timeout exceeded")

	// ErrFileNotFound is returned when specified file cannot be read.
	ErrFileNotFound = errors.New("html: file not found")

	// ErrInvalidFilePath is returned when file path validation fails.
	ErrInvalidFilePath = errors.New("html: invalid file path")

	// ErrInternalPanic is returned when an unexpected panic occurs during processing.
	// This error indicates an internal bug and should be reported to the library maintainers.
	ErrInternalPanic = errors.New("html: internal panic recovered")

	// ErrMultipleConfigs is returned when more than one Config is provided to a function.
	// Package-level functions like Extract accept at most one optional Config.
	ErrMultipleConfigs = errors.New("html: at most one Config may be provided")

	// ErrBatchTooLarge is returned when a batch extraction is asked to process
	// more items than the fixed maximum batch size. Split the batch and
	// resubmit. (Previously this condition was only matchable by message
	// string, inconsistent with every other batch failure mode.)
	ErrBatchTooLarge = errors.New("html: batch size exceeds maximum")

	// ErrTooManyConcurrentOps is returned when a timed extraction is rejected
	// because the cap on concurrently active timeout watchdogs is already
	// reached — a "too busy" condition distinct from ErrProcessingTimeout,
	// where nothing has timed out. Retry once in-flight operations drain.
	ErrTooManyConcurrentOps = errors.New("html: too many concurrent operations")
)

// InputError provides context for input-related errors.
// It supports errors.Is() checking against ErrInputTooLarge.
type InputError struct {
	Op       string // Operation that failed (e.g., "Extract", "ExtractFromFile")
	Size     int    // Actual input size
	MaxSize  int    // Maximum allowed size
	InputErr error  // Underlying error, if any
}

// Error returns a formatted error message.
//
// A nil *InputError yields a fixed message instead of panicking. Callers reach
// these methods through the `error` interface, and a typed-nil pointer stored
// in a non-nil interface (`var e *InputError; var err error = e`) passes the
// `err != nil` test and then crashes any code that formats it — including
// `log.Printf("%v", err)` and errors.Is. Public API must not panic (SEC-003),
// so each error type here is nil-safe.
func (e *InputError) Error() string {
	if e == nil {
		return "html: input error"
	}
	if e.InputErr != nil {
		return fmt.Sprintf("html: %s failed: %v (size=%d, max=%d)", e.Op, e.InputErr, e.Size, e.MaxSize)
	}
	return fmt.Sprintf("html: %s failed: input too large (size=%d, max=%d)", e.Op, e.Size, e.MaxSize)
}

// Unwrap returns the wrapped errors for errors.Is()/errors.As() support.
// When an underlying InputErr is present it is returned alongside
// ErrInputTooLarge, so errors.Is(err, ErrInputTooLarge) holds for every
// *InputError — including ones carrying a read failure — matching the
// type's documented contract. A nil receiver still unwraps to
// ErrInputTooLarge so the typed-nil case never panics.
func (e *InputError) Unwrap() []error {
	if e == nil || e.InputErr == nil {
		return []error{ErrInputTooLarge}
	}
	return []error{e.InputErr, ErrInputTooLarge}
}

// newInputError creates a new InputError with the provided details.
func newInputError(op string, size, maxSize int, err error) *InputError {
	return &InputError{
		Op:       op,
		Size:     size,
		MaxSize:  maxSize,
		InputErr: err,
	}
}

// ConfigError provides context for configuration errors.
// It supports errors.Is() checking against ErrInvalidConfig.
type ConfigError struct {
	Field   string // Configuration field that is invalid
	Value   any    // The invalid value
	Message string // Additional context about the error
}

// Error returns a formatted error message.
//
// A nil *ConfigError yields a fixed message instead of panicking (see
// InputError.Error for why the typed-nil case reaches public API).
func (e *ConfigError) Error() string {
	if e == nil {
		return "html: invalid config"
	}
	return fmt.Sprintf("html: invalid config: %s=%v, %s", e.Field, e.Value, e.Message)
}

// Unwrap returns ErrInvalidConfig for errors.Is() support.
func (e *ConfigError) Unwrap() error {
	return ErrInvalidConfig
}

// newConfigError creates a new ConfigError with the provided details.
func newConfigError(field string, value any, message string) *ConfigError {
	return &ConfigError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// FileError provides context for file-related errors.
// It supports errors.Is() checking against ErrFileNotFound or ErrInvalidFilePath.
type FileError struct {
	Op      string // Operation that failed (e.g., "ExtractFromFile", "ReadFile")
	Path    string // File path that caused the error
	FileErr error  // Underlying error from file system
}

// Error returns a formatted error message.
// SECURITY: Path information is truncated to prevent information disclosure
// in production environments. Use SafePath() for the full path when needed
// for internal logging or audit purposes.
//
// A nil *FileError yields a fixed message instead of panicking (see
// InputError.Error for why the typed-nil case reaches public API).
func (e *FileError) Error() string {
	if e == nil {
		return "html: file error"
	}
	// SECURITY: Truncate path in error message to prevent information disclosure
	// Only show the base filename, not the full path
	safePath := e.SafePath()
	if safePath == "" {
		safePath = "[file]"
	}
	// Sanitize underlying error message to remove any path information
	safeErr := e.sanitizeErrorMessage()
	return fmt.Sprintf("html: %s %q: %v", e.Op, safePath, safeErr)
}

// SafePath returns the path with potentially sensitive information removed.
// For production use, this truncates the path to just the filename.
// The full path is still available via the Path field for internal use.
func (e *FileError) SafePath() string {
	if e == nil {
		return ""
	}
	return safeBaseName(e.Path)
}

// safeBaseName returns the portion of path after the last path separator
// ('/' or '\\'), or path itself when it contains no separator. It backs both
// FileError.SafePath and the audit subsystem's AuditEntry.Path sanitization,
// keeping filesystem layouts out of error messages and audit logs while the
// full raw value remains available through explicitly opt-in channels.
func safeBaseName(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

// sanitizeErrorMessage removes potentially sensitive path information from
// the underlying error message while preserving error type information.
func (e *FileError) sanitizeErrorMessage() error {
	if e == nil || e.FileErr == nil {
		return nil
	}
	errStr := e.FileErr.Error()

	// SECURITY: Preserve error type information while removing path details.
	// This ensures that error messages still indicate the nature of the error
	// (e.g., "path traversal detected", "not found") without exposing
	// sensitive filesystem paths.

	// Check for known error patterns and return sanitized versions. The
	// Windows os package phrases itself differently ("The system cannot find
	// the file specified", "Access is denied", "… is a directory"), so match
	// those wordings too — they previously fell through to the generic
	// "file operation failed" below.
	lowerErr := strings.ToLower(errStr)
	if strings.Contains(lowerErr, "path traversal") {
		return fmt.Errorf("path traversal detected")
	}
	if strings.Contains(lowerErr, "not found") ||
		strings.Contains(lowerErr, "cannot find the file") ||
		strings.Contains(lowerErr, "no such file") {
		return fmt.Errorf("file not found")
	}
	if strings.Contains(lowerErr, "permission denied") ||
		strings.Contains(lowerErr, "access is denied") ||
		strings.Contains(lowerErr, "access denied") {
		return fmt.Errorf("permission denied")
	}
	if strings.Contains(lowerErr, "is a directory") {
		return fmt.Errorf("is a directory")
	}

	// Check if the error message contains what looks like a file path
	// and sanitize it to prevent information disclosure
	if strings.Contains(errStr, "/") || strings.Contains(errStr, "\\") {
		return fmt.Errorf("file operation failed")
	}

	return e.FileErr
}

// Unwrap returns the appropriate sentinel error for errors.Is() support.
// A nil receiver unwraps to ErrInvalidFilePath rather than panicking.
func (e *FileError) Unwrap() error {
	if e == nil {
		return ErrInvalidFilePath
	}
	if errors.Is(e.FileErr, ErrFileNotFound) {
		return ErrFileNotFound
	}
	if e.FileErr != nil {
		return e.FileErr
	}
	return ErrInvalidFilePath
}

// MarshalJSON implements custom JSON marshaling for FileError.
// The Path field is sanitized via SafePath() to prevent filesystem path
// disclosure when errors are serialized to JSON for API responses.
//
// A nil receiver marshals to null rather than panicking, matching
// encoding/json's handling of nil pointers.
func (e *FileError) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	msg := ""
	if sanitized := e.sanitizeErrorMessage(); sanitized != nil {
		msg = sanitized.Error()
	}
	return json.Marshal(&struct {
		Op      string `json:"op"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Op:      e.Op,
		Path:    e.SafePath(),
		Message: msg,
	})
}

// newFileError creates a new FileError with the provided details.
func newFileError(op, path string, err error) *FileError {
	return &FileError{
		Op:      op,
		Path:    path,
		FileErr: err,
	}
}
