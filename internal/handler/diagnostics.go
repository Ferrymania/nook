package handler

import (
	"errors"
	"net/http"
	"net/http/pprof"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

func registerAdminDiagnosticsRoutes(admin *gin.RouterGroup, svc *service.Container) {
	diagnostics := admin.Group("/diagnostics/pprof")
	diagnostics.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if svc.Diagnostics == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "运行诊断不可用，请重启 Nook 后重试"})
			return
		}
		c.Next()
	})
	diagnostics.GET("/status", func(c *gin.Context) { c.JSON(http.StatusOK, svc.Diagnostics.Status()) })
	diagnostics.POST("/enable", diagnosticsControl(svc, true))
	diagnostics.POST("/disable", diagnosticsControl(svc, false))
	for _, name := range []string{"heap", "allocs", "goroutine", "profile"} {
		diagnostics.GET("/"+name, diagnosticsCapture(svc, name))
	}
}

func diagnosticsControl(svc *service.Container, enable bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Browser controls use the existing administrator session. Refuse
		// cross-origin mutations even when another origin is allowed by CORS.
		if origin := c.GetHeader("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, c.Request.Host) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "请从 Nook 设置页面操作运行诊断"})
				return
			}
		}
		status := service.DiagnosticsStatus{}
		if enable {
			status = svc.Diagnostics.Enable()
		} else {
			status = svc.Diagnostics.Disable()
		}
		if svc.Log != nil {
			svc.Log.Info("administrator changed runtime diagnostics", zap.String("user_id", c.GetString(middleware.CtxUserID)), zap.Bool("enabled", status.Enabled))
		}
		c.JSON(http.StatusOK, status)
	}
}

func diagnosticsCapture(svc *service.Container, name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !svc.Diagnostics.Status().Enabled {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "运行诊断已关闭，请在系统设置中重新开启"})
			return
		}
		// Only CPU duration is configurable. In particular, do not let arbitrary
		// pprof query parameters force GC, dump stacks as text, or start delta
		// profiles that hold a snapshot request open for an unbounded duration.
		query := url.Values{}
		if name == "profile" {
			seconds := 10
			if values, ok := c.Request.URL.Query()["seconds"]; ok {
				var err error
				if len(values) != 1 {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "CPU 采集时长须为 1–30 秒的整数"})
					return
				}
				seconds, err = strconv.Atoi(values[0])
				if err != nil || seconds < 1 || seconds > 30 {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "CPU 采集时长须为 1–30 秒的整数"})
					return
				}
			}
			query.Set("seconds", strconv.Itoa(seconds))
		}
		ctx, release, err := svc.Diagnostics.Begin(c.Request.Context())
		if err != nil {
			if errors.Is(err, service.ErrDiagnosticsBusy) {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "已有采集正在进行，请等待完成后重试"})
			} else {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "运行诊断已关闭，请在系统设置中重新开启"})
			}
			return
		}
		defer release()
		request := c.Request.Clone(ctx)
		request.URL.RawQuery = query.Encode()
		request.Form, request.PostForm = nil, nil
		filename := "nook-" + name + "-" + time.Now().UTC().Format("20060102T150405Z") + ".pprof"
		writer := &diagnosticsDownloadWriter{ResponseWriter: c.Writer, filename: filename}
		if name == "profile" {
			pprof.Profile(writer, request)
		} else {
			pprof.Handler(name).ServeHTTP(writer, request)
		}
	}
}

// The standard handlers set their own attachment names. Apply the dated name
// when headers are committed, while preserving plain error responses.
type diagnosticsDownloadWriter struct {
	http.ResponseWriter
	filename string
	wrote    bool
}

func (w *diagnosticsDownloadWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *diagnosticsDownloadWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		if code == http.StatusOK {
			w.Header().Set("Content-Disposition", `attachment; filename="`+w.filename+`"`)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *diagnosticsDownloadWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
