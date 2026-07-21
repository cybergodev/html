// Package table provides HTML table extraction and rendering functionality.
package table

// CellAlignment represents the text alignment of a table cell.
type CellAlignment int

const (
	// AlignLeft indicates left text alignment.
	AlignLeft CellAlignment = iota
	// AlignCenter indicates center text alignment.
	AlignCenter
	// AlignRight indicates right text alignment.
	AlignRight
	// AlignJustify indicates justified text alignment.
	AlignJustify
	// AlignDefault indicates default (unspecified) alignment.
	AlignDefault
)

// CellData contains cell metadata for table extraction.
type CellData struct {
	// Text is the cell's text content.
	Text string
	// Align is the cell's text alignment.
	Align CellAlignment
	// Colspan is the number of columns this cell spans.
	Colspan int
	// Rowspan is the number of rows this cell spans.
	Rowspan int
	// IsHeader indicates if this cell is a header cell (th).
	IsHeader bool
	// Width is the cell's width specification (e.g., "100px", "1.0%", "auto").
	Width string
	// IsExpanded indicates if this cell was created from colspan expansion.
	IsExpanded bool
	// OriginalColspan is the original colspan value before expansion (for HTML output).
	OriginalColspan int
}

// AlignCount tracks the number of cells with each alignment type for a column.
type AlignCount struct {
	Left, Center, Right, Justify int
}

// TrackedBuilder is a capacity-retaining byte buffer that remembers the last
// byte written, so callers can inspect the trailing byte without re-reading the
// buffer.
//
// It is backed by a []byte rather than a *strings.Builder so that Reset
// (buf[:0]) RETAINS the backing capacity across uses. A pooled TrackedBuilder
// therefore does not re-grow from zero on every call — unlike *strings.Builder,
// whose Reset() nils its buffer (see internal/pool.go's BuilderPool notes). The
// text-extraction hot path builds a document-length string per Extract() call, so
// retaining the buffer removes the per-call growth allocations and the GC churn
// they cause. Callers that want a retained instance should obtain one via
// internal.GetTrackedBuilder/PutTrackedBuilder rather than constructing directly.
//
// LastChar holds the trailing byte written. It is a byte, not a rune: for
// multi-byte UTF-8 input it is the final byte of the encoding (a leading or
// continuation byte), meaningful only for the ASCII single-byte comparisons the
// callers rely on — newline/space detection in EnsureNewline/EnsureSpacing and
// tests asserting the last byte of ASCII output. Code that writes non-ASCII
// content should not interpret LastChar as a character.
type TrackedBuilder struct {
	buf      []byte
	LastChar byte
}

// NewTrackedBuilder returns a ready-to-use TrackedBuilder. Callers that want a
// pooled, capacity-retaining instance should use internal.GetTrackedBuilder /
// PutTrackedBuilder instead of constructing directly.
func NewTrackedBuilder() *TrackedBuilder {
	return &TrackedBuilder{}
}

// Reset clears the written bytes while retaining the backing capacity for reuse.
func (tb *TrackedBuilder) Reset() {
	tb.buf = tb.buf[:0]
	tb.LastChar = 0
}

// Len returns the number of bytes written so far.
func (tb *TrackedBuilder) Len() int { return len(tb.buf) }

// Cap returns the capacity of the backing buffer.
func (tb *TrackedBuilder) Cap() int { return cap(tb.buf) }

// Bytes returns the bytes written so far. The returned slice aliases the buffer's
// backing array and is invalidated by subsequent writes or a Reset.
func (tb *TrackedBuilder) Bytes() []byte { return tb.buf }

// String returns the bytes written so far as a string. This copies the buffer
// (the single unavoidable allocation on a retaining buffer); the buffer itself is
// retained for reuse.
func (tb *TrackedBuilder) String() string { return string(tb.buf) }

// Grow reserves at least n more bytes of capacity. Mirrors strings.Builder.Grow
// so callers can pre-size when the eventual length is known, though a pooled
// instance typically already retains sufficient capacity.
func (tb *TrackedBuilder) Grow(n int) {
	if n < 0 {
		panic("table.TrackedBuilder.Grow: negative count")
	}
	if cap(tb.buf)-len(tb.buf) >= n {
		return
	}
	// Double until enough, like strings.Builder, but honor n as a floor.
	newCap := cap(tb.buf)
	if newCap < 64 {
		newCap = 64
	}
	for newCap < len(tb.buf)+n {
		newCap *= 2
	}
	buf := make([]byte, len(tb.buf), newCap)
	copy(buf, tb.buf)
	tb.buf = buf
}

// WriteByte appends a single byte and records it in LastChar.
func (tb *TrackedBuilder) WriteByte(c byte) error {
	tb.buf = append(tb.buf, c)
	tb.LastChar = c
	return nil
}

// Write appends p and, when non-empty, records its final byte in LastChar.
func (tb *TrackedBuilder) Write(p []byte) (int, error) {
	tb.buf = append(tb.buf, p...)
	if len(p) > 0 {
		tb.LastChar = p[len(p)-1]
	}
	return len(p), nil
}

// WriteString appends s and, when non-empty, records its final byte in LastChar.
func (tb *TrackedBuilder) WriteString(s string) (int, error) {
	tb.buf = append(tb.buf, s...)
	if len(s) > 0 {
		tb.LastChar = s[len(s)-1]
	}
	return len(s), nil
}

// EnsureNewline ensures the builder ends with a newline.
func EnsureNewline(tb *TrackedBuilder) {
	if tb.Len() > 0 && tb.LastChar != '\n' {
		_ = tb.WriteByte('\n')
	}
}

// EnsureSpacing ensures the builder ends with the specified character if not already ending with space or newline.
func EnsureSpacing(tb *TrackedBuilder, char byte) {
	if tb.Len() > 0 && tb.LastChar != ' ' && tb.LastChar != '\n' {
		_ = tb.WriteByte(char)
	}
}
