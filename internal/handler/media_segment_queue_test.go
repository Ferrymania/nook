package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
)

func TestSegmentQueueAPIListsRetriesAndCleansOnlyHistory(t *testing.T) {
	router, svc, secret := newManualSegmentQueryRouter(t)
	svc.Log = zap.NewNop()
	registerAdminSegmentRoutes(router.Group("/api/admin", middleware.AuthRequired(secret), middleware.AdminRequired()), svc)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(segmentsProviderBody))
	}))
	defer provider.Close()
	svc.Segments = service.NewMediaSegmentService(svc.Log, svc.Repo).SetIntroDB(service.NewIntroDBService(svc.Log).SetBaseURL(provider.URL))
	if err := svc.Repo.DB.Model(&model.Media{}).Where("id = ?", "media-1").Update("tm_db_id", 27205).Error; err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, authed bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		if authed {
			req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret))
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := request("GET", "/api/admin/segments/queue", false); w.Code != 401 {
		t.Fatalf("unauthenticated queue=%d", w.Code)
	}
	if w := request("POST", "/api/media/media-1/segments/query", true); w.Code != 200 {
		t.Fatalf("query=%d %s", w.Code, w.Body.String())
	}
	w := request("GET", "/api/admin/segments/queue?q=OpenList&status=found&page=1&page_size=1", true)
	var queue repository.SegmentQueueSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &queue); err != nil || w.Code != 200 || queue.Total != 1 || queue.Tasks[0].LibraryName != "OpenList" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("queue=%s error=%v", w.Body.String(), err)
	}
	id := queue.Tasks[0].ID
	if w := request("POST", "/api/admin/segments/queue/"+id+"/retry", true); w.Code != 200 {
		t.Fatalf("retry=%d %s", w.Code, w.Body.String())
	}
	var retried model.MediaSegmentTask
	if err := svc.Repo.DB.Where("trigger = ?", "retry").First(&retried).Error; err != nil || retried.ID == id || retried.Status != "found" {
		t.Fatalf("retry not recorded: %+v %v", retried, err)
	}
	started := time.Now()
	active := &model.MediaSegmentTask{MediaID: "media-1", Status: "running", StartedAt: &started}
	if err := svc.Repo.SegmentTask.Create(t.Context(), active); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"DELETE", "POST"} {
		path := "/api/admin/segments/queue/" + active.ID
		if method == "POST" {
			path += "/retry"
		}
		if w := request(method, path, true); w.Code != 409 {
			t.Fatalf("active action=%d %s", w.Code, w.Body.String())
		}
	}
	if w := request("POST", "/api/admin/segments/queue/clear-finished", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted":2`) {
		t.Fatalf("clear=%s", w.Body.String())
	}
	rows, err := svc.Repo.MediaSegment.ListByMedia(t.Context(), "media-1")
	if err != nil || len(rows) == 0 {
		t.Fatalf("clearing query history deleted timestamps: %v", err)
	}
	if task, err := svc.Repo.SegmentTask.FindByID(t.Context(), active.ID); err != nil || task == nil {
		t.Fatalf("active task deleted: %v", err)
	}
	for _, suffix := range []string{"?status=bogus", "?page=-1", "?page=1000001"} {
		if w := request("GET", "/api/admin/segments/queue"+suffix, true); w.Code != 400 {
			t.Fatalf("bad filter accepted: %d", w.Code)
		}
	}
	if w := request("POST", "/api/admin/segments/queue/"+id+"/retry", true); w.Code != 404 {
		t.Fatalf("deleted record retry=%d", w.Code)
	}
	// The role guard must reject viewers before touching the database.
	viewer := gin.New()
	registerAdminSegmentRoutes(viewer.Group("/api/admin", func(c *gin.Context) { c.Set(middleware.CtxUserRole, "user") }, middleware.AdminRequired()), svc)
	w = httptest.NewRecorder()
	viewer.ServeHTTP(w, httptest.NewRequest("GET", "/api/admin/segments/queue", nil))
	if w.Code != 403 {
		t.Fatalf("viewer queue=%d", w.Code)
	}
}

func TestSegmentQueueRecoversInterruptedTasksAndPagesSearch(t *testing.T) {
	_, svc, _ := newManualSegmentQueryRouter(t)
	old, recent := time.Now().Add(-5*time.Minute), time.Now()
	for _, started := range []*time.Time{&old, &recent} {
		if err := svc.Repo.SegmentTask.Create(t.Context(), &model.MediaSegmentTask{MediaTitle: "测试任务", LibraryName: "番剧", Status: "running", StartedAt: started}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Repo.SegmentTask.InterruptStale(t.Context(), time.Now().Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	queue, err := svc.Repo.SegmentTask.List(t.Context(), "all", "番剧", 2, 1)
	if err != nil || queue.Total != 2 || len(queue.Tasks) != 1 || queue.Counts["failed"] != 1 || queue.Counts["running"] != 1 {
		t.Fatalf("pagination=%+v %v", queue, err)
	}
	failed, err := svc.Repo.SegmentTask.List(t.Context(), "failed", "中断", 1, 50)
	if err != nil || failed.Total != 1 || failed.Tasks[0].FinishedAt == nil {
		t.Fatalf("interruption not actionable: %+v %v", failed, err)
	}
}
