package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
)

func newManualSegmentQueryRouter(t *testing.T) (*gin.Engine, *service.Container, string) {
	_, svc, secret := newPlaybackScopeTestRouter(t)
	router := gin.New()
	registerAuthedMediaRoutes(router.Group("/api", middleware.AuthRequired(secret)), svc)
	return router, svc, secret
}

func TestManualSegmentQueryHandlerReturnsResultsAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		upstream int
		body     string
		metadata bool
		want     int
		message  string
	}{
		{"found", 200, segmentsProviderBody, true, 200, ""},
		{"not_found", 404, "", true, 200, ""},
		{"missing_metadata", 200, segmentsProviderBody, false, 400, "请先完成刮削"},
		{"rate_limit", 429, "", true, 503, "限制了查询"},
		{"bad_response", 200, "invalid", true, 502, "查询失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, svc, secret := newManualSegmentQueryRouter(t)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/media" || r.URL.Query().Get("tmdb_id") != "27205" {
					t.Errorf("unexpected provider request: %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.upstream)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(provider.Close)
			svc.Segments = service.NewMediaSegmentService(svc.Log, svc.Repo).SetIntroDB(service.NewIntroDBService(svc.Log).SetBaseURL(provider.URL).SetRetryDelay(0))
			if tc.metadata {
				if err := svc.Repo.DB.Model(&model.Media{}).Where("id = ?", "media-1").Update("tm_db_id", 27205).Error; err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest("POST", "/api/media/media-1/segments/query", nil)
			req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.message) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("query response should not be cached")
			}
			if tc.want == 200 {
				var result service.MediaSegmentQueryResult
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Segments == nil || result.Found != (tc.upstream == 200) || result.FetchedAt.IsZero() {
					t.Fatalf("result=%#v err=%v", result, err)
				}
			}
		})
	}
}

func TestManualSegmentQueryHandlerRequiresAdminAndVisibleMedia(t *testing.T) {
	router, svc, secret := newManualSegmentQueryRouter(t)
	svc.Segments = service.NewMediaSegmentService(svc.Log, svc.Repo)
	req := httptest.NewRequest("POST", "/api/media/media-1/segments/query", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", w.Code)
	}
	for _, id := range []string{"missing", "emby-remote-id"} {
		req = httptest.NewRequest("POST", "/api/media/"+id+"/segments/query", nil)
		req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret))
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 404 {
			t.Fatalf("unknown media status=%d body=%s", w.Code, w.Body.String())
		}
	}
	// Verify the role guard independently of request contents and provider state.
	userRouter := gin.New()
	userRouter.POST("/query", func(c *gin.Context) { c.Set(middleware.CtxUserRole, "user"); c.Next() }, middleware.AdminRequired(), queryMediaSegmentsHandler(svc))
	w = httptest.NewRecorder()
	userRouter.ServeHTTP(w, httptest.NewRequest("POST", "/query", nil))
	if w.Code != 403 {
		t.Fatalf("viewer status=%d", w.Code)
	}
}
