package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud115"
)

// Test115OAuthQRFlow 用 mock 的 115 开放平台接口走完设备码扫码授权：
// 创建账号（空凭据）→ 发起授权（取二维码）→ 轮询（未扫码/已扫码/已确认）
// → 确认后 token 写入账号 → 驱动可用。
func Test115OAuthQRFlow(t *testing.T) {
	for _, authSource := range []string{string(cloud115.SourceTypeBuiltInAppID), string(cloud115.SourceTypeCustomAppID)} {
		t.Run(authSource, func(t *testing.T) {
			svc := testStrmService(t)

			var scanCalls int
			pro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/open/user/info":
					w.Write([]byte(`{"state":true,"data":{"user_id":"115001","user_name":"测试用户"}}`))
				default:
					t.Errorf("unexpected pro path %s", r.URL.Path)
				}
			}))
			defer pro.Close()
			passport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/open/authDeviceCode":
					w.Write([]byte(`{"state":true,"data":{"uid":"U1","time":1700,"sign":"S1","qrcode":"https://img/qr.png"}}`))
				case "/open/deviceCodeToToken":
					w.Write([]byte(`{"state":true,"data":{"access_token":"open-at","refresh_token":"open-rt","expires_in":7200}}`))
				default:
					t.Errorf("unexpected passport path %s", r.URL.Path)
				}
			}))
			defer passport.Close()
			qr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				scanCalls++
				switch scanCalls {
				case 1:
					w.Write([]byte(`{"state":true,"data":{"status":0}}`))
				case 2:
					w.Write([]byte(`{"state":true,"data":{"status":1}}`))
				default:
					w.Write([]byte(`{"state":true,"data":{"status":2}}`))
				}
			}))
			defer qr.Close()

			oldPro, oldPassport, oldQR := cloud115.ProAPIBase, cloud115.PassportAPIBase, cloud115.QRCodeAPIBase
			cloud115.ProAPIBase, cloud115.PassportAPIBase, cloud115.QRCodeAPIBase = pro.URL, passport.URL, qr.URL
			defer func() {
				cloud115.ProAPIBase, cloud115.PassportAPIBase, cloud115.QRCodeAPIBase = oldPro, oldPassport, oldQR
			}()

			// 1. 创建空凭据 115 账号
			acct, err := svc.CreateStrmAccount(context.Background(), "115 家庭网盘", model.StrmProvider115, map[string]string{})
			if err != nil {
				t.Fatalf("create account: %v", err)
			}
			if HasStrmAccountCredential(acct) {
				t.Fatalf("new account should have no credential")
			}

			// 2. 发起设备码授权
			result, err := svc.Start115OAuth(context.Background(), acct.ID,
				authSource, "100195125")
			if err != nil {
				t.Fatalf("start oauth: %v", err)
			}
			if result.Mode != "qrcode" || result.QRCode == nil || result.QRCode.Qrcode == "" {
				t.Fatalf("bad start result: %#v", result)
			}

			// 3. 轮询：waiting -> scanned
			for _, want := range []string{"waiting", "scanned"} {
				status, err := svc.Poll115OAuth(context.Background(), result.SessionID)
				if err != nil {
					t.Fatalf("poll: %v", err)
				}
				if status.Status != want {
					t.Fatalf("poll status = %s, want %s", status.Status, want)
				}
			}

			// 4. 确认
			status, err := svc.Poll115OAuth(context.Background(), result.SessionID)
			if err != nil {
				t.Fatalf("poll confirm: %v", err)
			}
			if status.Status != "confirmed" {
				t.Fatalf("final status = %s, want confirmed", status.Status)
			}

			// 5. 账号已保存 token 与用户信息
			updated, err := svc.repo.StrmAccount.FindByID(context.Background(), acct.ID)
			if err != nil || updated == nil {
				t.Fatal("account missing")
			}
			cfg, err := svc.strmAccountConfig(updated)
			if err != nil {
				t.Fatal(err)
			}
			if cfg["access_token"] != "open-at" || cfg["refresh_token"] != "open-rt" {
				t.Fatalf("token not saved: %#v", cfg)
			}
			if cfg["user_name"] != "测试用户" {
				t.Fatalf("user info not saved: %#v", cfg)
			}
			if updated.Name != acct.Name {
				t.Fatalf("authorization overwrote account remark: %q -> %q", acct.Name, updated.Name)
			}
			if !HasStrmAccountCredential(updated) {
				t.Fatal("credential should be present after auth")
			}
		})
	}
}

// Test115TokenRefreshLoop 令牌刷新与失效处理。
func Test115TokenRefreshLoop(t *testing.T) {
	svc := testStrmService(t)
	var refreshCalls int
	passport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open/refreshToken" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		refreshCalls++
		if strings.Contains(r.PostFormValue("refresh_token"), "dead") {
			w.Write([]byte(`{"state":false,"code":40140119,"message":"已过期"}`))
			return
		}
		w.Write([]byte(`{"state":true,"data":{"access_token":"new-at","refresh_token":"new-rt","expires_in":7200}}`))
	}))
	defer passport.Close()
	old := cloud115.PassportAPIBase
	cloud115.PassportAPIBase = passport.URL
	defer func() { cloud115.PassportAPIBase = old }()

	cfg := map[string]string{
		"app_id": "100195125", "access_token": "at1", "refresh_token": "rt1",
	}
	acct, err := svc.CreateStrmAccount(context.Background(), "刷新测试", model.StrmProvider115, cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc.refresh115TokensOnce(context.Background())
	updated, _ := svc.repo.StrmAccount.FindByID(context.Background(), acct.ID)
	updatedCfg, _ := svc.strmAccountConfig(updated)
	if updatedCfg["access_token"] != "new-at" || updatedCfg["refresh_token"] != "new-rt" {
		t.Fatalf("token not refreshed: %#v", updatedCfg)
	}

	// 失效的 refresh_token：access_token 清空 + 标记失败
	deadCfg := map[string]string{"app_id": "100195125", "access_token": "at-d", "refresh_token": "rt-dead"}
	deadAcct, err := svc.CreateStrmAccount(context.Background(), "失效测试", model.StrmProvider115, deadCfg)
	if err != nil {
		t.Fatal(err)
	}
	svc.refresh115TokensOnce(context.Background())
	deadUpdated, _ := svc.repo.StrmAccount.FindByID(context.Background(), deadAcct.ID)
	deadUpdatedCfg, _ := svc.strmAccountConfig(deadUpdated)
	if deadUpdatedCfg["access_token"] != "" {
		t.Fatalf("dead account access_token should be cleared: %#v", deadUpdatedCfg)
	}
	if deadUpdated.LastTestOK {
		t.Fatal("dead account should be marked failed")
	}
}

func TestResolve115Source(t *testing.T) {
	for _, tc := range []struct {
		source string
		appID  string
		valid  bool
	}{
		{"built_in_appid", "100195125", true},
		{"custom_appid", " 123456789 ", true},
		{"custom_appid", " ", false},
		{"built_in_appid", "unknown", false},
		{"built_in_relay", "100195125", false},
		{"third_party_service", "100195125", false},
	} {
		t.Run(tc.source+"/"+tc.appID, func(t *testing.T) {
			source, err := resolve115Source(tc.source, tc.appID)
			if (err == nil) != tc.valid {
				t.Fatalf("source resolution valid = %v, want %v: %v", err == nil, tc.valid, err)
			}
			if err == nil && (source.Provider != cloud115.ProviderOfficialPKCE || source.AppID != strings.TrimSpace(tc.appID)) {
				t.Fatalf("unexpected source: %+v", source)
			}
		})
	}
}

func TestList115SourcesOnlyOfficialCatalog(t *testing.T) {
	sources, err := (&StrmService{}).List115Sources(context.Background())
	if err != nil || len(sources) != 1 || len(sources["built_in"]) == 0 {
		t.Fatalf("unexpected auth catalog: %d groups, %v", len(sources), err)
	}
	for _, source := range sources["built_in"] {
		if source.SourceType != cloud115.SourceTypeBuiltInAppID || source.Provider != cloud115.ProviderOfficialPKCE {
			t.Fatalf("unexpected catalog source: %+v", source)
		}
	}
}
