package service

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// 内置回退凭据必须能解出有效的 AppId（AppId 非机密），且与密文
// XOR 回环一致（AppKey 以回环校验完整性，避免把明文密钥再抄一遍）。
func TestDanmakuEmbeddedCredentials(t *testing.T) {
	appID, appKey := danmakuEmbeddedCredentials()
	require.Equal(t, "xap9r8p3g3", appID)
	require.NotEmpty(t, appKey)
	require.GreaterOrEqual(t, len(appKey), 24)

	// 回环：解出的明文再用同一混淆密钥 XOR 后必须还原出原密文，
	// 否则说明密文与密钥失配（改了一边忘了另一边）。
	require.Equal(t, danmakuEmbeddedAppIDHex, xorEncode(appID, dandanplayObfuscationKey))
	require.Equal(t, danmakuEmbeddedAppKeyHex, xorEncode(appKey, dandanplayObfuscationKey))
}

// xorEncode 是 xorDecode 的逆操作（测试辅助，与生产实现同规则）。
func xorEncode(plain, key string) string {
	out := make([]byte, len(plain))
	for i := range plain {
		out[i] = plain[i] ^ key[i%len(key)]
	}
	return hex.EncodeToString(out)
}

// 签名向量：算法 base64(sha256(AppId+Timestamp+Path+AppSecret))，
// 用固定时间戳与路径交叉验证实现与文档一致（向量由独立脚本生成）。
func TestDandanplaySignatureVectors(t *testing.T) {
	appID, appKey := danmakuEmbeddedCredentials()
	const ts = int64(1700000000)
	vectors := map[string]string{
		"/api/v2/comment/25484":   "p3OJPfcsm0aFUUXzUTIoKA3vo9fUUtpZRV7/fqX0t0Y=",
		"/api/v2/search/episodes": "x9Wr1tPWmeXAT8UeRK2eut9NRofOPsbp5qEl/uqXHC0=",
	}
	for path, want := range vectors {
		require.Equal(t, want, dandanplaySignature(appID, appKey, ts, path), "path=%s", path)
	}
}

// 用户凭据只用于其配置源；没有可用凭据时，仅官方源使用内置凭据。
func TestDanmakuCredentialsSelection(t *testing.T) {
	svc := newDanmakuTestService(t)
	ctx := context.Background()
	// These retired values must never override user or built-in credentials.
	require.NoError(t, svc.repo.Setting.Set(ctx, "danmaku.app_id", "retired-id"))
	require.NoError(t, svc.repo.Setting.Set(ctx, "danmaku.app_key", "retired-key"))
	embedID, embedKey := danmakuEmbeddedCredentials()
	official := "https://api.dandanplay.net/api/v2/comment/25484?withRelated=true"
	thirdParty := "https://dm.example.com/api/v2/comment/25484"
	for _, tc := range []struct {
		name, target    string
		user            *danmakuUserContext
		wantID, wantKey string
		wantOK          bool
	}{
		{"official defaults ignore global credentials", official, nil, embedID, embedKey, true},
		{"explicit official credentials", official, &danmakuUserContext{source: DanmakuDefaultSource, appID: "user-id", appKey: "user-key"}, "user-id", "user-key", true},
		{"empty user source means official", official, &danmakuUserContext{appID: "user-id", appKey: "user-key"}, "user-id", "user-key", true},
		{"incomplete official credentials", official, &danmakuUserContext{appID: "user-id"}, embedID, embedKey, true},
		{"mirror credentials do not reach official", official, &danmakuUserContext{source: "https://dm.example.com", appID: "mirror-id", appKey: "mirror-key"}, embedID, embedKey, true},
		{"mirror gets no built-in or global credentials", thirdParty, nil, "", "", false},
		{"official user credentials do not reach mirror", thirdParty, &danmakuUserContext{appID: "user-id", appKey: "user-key"}, "", "", false},
		{"explicit mirror credentials", thirdParty, &danmakuUserContext{source: "https://dm.example.com", appID: "mirror-id", appKey: "mirror-key"}, "mirror-id", "mirror-key", true},
		{"incomplete mirror credentials", thirdParty, &danmakuUserContext{source: "https://dm.example.com", appKey: "mirror-key"}, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctx
			if tc.user != nil {
				ctx = context.WithValue(ctx, danmakuUserContextKey{}, *tc.user)
			}
			id, key, ok := svc.danmakuCredentials(ctx, tc.target)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantID, id)
			require.True(t, key == tc.wantKey, "unexpected credential selection")
		})
	}
	// An incomplete retired pair is ignored too.
	require.NoError(t, svc.repo.Setting.Delete(ctx, "danmaku.app_key"))
	id, key, ok := svc.danmakuCredentials(ctx, official)
	require.True(t, ok)
	require.Equal(t, embedID, id)
	require.True(t, key == embedKey, "incomplete global credentials affected the built-in fallback")
}
