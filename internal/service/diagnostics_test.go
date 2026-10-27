package service

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestDiagnosticsWindowAndCaptureLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewDiagnosticsService()
		if s.Status().Enabled {
			t.Fatal("diagnostics must start disabled")
		}
		if _, _, err := s.Begin(t.Context()); !errors.Is(err, ErrDiagnosticsDisabled) {
			t.Fatalf("disabled capture: %v", err)
		}
		status := s.Enable()
		if !status.Enabled || status.ExpiresAt.Sub(time.Now()) != DiagnosticsWindow {
			t.Fatalf("wrong window: %+v", status)
		}
		time.Sleep(time.Minute)
		if !s.Enable().ExpiresAt.Equal(*status.ExpiresAt) {
			t.Fatal("enabling an open window must not silently extend it")
		}
		ctx, release, err := s.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Begin(t.Context()); !errors.Is(err, ErrDiagnosticsBusy) {
			t.Fatalf("concurrent capture: %v", err)
		}
		time.Sleep(14 * time.Minute)
		synctest.Wait()
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("expiry did not cancel active capture: %v", ctx.Err())
		}
		if s.Status().Enabled {
			t.Fatal("diagnostics still enabled at deadline")
		}
		if _, _, err := s.Begin(t.Context()); !errors.Is(err, ErrDiagnosticsDisabled) {
			t.Fatalf("expired capture: %v", err)
		}
		release()
		release() // release is safe on cleanup paths that run more than once
		if s.Status().Capturing {
			t.Fatal("capture lease leaked")
		}
	})
}

func TestDiagnosticsDisableAndDisconnectCancelCapture(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		s := NewDiagnosticsService()
		s.Enable()
		parent, cancel := context.WithCancel(t.Context())
		ctx, release, err := s.Begin(parent)
		if err != nil {
			t.Fatal(err)
		}
		if disconnect {
			cancel()
		} else {
			s.Disable()
		}
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("capture was not canceled: %v", ctx.Err())
		}
		if !s.Status().Capturing {
			t.Fatal("must hold lease until handler finishes stopping the profiler")
		}
		release()
		cancel()
		if s.Status().Capturing {
			t.Fatal("capture still busy after release")
		}
	}
	// A new process/container always starts disabled.
	if NewDiagnosticsService().Status().Enabled {
		t.Fatal("diagnostic state was persisted")
	}
}
