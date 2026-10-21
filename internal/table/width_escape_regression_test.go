package table

// width_escape_regression_test.go — regression for the D-002 re-check round:
// calculateMaxColumnWidths now counts the escape sequences
// writeMarkdownCellText emits, so a cell containing '|' or '\' no longer
// renders one character wider than its padded column.

import (
	"strings"
	"testing"
)

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

// TestMarkdownRowsAlignWithEscapedCells is the end-to-end counterpart: the
// renderer's per-cell padding must measure the same width
// calculateMaxColumnWidths used. It previously counted runes only, so a cell
// containing '|' or '\' rendered wider than its column — the header row and
// the escaped data row came out different lengths.
func TestMarkdownRowsAlignWithEscapedCells(t *testing.T) {
	t.Parallel()

	td := [][]CellData{
		{{Text: "head"}, {Text: "x"}},
		{{Text: "a|b"}, {Text: "y"}},
		{{Text: "abcd"}, {Text: "z"}},
		{{Text: `back\slash`}, {Text: "w"}},
	}

	var tb TrackedBuilder
	extractTableAsMarkdown(td, &tb, 2)

	var widths []int
	for _, line := range strings.Split(strings.TrimRight(tb.String(), "\n"), "\n") {
		// The separator row carries fixed-width alignment markers and is not
		// padded to the column width by design.
		if strings.Contains(line, "---") {
			continue
		}
		widths = append(widths, len(line))
	}
	if len(widths) < 2 {
		t.Fatalf("unexpected output: %q", tb.String())
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[0] {
			t.Fatalf("row %d rendered %d bytes wide, header row %d — cells containing a pipe or backslash knock the row out of alignment:\n%s",
				i, widths[i], widths[0], tb.String())
		}
	}
}
