package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

const DiagnosticsWindow = 15 * time.Minute

var (
	ErrDiagnosticsDisabled = errors.New("diagnostics disabled")
	ErrDiagnosticsBusy     = errors.New("diagnostics capture already running")
)

type DiagnosticsStatus struct {
	Enabled   bool       `json:"enabled"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Capturing bool       `json:"capturing"`
}

// DiagnosticsService keeps a process-local opt-in window and one capture lease
// shared by the HTTP and HTTPS routers. No settings or profiles are persisted.
// The zero value is disabled and ready to use.
type DiagnosticsService struct {
	mu           sync.Mutex
	expiresAt    time.Time
	capturing    bool
	activeCancel context.CancelFunc
}

func NewDiagnosticsService() *DiagnosticsService { return &DiagnosticsService{} }

func (s *DiagnosticsService) Status() DiagnosticsStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *DiagnosticsService) statusLocked() DiagnosticsStatus {
	if !s.expiresAt.IsZero() && !time.Now().Before(s.expiresAt) {
		s.disableLocked()
	}
	status := DiagnosticsStatus{Capturing: s.capturing}
	if !s.expiresAt.IsZero() {
		expires := s.expiresAt
		status.Enabled = true
		status.ExpiresAt = &expires
	}
	return status
}

func (s *DiagnosticsService) Enable() DiagnosticsStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.statusLocked().Enabled {
		s.expiresAt = time.Now().UTC().Add(DiagnosticsWindow)
	}
	return s.statusLocked()
}

func (s *DiagnosticsService) Disable() DiagnosticsStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableLocked()
	return s.statusLocked()
}

func (s *DiagnosticsService) disableLocked() {
	s.expiresAt = time.Time{}
	if s.activeCancel != nil {
		s.activeCancel()
	}
}

// Begin rejects new work after expiry. Its context also cancels a running CPU
// capture at expiry, on Disable, or when the requesting client disconnects.
// Release must run after the profile handler has stopped using the runtime.
func (s *DiagnosticsService) Begin(parent context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.statusLocked().Enabled {
		return nil, nil, ErrDiagnosticsDisabled
	}
	if s.capturing {
		return nil, nil, ErrDiagnosticsBusy
	}
	ctx, cancel := context.WithDeadline(parent, s.expiresAt)
	s.capturing = true
	s.activeCancel = cancel
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			s.mu.Lock()
			defer s.mu.Unlock()
			s.activeCancel = nil
			s.capturing = false
		})
	}
	return ctx, release, nil
}
