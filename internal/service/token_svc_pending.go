package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

const loginRefreshTokenStoreTimeout = 750 * time.Millisecond

func (s *TokenService) storeRefreshTokenBestEffort(token model.RefreshToken) {
	if !s.trackDelayedStore(token) {
		return
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := helper.Recover(s.log, "token.storeRefreshToken", func() error {
			row := token
			return s.storeRefreshToken(ctx, &row)
		})
		done <- err
	}()
	select {
	case err := <-done:
		s.finishBestEffortRefreshTokenStore(token, err)
	case <-time.After(loginRefreshTokenStoreTimeout):
		if s.log != nil {
			s.log.Warn("refresh token store delayed; login will continue",
				zap.String("user_id", token.UserID),
				zap.Error(context.DeadlineExceeded))
		}
		go func() {
			err := <-done
			s.finishBestEffortRefreshTokenStore(token, err)
		}()
	}
}

func (s *TokenService) finishBestEffortRefreshTokenStore(token model.RefreshToken, err error) {
	if err == nil {
		s.untrackDelayedStore(token.TokenHash)
		return
	}
	if repository.IsTransientDatabaseError(err) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		if s.log != nil {
			s.log.Warn("refresh token store delayed; login will continue",
				zap.String("user_id", token.UserID),
				zap.Error(err))
		}
		s.storeRefreshTokenEventually(token)
		return
	}
	s.untrackDelayedStore(token.TokenHash)
	if s.log != nil {
		s.log.Warn("refresh token delayed store failed permanently", zap.String("user_id", token.UserID), zap.Error(err))
	}
}

func (s *TokenService) storeRefreshTokenEventually(token model.RefreshToken) {
	defer s.untrackDelayedStore(token.TokenHash)
	delay := time.Second
	for attempt := 1; attempt <= 8; attempt++ {
		timer := time.NewTimer(delay)
		<-timer.C
		// 令牌可能已在等待期间被轮换/登出（从 pending 表移除），
		// 此时绝不能再写库，否则会复活一个已被替换的旧令牌。
		if _, stillPending := s.pendingDelayedStore(token.TokenHash); !stillPending {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		row := token
		err := s.storeRefreshToken(ctx, &row)
		cancel()
		if err == nil {
			return
		}
		if !repository.IsTransientDatabaseError(err) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			if s.log != nil {
				s.log.Warn("refresh token delayed store failed permanently", zap.String("user_id", token.UserID), zap.Error(err))
			}
			return
		}
		if s.log != nil && (attempt == 1 || attempt == 4 || attempt == 8) {
			s.log.Warn("refresh token delayed store still waiting",
				zap.String("user_id", token.UserID),
				zap.Int("attempt", attempt),
				zap.Error(err))
		}
		if delay < 60*time.Second {
			delay *= 2
		}
	}
	if s.log != nil {
		s.log.Warn("refresh token delayed store gave up", zap.String("user_id", token.UserID))
	}
}

func (s *TokenService) trackDelayedStore(token model.RefreshToken) bool {
	if s == nil {
		return false
	}
	s.delayedStoreMu.Lock()
	defer s.delayedStoreMu.Unlock()
	if s.delayedStores == nil {
		s.delayedStores = make(map[string]model.RefreshToken)
	}
	if _, ok := s.delayedStores[token.TokenHash]; ok {
		return false
	}
	s.delayedStores[token.TokenHash] = token
	return true
}

func (s *TokenService) untrackDelayedStore(tokenHash string) {
	if s == nil {
		return
	}
	s.delayedStoreMu.Lock()
	delete(s.delayedStores, tokenHash)
	s.delayedStoreMu.Unlock()
}

// pendingDelayedStore 返回尚未落库的 refresh token 信息（如果存在）。
func (s *TokenService) pendingDelayedStore(tokenHash string) (model.RefreshToken, bool) {
	if s == nil {
		return model.RefreshToken{}, false
	}
	s.delayedStoreMu.Lock()
	defer s.delayedStoreMu.Unlock()
	pending, ok := s.delayedStores[tokenHash]
	return pending, ok
}
