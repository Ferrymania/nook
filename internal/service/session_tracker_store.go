package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
)

func (s *SessionTrackerService) upsert(ctx context.Context, in realtimeSessionInput) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return
	}
	now := s.now()
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.DeviceName = strings.TrimSpace(in.DeviceName)
	in.Client = strings.TrimSpace(in.Client)
	in.RemoteEndPoint = strings.TrimSpace(in.RemoteEndPoint)
	if in.DeviceID == "" {
		in.DeviceID = fallbackSessionDeviceID(in.DeviceName, in.Client, in.RemoteEndPoint)
	}
	key := userID + "\x00" + realtimeSessionTerminalKey(in.DeviceID, in.DeviceName, in.Client, in.RemoteEndPoint)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	if s.activity == nil {
		s.activity = make(map[string]time.Time)
	}
	existing, existed := s.sessions[key]
	if in.PlaybackUpdate && in.PlaybackSequence > 0 && in.PlaybackSequence <= existing.PlaybackSequence {
		return // Delayed web progress/stop requests must not replace a newer report.
	}
	s.activity[userID] = now
	if strings.TrimSpace(in.UserName) == "" {
		in.UserName = existing.UserName
	}
	if in.DeviceName == "" {
		in.DeviceName = existing.DeviceName
	}
	if in.Client == "" {
		in.Client = existing.Client
	}
	if in.RemoteEndPoint == "" {
		in.RemoteEndPoint = existing.RemoteEndPoint
	}
	lastPlaybackAt := existing.LastPlaybackAt
	itemID := existing.ItemID
	positionTicks := existing.PositionTicks
	runtimeTicks := existing.RuntimeTicks
	isPlaying := existing.IsPlaying
	isPaused := existing.IsPaused
	sequence := existing.PlaybackSequence
	if in.PlaybackUpdate {
		itemID = firstNonEmptyString(in.ItemID, existing.ItemID)
		if in.ItemID != "" || in.PositionTicks != 0 {
			positionTicks = in.PositionTicks
		}
		if in.ItemID != "" || in.RuntimeTicks != 0 {
			runtimeTicks = in.RuntimeTicks
		}
		isPlaying = in.IsPlaying
		isPaused = in.IsPaused
		sequence = in.PlaybackSequence
	}
	if in.PlaybackUpdate && (in.ItemID != "" || in.IsPlaying) {
		t := now
		lastPlaybackAt = &t
	}
	s.sessions[key] = RealtimeSession{
		ID:               key,
		UserID:           userID,
		UserName:         strings.TrimSpace(in.UserName),
		DeviceID:         in.DeviceID,
		DeviceName:       in.DeviceName,
		Client:           in.Client,
		RemoteEndPoint:   in.RemoteEndPoint,
		LastActivityAt:   now,
		ItemID:           itemID,
		PositionTicks:    positionTicks,
		RuntimeTicks:     runtimeTicks,
		IsPlaying:        isPlaying,
		IsPaused:         isPaused,
		LastPlaybackAt:   lastPlaybackAt,
		PlaybackSequence: sequence,
	}
	if !existed && s.log != nil {
		s.log.Debug("realtime session started",
			zap.String("user_id", userID),
			zap.String("device_id", in.DeviceID),
			zap.String("client", in.Client),
			zap.String("remote", in.RemoteEndPoint),
		)
	}
}

func (s *SessionTrackerService) pruneLocked(now time.Time) {
	expiresBefore := now.Add(-realtimeSessionTTL)
	for key, sess := range s.sessions {
		if sess.LastActivityAt.Before(expiresBefore) {
			delete(s.sessions, key)
			continue
		}
		if sess.IsPlaying && (sess.LastPlaybackAt == nil || !sess.LastPlaybackAt.After(now.Add(-realtimePlaybackTTL))) {
			sess.IsPlaying = false
			sess.IsPaused = false
			s.sessions[key] = sess
		}
	}
	for userID, lastActivity := range s.activity {
		if lastActivity.Before(expiresBefore) {
			delete(s.activity, userID)
		}
	}
}
