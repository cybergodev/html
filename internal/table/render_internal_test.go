package table

// render_internal_test.go — in-package tests for unexported render helpers
// that the external table_test package cannot reach.

import "testing"

// TestSpannedFill pins spannedFill's empty-text substitution: an empty cell
// body becomes a single space so the rendered cell keeps its column, while
// non-empty text passes through unchanged.
func TestSpannedFill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty text becomes single space", "", " "},
		{"non-empty text unchanged", "value", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := spannedFill(tt.in)
			if got.Text != tt.want {
				t.Errorf("spannedFill(%q).Text = %q, want %q", tt.in, got.Text, tt.want)
			}
			if got.Align != AlignDefault || got.Colspan != 1 || got.Rowspan != 1 {
				t.Errorf("spannedFill(%q) = %+v, want default alignment and 1x1 span", tt.in, got)
			}
		})
	}
}
