package service

import (
	"context"
	"strings"
	"testing"

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

// 设备指纹警告仍须通知管理员。
func TestFingerprintWarnNotifiesAdmin(t *testing.T) {
	svc, repos, userID := newDeviceServiceForTest(t)

	// 启用防共享策略
	if err := repos.Setting.Set(context.Background(), SettingAntiShareEnabled, "true"); err != nil {
		t.Fatal(err)
	}

	type call struct{ text string }
	var adminCalls []call
	svc.SetAdminNotifier(func(_ context.Context, text string) {
		adminCalls = append(adminCalls, call{text})
	})

	// 第一次登录注册设备
	svc.RecordLogin(context.Background(), userID, "dev-1", "Phone-A", "Infuse", "1.2.3.4")
	// 同设备 ID 换设备名 → 触发指纹变更警告
	svc.RecordLogin(context.Background(), userID, "dev-1", "Phone-B", "Infuse", "1.2.3.4")

	if len(adminCalls) == 0 {
		t.Fatal("admin must be notified on fingerprint warn")
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
