package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/service"
)

type settingReq struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

// maskedSettingKeys 里的设置值绝不能被完整下发：它们是可用于对外操作的凭据。
// 下发脱敏值，保存时再靠 isMaskedSettingValue 还原为「保持原值」。
var maskedSettingKeys = map[string]bool{
	service.SettingTelegramBotToken: true,
}

func listSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := svc.Repo.Setting.All(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for i := range settings {
			if maskedSettingKeys[settings[i].Key] {
				settings[i].Value = service.MaskSecret(settings[i].Value)
			}
		}
		c.JSON(http.StatusOK, settings)
	}
}

// isMaskedSettingValue 识别「前端把脱敏值原样提交回来」的情况。此时必须保留
// 已存的真实值，否则一次保存就会把凭据覆盖成 ***。
func isMaskedSettingValue(value string) bool {
	return strings.Contains(value, "***")
}

func updateSettingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// 脱敏值回传 == 用户没改这个凭据，保留库里已存的真实值。
		if maskedSettingKeys[req.Key] && isMaskedSettingValue(req.Value) {
			c.Status(http.StatusNoContent)
			return
		}
		if err := svc.Repo.Setting.Set(c.Request.Context(), req.Key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		service.ApplyRuntimeSetting(svc.Cfg, req.Key, req.Value)
		if err := applyHTTPSetting(svc, req.Key, req.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if svc.FFprobe != nil && (req.Key == "ffprobe.max_concurrent" || req.Key == "app.ffprobe_max_concurrent") {
			svc.FFprobe.SetMaxConcurrent(svc.Cfg.App.FFprobeMaxConcurrent)
		}
		if req.Key == "transcode.enabled" && !svc.Cfg.Transcoder.Enabled {
			svc.Transcoder.StopAll()
		}
		if req.Key == "transcode.hw_enabled" || req.Key == "transcode.hw_accel" || req.Key == "transcoder.hardware_accel" || req.Key == "transcoder.encoder" {
			svc.Transcoder.StopAll()
		}
		if req.Key == "cache.images_max_size_mb" || req.Key == "cache.images_originals_max_size_mb" || req.Key == "cache.images_originals_ttl_hours" {
			if svc.Scheduler != nil {
				_ = svc.Scheduler.RunNowAsync(c.Request.Context(), "image_cache_cleanup")
			}
		}
		if req.Key == "cache.memory_max_size_mb" && svc.Cache != nil {
			svc.Cache.SetMaxSizeMB(svc.Cfg.Cache.MemoryMaxSizeMB)
		}
		c.Status(http.StatusNoContent)
	}
}

// applyHTTPSetting 校验 HTTPS 相关设置，并在可行时热重载监听。
// 必须在 ApplyRuntimeSetting 之后调用，这样 svc.Cfg 已反映刚保存的值。
//
// 规则：
//   - https.enabled=true 时强制要求证书与私钥都已配置（内容或路径均可）且匹配，
//     否则返回错误（"如果启用就必须配置 SSL 证书和密钥"）；
//   - 证书/私钥（内容或路径）单独保存时只校验格式；若 HTTPS 已开启且新的整体
//     配置可解析匹配才触发重载，避免"只存了新证书、私钥还没保存"时用旧私钥带
//     新证书对外提供服务。
func applyHTTPSetting(svc *service.Container, key, value string) error {
	skipReload := func(reason string) {
		if svc.Log != nil {
			svc.Log.Warn("https setting saved but not applied yet", zap.String("key", key), zap.String("reason", reason))
		}
	}

	config.RuntimeMu.RLock()
	httpsEnabled := svc.Cfg.App.HTTPSEnabled
	cert := svc.Cfg.App.SSLCert
	certPath := svc.Cfg.App.SSLCertPath
	keyMaterial := svc.Cfg.App.SSLKey
	keyPath := svc.Cfg.App.SSLKeyPath
	config.RuntimeMu.RUnlock()

	switch key {
	case "https.enabled":
		if httpsEnabled {
			if _, err := service.ResolveSSLKeyPair(cert, certPath, keyMaterial, keyPath); err != nil {
				return fmt.Errorf("启用 HTTPS 失败：%v", err)
			}
		}
	case "https.cert", "https.cert_path", "https.key", "https.key_path":
		if err := validateSSLMaterialSource(key, value); err != nil {
			return err
		}
		if !httpsEnabled {
			return nil
		}
		if !httpsPairReady(svc) {
			skipReload("证书与私钥尚未匹配，等待另一半保存后生效")
			return nil
		}
	default:
		return nil
	}
	if svc.ReloadHTTPServer != nil {
		return svc.ReloadHTTPServer()
	}
	return nil
}

// validateSSLMaterialSource 校验刚保存的证书/私钥来源（内容或路径）本身格式合法。
func validateSSLMaterialSource(key, value string) error {
	switch key {
	case "https.cert":
		return service.ValidateSSLCert(value)
	case "https.cert_path":
		if strings.TrimSpace(value) == "" {
			return nil // 清空路径也允许，启用时由整体校验把关
		}
		pemStr, err := service.ResolveSSLMaterial("", value, "证书")
		if err != nil {
			return err
		}
		return service.ValidateSSLCert(pemStr)
	case "https.key":
		return service.ValidateSSLKey(value)
	case "https.key_path":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		pemStr, err := service.ResolveSSLMaterial("", value, "私钥")
		if err != nil {
			return err
		}
		return service.ValidateSSLKey(pemStr)
	}
	return nil
}

// httpsPairReady 判断基于当前配置解析出的证书/私钥是否完整且匹配。
func httpsPairReady(svc *service.Container) bool {
	config.RuntimeMu.RLock()
	cert := svc.Cfg.App.SSLCert
	certPath := svc.Cfg.App.SSLCertPath
	keyMaterial := svc.Cfg.App.SSLKey
	keyPath := svc.Cfg.App.SSLKeyPath
	config.RuntimeMu.RUnlock()
	_, err := service.ResolveSSLKeyPair(cert, certPath, keyMaterial, keyPath)
	return err == nil
}

func recentLogsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.Repo.Log.Recent(c.Request.Context(), 200)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rows)
	}
}
