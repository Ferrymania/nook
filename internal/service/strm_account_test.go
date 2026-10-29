package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service/cloud"
	"github.com/Ferrymania/nook/internal/service/cloud115"
)

func TestOpenListTokenValidationAndPreview(t *testing.T) {
	svc := &StrmService{crypto: NewCryptoService("test-secret", zap.NewNop())}
	encrypted := svc.crypto.Encrypt("saved-token")
	legacy := mustJSON(map[string]string{
		"server": "https://list.example.com", "username": "legacy", "password": svc.crypto.Encrypt("secret"), "token": encrypted,
	})
	account := &model.StrmAccount{Provider: model.StrmProviderOpenList, Config: legacy}
	preview := svc.StrmAccountConfigPreviewOf(account)
	if preview.Server != "https://list.example.com" || !preview.HasToken || preview.Username != "" || preview.HasPassword {
		t.Fatalf("OpenList preview must only expose address and Token state: %#v", preview)
	}
	if err := svc.validateOpenListAccountToken(account.Config); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "  ", svc.crypto.Encrypt("  ")} {
		raw := mustJSON(map[string]string{"username": "legacy", "password": "secret", "token": token})
		if err := svc.validateOpenListAccountToken(raw); !errors.Is(err, cloud.ErrOpenListTokenRequired) {
			t.Fatalf("invalid Token validation = %v", err)
		}
	}
	for _, raw := range []string{`{"password":"secret"}`, `{"token":""}`, `{"token":"  "}`, `not-json`} {
		if HasStrmAccountCredential(&model.StrmAccount{Provider: model.StrmProviderOpenList, Config: raw}) {
			t.Error("missing Token counted as configured")
		}
	}
	emby := &model.StrmAccount{Provider: model.StrmProviderEmbyRemote, Config: legacy}
	embyPreview := svc.StrmAccountConfigPreviewOf(emby)
	if embyPreview.Username != "legacy" || !embyPreview.HasPassword {
		t.Error("Emby username/password preview changed")
	}
}

func TestUpdateOpenListTokenPreservesSavedToken(t *testing.T) {
	svc := testStrmService(t)
	ctx := context.Background()
	for _, token := range []string{"", "  "} {
		if _, err := svc.CreateStrmAccount(ctx, "password-only", model.StrmProviderOpenList, map[string]string{
			"server": "https://list.example.com", "username": "alice", "password": "secret", "token": token,
		}); !errors.Is(err, cloud.ErrOpenListTokenRequired) {
			t.Fatalf("password-only create = %v", err)
		}
	}
	acct, err := svc.CreateStrmAccount(ctx, "openlist", model.StrmProviderOpenList, map[string]string{
		"server": "https://list.example.com", "token": "original-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.UpdateStrmAccount(ctx, acct.ID, "renamed", nil, map[string]string{"server": "https://new.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.strmAccountConfig(updated)
	if err != nil {
		t.Fatal(err)
	}
	if cfg["token"] != "original-token" || cfg["server"] != "https://new.example.com" || updated.Name != "renamed" {
		t.Error("address-only save did not preserve Token")
	}
	if _, err := svc.UpdateStrmAccount(ctx, acct.ID, "", nil, map[string]string{"token": "", "username": "alice", "password": "secret"}); !errors.Is(err, cloud.ErrOpenListTokenRequired) {
		t.Fatalf("switch to password = %v", err)
	}
	fresh, err := svc.repo.StrmAccount.FindByID(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = svc.strmAccountConfig(fresh)
	if err != nil || cfg["token"] != "original-token" {
		t.Fatal("failed save changed stored Token")
	}
	replaced, err := svc.UpdateStrmAccount(ctx, acct.ID, "", nil, map[string]string{"token": "replacement-token"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = svc.strmAccountConfig(replaced)
	if err != nil || cfg["token"] != "replacement-token" {
		t.Fatal("Token replacement failed")
	}

}

func TestProviderForReuses115ClientUntilCredentialsChange(t *testing.T) {
	svc := testStrmService(t)
	ctx := context.Background()
	acct, err := svc.CreateStrmAccount(ctx, "115", model.StrmProvider115, map[string]string{
		"app_id":        "100195129",
		"access_token":  "at-1",
		"refresh_token": "rt-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	first, err := svc.providerFor(ctx, acct)
	if err != nil {
		t.Fatalf("first provider: %v", err)
	}
	second, err := svc.providerFor(ctx, acct)
	if err != nil {
		t.Fatalf("second provider: %v", err)
	}
	if first != second {
		t.Fatal("115 provider should be shared per account")
	}

	updated, err := svc.UpdateStrmAccount(ctx, acct.ID, "", nil, map[string]string{
		"access_token":  "at-2",
		"refresh_token": "rt-2",
	})
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	third, err := svc.providerFor(ctx, updated)
	if err != nil {
		t.Fatalf("provider after credential update: %v", err)
	}
	if first == third {
		t.Fatal("credential update must invalidate the shared provider")
	}

	oldClient := first.(interface{ OpenClient() *cloud115.OpenClient }).OpenClient()
	oldClient.OnTokenRefreshed("at-stale", "rt-stale")
	fresh, err := svc.repo.StrmAccount.FindByID(ctx, acct.ID)
	if err != nil || fresh == nil {
		t.Fatalf("reload account: %v", err)
	}
	freshCfg, err := svc.strmAccountConfig(fresh)
	if err != nil {
		t.Fatalf("decode refreshed account: %v", err)
	}
	if freshCfg["access_token"] != "at-2" || freshCfg["refresh_token"] != "rt-2" {
		t.Fatalf("stale client overwrote new credentials: %#v", freshCfg)
	}
}

func TestPersist115TokensKeepsSharedClient(t *testing.T) {
	svc := testStrmService(t)
	ctx := context.Background()
	acct, err := svc.CreateStrmAccount(ctx, "115", model.StrmProvider115, map[string]string{
		"app_id":        "100195129",
		"access_token":  "at-1",
		"refresh_token": "rt-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	first, err := svc.providerFor(ctx, acct)
	if err != nil {
		t.Fatalf("first provider: %v", err)
	}

	svc.persist115Tokens(acct.ID, "at-2", "rt-2")
	fresh, err := svc.repo.StrmAccount.FindByID(ctx, acct.ID)
	if err != nil || fresh == nil {
		t.Fatalf("reload account: %v", err)
	}
	second, err := svc.providerFor(ctx, fresh)
	if err != nil {
		t.Fatalf("second provider: %v", err)
	}
	if first != second {
		t.Fatal("automatic token persistence must keep the in-memory shared client")
	}
}

// TestTestStrmAccountKeepsTokensRefreshedDuringPing 回归测试：
// Ping 期间 115 客户端可能刷新并持久化 token。账号测试只能写 last_test_*
// 字段，不能再用请求开始时读取的旧 Config 整包覆盖，否则新 token 会被旧值
// 覆盖，最终导致账号在下次重启后失效。
func TestTestStrmAccountKeepsTokensRefreshedDuringPing(t *testing.T) {
	svc := testStrmService(t)
	ctx := context.Background()
	acct, err := svc.CreateStrmAccount(ctx, "115", model.StrmProvider115, map[string]string{
		"app_id":        "100195129",
		"access_token":  "at-old",
		"refresh_token": "rt-old",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	svc.providerMu.Lock()
	svc.provider115Cache[acct.ID] = &refreshOnPing115Provider{svc: svc, accountID: acct.ID}
	svc.providerMu.Unlock()

	got := svc.TestStrmAccount(ctx, acct.ID)
	if got == nil {
		t.Fatal("TestStrmAccount returned nil")
	}
	if !got.LastTestOK || got.LastTestResult != "ok" {
		t.Fatalf("test result = (%v, %q), want ok", got.LastTestOK, got.LastTestResult)
	}
	gotCfg, err := svc.strmAccountConfig(got)
	if err != nil {
		t.Fatalf("decode returned config: %v", err)
	}
	if gotCfg["access_token"] != "at-new" || gotCfg["refresh_token"] != "rt-new" {
		t.Fatalf("returned account lost refreshed tokens: %#v", gotCfg)
	}

	fresh, err := svc.repo.StrmAccount.FindByID(ctx, acct.ID)
	if err != nil || fresh == nil {
		t.Fatalf("reload account: %v", err)
	}
	freshCfg, err := svc.strmAccountConfig(fresh)
	if err != nil {
		t.Fatalf("decode persisted config: %v", err)
	}
	if freshCfg["access_token"] != "at-new" || freshCfg["refresh_token"] != "rt-new" {
		t.Fatalf("persisted account lost refreshed tokens: %#v", freshCfg)
	}
}

// refreshOnPing115Provider 模拟真实 115 客户端在 Ping 内完成 token 轮转并
// 通过持久化回调写回新 token 的行为。
type refreshOnPing115Provider struct {
	svc       *StrmService
	accountID string
}

func (p *refreshOnPing115Provider) Type() string { return model.StrmProvider115 }

func (p *refreshOnPing115Provider) Ping(context.Context) error {
	p.svc.persist115Tokens(p.accountID, "at-new", "rt-new")
	return nil
}

func (p *refreshOnPing115Provider) List(context.Context, string) ([]cloud.FileEntry, error) {
	return nil, nil
}

func (p *refreshOnPing115Provider) Resolve(context.Context, string) (*cloud.DirectLink, error) {
	return nil, nil
}

func TestDeleteStrmAccountCascadesEmbyMounts(t *testing.T) {
	ctx := context.Background()
	db := newServiceTestDB(t, &model.StrmAccount{}, &model.EmbyMount{}, &model.StrmSyncPath{})
	repos := repository.New(db)
	svc := NewStrmService(nil, zap.NewNop(), repos, nil)

	createEmbyAcct := func(id, name string) *model.StrmAccount {
		acct := &model.StrmAccount{
			Base:     model.Base{ID: id},
			Name:     name,
			Provider: model.StrmProviderEmbyRemote,
			Enabled:  true,
		}
		if err := repos.StrmAccount.Create(ctx, acct); err != nil {
			t.Fatalf("create account %s: %v", id, err)
		}
		return acct
	}
	createMounts := func(accountID string, viewIDs ...string) {
		for _, vid := range viewIDs {
			m := &model.EmbyMount{
				AccountID:      accountID,
				RemoteViewID:   vid,
				RemoteViewName: "库-" + vid,
				Enabled:        true,
			}
			if err := repos.EmbyMount.Create(ctx, m); err != nil {
				t.Fatalf("create mount %s: %v", vid, err)
			}
		}
	}

	gone := createEmbyAcct("acct-gone", "要删除的账号")
	createMounts(gone.ID, "view-1", "view-2", "view-3")
	keep := createEmbyAcct("acct-keep", "保留的账号")
	createMounts(keep.ID, "view-a")

	if err := svc.DeleteStrmAccount(ctx, gone.ID); err != nil {
		t.Fatalf("DeleteStrmAccount failed: %v", err)
	}

	remaining, err := repos.StrmAccount.FindByID(ctx, gone.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if remaining != nil {
		t.Fatalf("account %s should be deleted", gone.ID)
	}
	goneMounts, err := repos.EmbyMount.ListByAccountID(ctx, gone.ID)
	if err != nil {
		t.Fatalf("list mounts: %v", err)
	}
	if len(goneMounts) != 0 {
		t.Fatalf("deleted account still has %d mounts (orphans)", len(goneMounts))
	}
	keepMounts, err := repos.EmbyMount.ListByAccountID(ctx, keep.ID)
	if err != nil {
		t.Fatalf("list mounts: %v", err)
	}
	if len(keepMounts) != 1 || keepMounts[0].RemoteViewID != "view-a" {
		t.Fatalf("keep account mounts = %#v, want 1 (view-a)", keepMounts)
	}
}

func TestCleanupOrphanMountsRemovesStaleRows(t *testing.T) {
	ctx := context.Background()
	db := newServiceTestDB(t, &model.StrmAccount{}, &model.EmbyMount{})
	repos := repository.New(db)
	svc := NewEmbyRemoteService(&config.Config{}, zap.NewNop(), repos, nil)

	acct := &model.StrmAccount{
		Base:     model.Base{ID: "acct-1"},
		Name:     "emby",
		Provider: model.StrmProviderEmbyRemote,
		Enabled:  true,
	}
	if err := repos.StrmAccount.Create(ctx, acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	for i, vid := range []string{"v1", "v2", "v-orphan-1", "v-orphan-2"} {
		m := &model.EmbyMount{
			Base:         model.Base{ID: "mount-" + vid},
			AccountID:    acct.ID,
			RemoteViewID: vid,
			Enabled:      true,
		}
		if i >= 2 {
			// 模拟历史残留：挂载归属不存在的账号
			m.AccountID = "no-such-account"
		}
		if err := repos.EmbyMount.Create(ctx, m); err != nil {
			t.Fatalf("create mount %s: %v", vid, err)
		}
	}

	svc.CleanupOrphanMounts(ctx)

	left, err := repos.EmbyMount.List(ctx)
	if err != nil {
		t.Fatalf("list mounts: %v", err)
	}
	if len(left) != 2 {
		t.Fatalf("mounts after cleanup = %d, want 2 (orphans removed)", len(left))
	}
	for _, m := range left {
		if m.AccountID != acct.ID {
			t.Fatalf("mount %s still orphan (account %s)", m.ID, m.AccountID)
		}
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestStrmAccountUsernameIsSeparateFromRemarkAndCredentials(t *testing.T) {
	for _, provider := range []string{model.StrmProvider115, model.StrmProviderOpenList} {
		account := &model.StrmAccount{Name: "家庭网盘", Provider: provider, Config: `{"user_name":"网盘用户","username":"登录字段","token":"secret"}`}
		if got := StrmAccountUsernameOf(account); got != "网盘用户" {
			t.Errorf("username = %q", got)
		}
		if account.Name != "家庭网盘" {
			t.Error("remark was overwritten")
		}
		account.Config = `{"username":"登录字段"}`
		if got := StrmAccountUsernameOf(account); got != "" {
			t.Error("login field was mistaken for API username")
		}
	}
}

func TestRefreshOpenListUsernamePreservesRemarkAndCredentials(t *testing.T) {
	svc := testStrmService(t)
	ctx := context.Background()
	requests := 0
	var duringFetch func()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.Path != "/api/me" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if duringFetch != nil {
			duringFetch()
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"username":"OpenList用户"}}`))
	}))
	defer srv.Close()
	acct, err := svc.CreateStrmAccount(ctx, "家庭备注", model.StrmProviderOpenList, map[string]string{"server": srv.URL, "token": "original-token"})
	if err != nil {
		t.Fatal(err)
	}
	first := svc.RefreshStrmAccountUsername(ctx, acct)
	if first.Name != "家庭备注" || StrmAccountUsernameOf(first) != "OpenList用户" {
		t.Fatal("remark and username were not kept separate")
	}
	cfg, err := svc.strmAccountConfig(first)
	if err != nil || cfg["token"] != "original-token" {
		t.Fatal("fetch username changed credentials")
	}
	before := requests
	listed, err := svc.ListAccounts(ctx)
	if err != nil || len(listed) != 1 || StrmAccountUsernameOf(&listed[0]) != "OpenList用户" || requests != before {
		t.Fatal("listing should use locally stored username")
	}
	// A credential update during the API request must win over the stale response.
	duringFetch = func() {
		if _, err := svc.UpdateStrmAccount(ctx, acct.ID, "新备注", nil, map[string]string{"token": "new-token", "user_name": "新用户"}); err != nil {
			t.Error(err)
		}
	}
	fresh := svc.RefreshStrmAccountUsername(ctx, first)
	cfg, err = svc.strmAccountConfig(fresh)
	if err != nil || cfg["token"] != "new-token" || cfg["user_name"] != "新用户" || fresh.Name != "新备注" {
		t.Fatal("stale user-info lookup overwrote concurrent account update")
	}
}
