package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestSeedAdminRequiresChangingDefaultPassword(t *testing.T) {
	t.Setenv("ADMIN_INITIAL_PASSWORD", "")
	repos, auth, _, _ := newAuthTestServices(t)
	ctx := context.Background()
	if err := auth.SeedAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	u, err := repos.User.FirstAdmin(ctx)
	if err != nil || u == nil || !u.ForcePasswordReset {
		t.Fatalf("seeded admin: %#v %v", u, err)
	}

	// Simulate an old installation that cleared the flag without changing the
	// default password, then renamed its built-in administrator.
	if err := repos.User.UpdateFields(ctx, u.ID, map[string]any{"username": "renamed-admin", "force_password_reset": false}); err != nil {
		t.Fatal(err)
	}
	if err := auth.SeedAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := repos.User.FindByID(ctx, u.ID)
	if err != nil || !next.ForcePasswordReset || next.PasswordHash != u.PasswordHash || next.Username != "renamed-admin" {
		t.Fatalf("existing admin: %#v %v", next, err)
	}
}

func TestSeedAdminPreservesCustomPassword(t *testing.T) {
	t.Setenv("ADMIN_INITIAL_PASSWORD", "custom-password")
	repos, auth, _, _ := newAuthTestServices(t)
	if err := auth.SeedAdmin(t.Context()); err != nil {
		t.Fatal(err)
	}
	before, _ := repos.User.FirstAdmin(t.Context())
	t.Setenv("ADMIN_INITIAL_PASSWORD", "admin123")
	if err := auth.SeedAdmin(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, err := repos.User.FirstAdmin(t.Context())
	if err != nil || after.ForcePasswordReset || before.PasswordHash != after.PasswordHash {
		t.Fatalf("custom password changed: %#v %v", after, err)
	}
}

func TestInitialPasswordResetLoginAndCompletion(t *testing.T) {
	t.Setenv("ADMIN_INITIAL_PASSWORD", "")
	repos, auth, _, _ := newAuthTestServices(t)
	ctx := t.Context()
	if err := auth.SeedAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	u, _ := repos.User.FirstAdmin(ctx)
	// A refresh credential from before this upgrade must not lift the flag.
	oldPair, err := auth.tokenSvc.IssuePair(ctx, u.ID, u.Role, u.Tier)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RefreshTokens(ctx, oldPair.RefreshToken); !errors.Is(err, ErrPasswordResetRequired) {
		t.Fatalf("refresh: %v", err)
	}
	resp, err := auth.Login(ctx, "admin", "admin123")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.tokenSvc.ValidateAccessToken(resp.Tokens.AccessToken)
	if err != nil || claims.Purpose != PasswordResetTokenPurpose || resp.Tokens.RefreshToken != "" {
		t.Fatalf("reset login issued full credentials: %#v %v", claims, err)
	}
	if _, _, err := auth.CreateTemporaryPassword(ctx, u.ID); !errors.Is(err, ErrPasswordResetRequired) {
		t.Fatalf("OTP: %v", err)
	}
	if _, err := auth.IssueEmbyToken(u); !errors.Is(err, ErrPasswordResetRequired) {
		t.Fatalf("Emby token: %v", err)
	}
	if _, err := auth.IssueExternalPlaybackToken(u, "movie", 120); !errors.Is(err, ErrPasswordResetRequired) {
		t.Fatalf("external token: %v", err)
	}
	for _, input := range []struct{ old, next string }{{"wrong", "valid-password"}, {"admin123", "admin123"}, {"admin123", "short"}} {
		if err := auth.ChangePassword(ctx, u.ID, input.old, input.next); err == nil {
			t.Fatalf("accepted password change: %#v", input)
		}
		current, _ := repos.User.FindByID(ctx, u.ID)
		if !current.ForcePasswordReset || current.PasswordHash != u.PasswordHash {
			t.Fatal("failed change lifted restriction")
		}
	}
	if err := auth.ChangePassword(ctx, u.ID, "admin123", "valid-password"); err != nil {
		t.Fatal(err)
	}
	u, _ = repos.User.FindByID(ctx, u.ID)
	if u.ForcePasswordReset {
		t.Fatal("successful change kept flag")
	}
	if _, err := auth.RefreshTokens(ctx, oldPair.RefreshToken); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("old refresh survived: %v", err)
	}
	if _, err := auth.Login(ctx, "admin", "admin123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("default password survived: %v", err)
	}
	resp, err = auth.Login(ctx, "admin", "valid-password")
	if err != nil || resp.Tokens.RefreshToken == "" {
		t.Fatalf("normal login: %#v %v", resp, err)
	}
	if err := auth.ChangePassword(ctx, u.ID, "valid-password", "valid-password"); err == nil {
		t.Fatal("accepted unchanged password")
	}
	if err := auth.ResetPassword(ctx, u.ID, DefaultAdminPassword); err == nil {
		t.Fatal("administrator reset restored default password")
	}
}

func TestFirstRegisteredAdminWithDefaultPasswordRequiresReset(t *testing.T) {
	_, auth, _, _ := newAuthTestServices(t)
	u, pair, err := auth.Register(t.Context(), "owner", DefaultAdminPassword)
	if err != nil || u.Role != "admin" || !u.ForcePasswordReset || pair.RefreshToken != "" {
		t.Fatalf("registration bypass: %#v %#v %v", u, pair, err)
	}
}

func TestTemporaryPasswordCannotBypassNewResetRequirement(t *testing.T) {
	repos, auth, _, _ := newAuthTestServices(t)
	u, _, err := auth.Register(t.Context(), "owner", "custom-password")
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := auth.CreateTemporaryPassword(t.Context(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.User.UpdateFields(t.Context(), u.ID, map[string]any{"force_password_reset": true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := auth.VerifyAndConsumeTemporaryPassword(t.Context(), u.Username, code); ok {
		t.Fatal("old OTP bypassed reset")
	}
	// No other user fields should be changed by the flag.
	var count int64
	repos.DB.Model(&model.User{}).Where("id = ? AND username = ?", u.ID, u.Username).Count(&count)
	if count != 1 {
		t.Fatal("user was replaced")
	}
}

func TestCachedRefreshRotationRespectsPasswordReset(t *testing.T) {
	repos, auth, _, _ := newAuthTestServices(t)
	u, pair, err := auth.Register(t.Context(), "owner", "custom-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RefreshTokens(t.Context(), pair.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := repos.User.UpdateFields(t.Context(), u.ID, map[string]any{"force_password_reset": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RefreshTokens(t.Context(), pair.RefreshToken); !errors.Is(err, ErrPasswordResetRequired) {
		t.Fatalf("cached refresh bypass: %v", err)
	}
}
