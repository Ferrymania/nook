package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func newDeviceServiceForTest(t *testing.T) (*DeviceService, *repository.Container, string) {
	t.Helper()
	repos := repository.New(newServiceTestDB(t))
	const userID = "user-1"
	if err := repos.User.Create(context.Background(), &model.User{
		Base:         model.Base{ID: userID},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "user",
		IsActive:     true,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewDeviceService(zap.NewNop(), repos)
	svc.SetSessionTracker(NewSessionTrackerService(zap.NewNop()))
	return svc, repos, userID
}

// 移除个人通知后，设备记录和踢下线仍应可用。
func TestDeviceServiceRecordsAndKicksDevices(t *testing.T) {
	svc, _, userID := newDeviceServiceForTest(t)
	svc.RecordLogin(context.Background(), userID, "dev-1", "Phone", "Infuse", "1.2.3.4")
	devices, err := svc.ListDevices(t.Context(), userID)
	if err != nil || len(devices) != 1 || devices[0].DeviceID != "dev-1" {
		t.Fatalf("recorded devices = %+v, error = %v", devices, err)
	}
	if err := svc.KickAllDevices(context.Background(), userID); err != nil {
		t.Fatal(err)
	}
	if !svc.IsDeviceKicked(t.Context(), userID, "dev-1") {
		t.Fatal("kicked device must require re-authentication")
	}
}

func TestDeviceRenameDoesNotWarnOrDisable(t *testing.T) {
	for _, warnings := range []int{0, 3} {
		t.Run(fmt.Sprintf("historical_warnings_%d", warnings), func(t *testing.T) {
			svc, repos, userID := newDeviceServiceForTest(t)
			ctx := t.Context()
			for _, kv := range [][2]string{
				{SettingAntiShareEnabled, "true"},
				{SettingMaxLoggedClients, "1"},
				// A saved value from the removed setting must have no effect.
				{"device.warn_threshold", "0"},
			} {
				if err := repos.Setting.Set(ctx, kv[0], kv[1]); err != nil {
					t.Fatal(err)
				}
			}
			if err := repos.User.UpdateFields(ctx, userID, map[string]any{"share_warnings": warnings}); err != nil {
				t.Fatal(err)
			}
			var adminCalls []string
			svc.SetAdminNotifier(func(_ context.Context, text string) {
				adminCalls = append(adminCalls, text)
			})
			now := time.Now().UTC()
			svc.sessions.now = func() time.Time { return now }
			for _, name := range []string{"Phone-A", "Phone-B", "Phone-C", "Phone-D"} {
				now = now.Add(2 * time.Minute)
				svc.RecordLogin(ctx, userID, "dev-1", name, "Infuse", "1.2.3.4")
			}
			u, err := repos.User.FindByID(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}
			if !u.IsActive || u.ShareWarnings != warnings || u.LastShareWarnAt != nil {
				t.Fatalf("renaming changed account state: active=%v warnings=%d last_warning=%v", u.IsActive, u.ShareWarnings, u.LastShareWarnAt)
			}
			if len(adminCalls) != 0 {
				t.Fatalf("renaming sent notifications: %v", adminCalls)
			}
			rows, err := repos.UserDevice.ListByUser(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].DeviceID != "dev-1" || rows[0].DeviceName != "Phone-D" || rows[0].Kicked {
				t.Fatalf("renaming must refresh the same device without kicking it: %+v", rows)
			}
		})
	}
}

func TestLoginDeviceLimitStillAppliesAfterRename(t *testing.T) {
	svc, repos, userID := newDeviceServiceForTest(t)
	ctx := t.Context()
	// Record two terminals before enabling the device limit.
	svc.RecordLogin(ctx, userID, "dev-1", "Phone", "Infuse", "1.2.3.4")
	svc.RecordLogin(ctx, userID, "dev-2", "TV", "Emby", "1.2.3.5")
	for _, kv := range [][2]string{
		{SettingAntiShareEnabled, "true"},
		{SettingMaxLoggedClients, "1"},
	} {
		if err := repos.Setting.Set(ctx, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	svc.RecordLogin(ctx, userID, "dev-1", "Renamed Phone", "Infuse", "1.2.3.4")
	u, err := repos.User.FindByID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if u.IsActive {
		t.Fatal("renaming must not bypass the logged-in terminal limit")
	}
}

func TestConcurrentPlaybackLimitStillApplies(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled_%v", enabled), func(t *testing.T) {
			svc, repos, userID := newDeviceServiceForTest(t)
			ctx := t.Context()
			for _, kv := range [][2]string{
				{SettingAntiShareEnabled, strconv.FormatBool(enabled)},
				{SettingMaxConcurrentPlay, "1"},
			} {
				if err := repos.Setting.Set(ctx, kv[0], kv[1]); err != nil {
					t.Fatal(err)
				}
			}
			svc.RecordPlayback(ctx, userID, "dev-1", "Phone", "Infuse")
			u, err := repos.User.FindByID(ctx, userID)
			if err != nil || !u.IsActive {
				t.Fatalf("one playback must remain allowed: user=%+v err=%v", u, err)
			}
			svc.RecordPlayback(ctx, userID, "dev-2", "TV", "Emby")
			u, err = repos.User.FindByID(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}
			if u.IsActive == enabled {
				t.Fatalf("active=%v, want %v for anti-share enabled=%v", u.IsActive, !enabled, enabled)
			}
		})
	}
}

// 账号因设备策略被禁用时，管理员也必须收到告警。
func TestPolicyDisableNotifiesAdmin(t *testing.T) {
	svc, repos, userID := newDeviceServiceForTest(t)

	// 启用防共享策略，设置最大并发客户端为 1
	for _, kv := range [][2]string{
		{SettingAntiShareEnabled, "true"},
		{SettingMaxLoggedClients, "1"},
		{SettingClientActiveDays, "30"},
	} {
		if err := repos.Setting.Set(context.Background(), kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}

	var adminCalls []string
	svc.SetAdminNotifier(func(_ context.Context, text string) {
		adminCalls = append(adminCalls, text)
	})

	// 两台不同设备登录，超出上限 → 触发禁用
	svc.RecordLogin(context.Background(), userID, "dev-1", "Phone", "Infuse", "1.2.3.4")
	svc.RecordLogin(context.Background(), userID, "dev-2", "TV", "Emby", "1.2.3.5")

	if len(adminCalls) == 0 {
		t.Fatal("admin must be notified when account is disabled by policy")
	}
	if !strings.Contains(adminCalls[0], "禁用") {
		t.Fatalf("admin notification = %q, want it to mention 禁用", adminCalls[0])
	}
}
