// Package table provides HTML table extraction and rendering functionality.
package table

import (
	"strings"
)

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

// TrackedBuilder is a strings.Builder that remembers the last byte written, so
// callers can inspect the trailing byte without re-reading the buffer.
//
// LastChar holds that trailing byte. It is a byte, not a rune: for multi-byte
// UTF-8 input it is the final byte of the encoding (a leading or continuation
// byte), which is meaningful only for the ASCII single-byte comparisons the
// callers rely on — newline/space detection in EnsureNewline/EnsureSpacing and
// tests asserting the last byte of ASCII output. Code that writes non-ASCII
// content should not interpret LastChar as a character.
type TrackedBuilder struct {
	*strings.Builder
	LastChar byte
}

// NewTrackedBuilder creates a new TrackedBuilder wrapping the provided strings.Builder.
func NewTrackedBuilder(sb *strings.Builder) *TrackedBuilder {
	return &TrackedBuilder{
		Builder:  sb,
		LastChar: 0,
	}
}

// WriteByte writes a single byte and records it in LastChar.
func (tb *TrackedBuilder) WriteByte(c byte) error {
	tb.LastChar = c
	return tb.Builder.WriteByte(c)
}

// WriteString writes a string and, when at least one byte was written, records
// the final byte of s in LastChar.
func (tb *TrackedBuilder) WriteString(s string) (int, error) {
	n, err := tb.Builder.WriteString(s)
	if n > 0 && err == nil {
		tb.LastChar = s[len(s)-1]
	}
	return n, err
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
