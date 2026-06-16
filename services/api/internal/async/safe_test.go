package async

import (
	"testing"
	"time"
)

func TestSafeGoRunsFunction(t *testing.T) {
	done := make(chan struct{})

	SafeGo("test.run", func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("SafeGo did not run function before timeout")
	}
}

func TestSafeGoRecoversPanic(t *testing.T) {
	done := make(chan struct{})

	SafeGo("test.panic", func() {
		close(done)
		panic("boom")
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("SafeGo did not start function before timeout")
	}
}
