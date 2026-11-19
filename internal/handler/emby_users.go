package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

// ─── Users / Auth ────────────────────────────────────────────────────────────

// embyAuthByNameHandler 处理 POST /Users/AuthenticateByName。
//
// 这是 Emby 客户端登录的唯一入口（Infuse / Yamby / Hills 等都走这里）。
// 验证账号凭据后返回 Emby 访问令牌和用户信息。
func embyAuthByNameHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, err := parseEmbyAuthByNameReq(c)
		if err != nil {
			embyError(c, http.StatusBadRequest, "invalid body")
			return
		}
		password := req.Pw
		if password == "" {
			password = req.Password
		}
		if strings.TrimSpace(req.Username) == "" || password == "" {
			if req.PasswordMd5 != "" || req.PasswordSha1 != "" {
				embyError(c, http.StatusBadRequest, "plain password required")
				return
			}
			embyError(c, http.StatusBadRequest, "missing username or password")
			return
		}
		user, accessToken, err := svc.Auth.LoginEmby(c.Request.Context(), req.Username, password)
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, service.ErrPasswordResetRequired) {
				status = http.StatusForbidden
			}
			embyError(c, status, err.Error())
			return
		}
		// 记录登录设备会话并执行防共享检测（登录客户端数 / 设备指纹）。
		clientInfo := embyClientInfoFromRequest(c)
		if svc.Sessions != nil {
			svc.Sessions.RecordLogin(c.Request.Context(), user.ID, user.Username,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP())
		}
		if svc.Device != nil {
			svc.Device.RecordLogin(c.Request.Context(), user.ID,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP())
		}
		userPayload, _ := svc.Emby.FindUser(c.Request.Context(), user.ID)
		// Login updates its timestamp asynchronously. The successful response
		// must already contain concrete dates, including on the first login.
		now := time.Now().UTC()
		if userPayload != nil {
			userPayload["LastLoginDate"] = now
			userPayload["LastActivityDate"] = now
		}
		embyRememberCompatSession(c, accessToken)
		c.JSON(http.StatusOK, gin.H{
			"AccessToken": accessToken,
			"ServerId":    "nook-001",
			"User":        userPayload,
			"SessionInfo": gin.H{
				"Id":                    user.ID,
				"UserId":                user.ID,
				"UserName":              user.Username,
				"Client":                clientInfo.Client,
				"DeviceId":              clientInfo.DeviceID,
				"DeviceName":            clientInfo.DeviceName,
				"ServerId":              "nook-001",
				"ApplicationVersion":    clientInfo.Version,
				"LastActivityDate":      now,
				"AdditionalUsers":       []any{},
				"PlayableMediaTypes":    []string{"Video", "Audio"},
				"SupportedCommands":     []string{},
				"SupportsRemoteControl": false,
				"PlayState": gin.H{
					"PositionTicks": int64(0), "CanSeek": false,
					"IsPaused": false, "IsMuted": false,
					"RepeatMode": "RepeatNone",
				},
			},
		})
	}
}

func embyPublicUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 公开用户列表（Emby Web 客户端登录页拉这个，列出可见用户）。
		users, err := svc.Emby.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, []any{})
			return
		}
		// 公开版本只暴露 Id + Name，不包含 Policy。
		out := make([]map[string]any, 0, len(users))
		for _, u := range users {
			out = append(out, map[string]any{
				"Id":          u["Id"],
				"Name":        u["Name"],
				"ServerId":    u["ServerId"],
				"HasPassword": true,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyListUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := svc.Emby.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, users)
	}
}

func embyMeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			embyError(c, http.StatusUnauthorized, "not authenticated")
			return
		}
		u, err := svc.Emby.FindUser(c.Request.Context(), uid)
		if err != nil || u == nil {
			embyError(c, http.StatusNotFound, "user not found")
			return
		}
		c.JSON(http.StatusOK, u)
	}
}

func embyGetUserByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			embyError(c, http.StatusUnauthorized, "not authenticated")
			return
		}
		// 只返回调用者自己的用户对象：客户端误传其他 userId 时回退到
		// 调用者自身（保留旧行为的兼容语义），但绝不返回他人数据。
		u, err := svc.Emby.FindUser(c.Request.Context(), uid)
		if err == nil && u != nil {
			c.JSON(http.StatusOK, u)
			return
		}
		c.JSON(http.StatusOK, embyFallbackUser(uid))
	}
}

// embyFallbackUser 是查库失败时的最后兜底（保持客户端可渲染）。
// Policy 必须是最小权限：不声明管理员/删除内容/控制他人等能力，
// 实际权限始终由服务端各路由的校验决定。
func embyFallbackUser(id string) gin.H {
	if strings.TrimSpace(id) == "" {
		id = "nook-user"
	}
	return gin.H{
		"Id":                        id,
		"Name":                      "Nook",
		"ServerId":                  "nook-001",
		"HasPassword":               true,
		"HasConfiguredPassword":     true,
		"HasConfiguredEasyPassword": false,
		"EnableAutoLogin":           false,
		"Policy": gin.H{
			"IsAdministrator":                 false,
			"EnableContentDeletion":           false,
			"EnableRemoteControlOfOtherUsers": false,
			"EnableSharedDeviceControl":       false,
			"EnableRemoteAccess":              true,
			"EnableAllDevices":                true,
			"EnableAllChannels":               true,
			"EnableAllFolders":                true,
		},
	}
}
