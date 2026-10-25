package html

import (
	"context"
	"errors"
	"testing"
)

// TestUniformErrorBatch covers the unexported uniformErrorBatch helper directly.
// It is the only constructor path for an all-items-failed BatchResult, so the
// index-correspondence and counter invariants are worth pinning down at the unit
// level (batch_test.go is an external test package and cannot reach it).
func TestUniformErrorBatch(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("batch setup failed")

	tests := []struct {
		name string
		n    int
		err  error
	}{
		{"zero items", 0, sentinel},
		{"single item", 1, sentinel},
		{"three items", 3, sentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			br := uniformErrorBatch(tt.n, tt.err)
			if br == nil {
				t.Fatal("uniformErrorBatch returned nil")
			}
			if len(br.Results) != tt.n {
				t.Errorf("len(Results) = %d, want %d", len(br.Results), tt.n)
			}
			if len(br.Errors) != tt.n {
				t.Errorf("len(Errors) = %d, want %d", len(br.Errors), tt.n)
			}
			if br.Failed != tt.n {
				t.Errorf("Failed = %d, want %d", br.Failed, tt.n)
			}
			if br.Success != 0 {
				t.Errorf("Success = %d, want 0", br.Success)
			}
			if br.Cancelled != 0 {
				t.Errorf("Cancelled = %d, want 0", br.Cancelled)
			}
			for i, e := range br.Errors {
				if !errors.Is(e, tt.err) {
					t.Errorf("Errors[%d] = %v, want %v", i, e, tt.err)
				}
			}
		})
	}
}

// TestRunBatchWithContextWorkerPanic drives the per-worker recover branch of
// runBatchWithContext directly: a panicking extractor must surface as
// ErrInternalPanic for its item only, while the sibling item still succeeds.
func TestRunBatchWithContextWorkerPanic(t *testing.T) {
	t.Parallel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	ok := func() (*Result, error) { return &Result{Text: "fine"}, nil }
	boom := func() (*Result, error) { panic("worker exploded") }

	br := p.runBatchWithContext(context.Background(), []extractFunc{ok, boom, ok})
	if br.Success != 2 || br.Failed != 1 || br.Cancelled != 0 {
		t.Fatalf("Success=%d Failed=%d Cancelled=%d, want 2/1/0", br.Success, br.Failed, br.Cancelled)
	}
	if !errors.Is(br.Errors[1], ErrInternalPanic) {
		t.Errorf("Errors[1] = %v, want ErrInternalPanic identity", br.Errors[1])
	}
	if br.Errors[0] != nil || br.Errors[2] != nil {
		t.Errorf("sibling items received errors from the panicking worker: %v / %v", br.Errors[0], br.Errors[2])
	}
	if br.Results[0] == nil || br.Results[0].Text != "fine" {
		t.Errorf("sibling results lost: %+v", br.Results)
	}
}

// TestRunBatchWithContextCallerPanicBackstop covers the caller-goroutine
// recover: a nil Processor panics in the preamble (p.config dereference), and
// runBatchWithContext must fold it into a uniform ErrInternalPanic batch
// rather than letting the panic escape.
func TestRunBatchWithContextCallerPanicBackstop(t *testing.T) {
	t.Parallel()

	var p *Processor
	br := p.runBatchWithContext(context.Background(), []extractFunc{
		func() (*Result, error) { return nil, nil },
	})
	if br == nil {
		t.Fatal("runBatchWithContext returned nil batch")
	}
	if br.Failed != 1 || br.Success != 0 {
		t.Fatalf("Failed=%d Success=%d, want 1/0", br.Failed, br.Success)
	}
	if !errors.Is(br.Errors[0], ErrInternalPanic) {
		t.Errorf("Errors[0] = %v, want ErrInternalPanic identity", br.Errors[0])
	}
}
