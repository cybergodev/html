package table

// width_escape_regression_test.go — regression for the D-002 re-check round:
// calculateMaxColumnWidths now counts the escape sequences
// writeMarkdownCellText emits, so a cell containing '|' or '\' no longer
// renders one character wider than its padded column.

import "testing"

func TestColumnWidthCountsEscapes(t *testing.T) {
	t.Parallel()

	td := [][]CellData{
		{{Text: "a|b"}, {Text: "x"}},
		{{Text: "ccc"}, {Text: "yy"}},
	}
	w := calculateMaxColumnWidths(td, 2)

	// "a|b" has 3 runes but renders as "a\|b" (4 chars), so the column pads
	// to 4 and the plain "ccc" row aligns with it.
	if w[0] != 4 {
		t.Errorf("column width = %d, want 4 (rune count plus pipe escape)", w[0])
	}
	if w[1] != 2 {
		t.Errorf("plain column width = %d, want 2", w[1])
	}
}
