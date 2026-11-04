package printer

import (
	"context"
	"testing"
	"time"

	"aurink/internal/contract"
)

func TestPrintTimeoutAndCancellationCloseWindow(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := make(chan time.Time)
			entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
			defer close(release)
			done := make(chan error, 1)
			go func() {
				data, err := waitPrint(ctx, timeout, func() ([]byte, error) {
					close(entered)
					<-release
					return []byte("late PDF"), nil
				}, func() { close(closed) })
				if data != nil {
					done <- contract.E(contract.CodeInternal, "test", nil)
					return
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("print did not start")
			}
			want := contract.CodeTimeout
			if mode == "cancel" {
				want = contract.CodeCanceled
				cancel()
			} else {
				close(timeout)
			}
			select {
			case err := <-done:
				if contract.CodeOf(err) != want {
					t.Fatalf("got %v, want %s", err, want)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("print wait did not end")
			}
			select {
			case <-closed:
			default:
				t.Fatal("window was not closed before returning")
			}
		})
	}
}

func TestRejectInvalidMargins(t *testing.T) {
	for i := range 4 {
		var margins [4]float32
		margins[i] = -.1
		data, err := New().HTMLToPDF(context.Background(), "", "", contract.PDFOptions{PageSize: "Letter", Margins: margins})
		if data != nil || contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatalf("margins %v: data=%q error=%v", margins, data, err)
		}
	}
}
