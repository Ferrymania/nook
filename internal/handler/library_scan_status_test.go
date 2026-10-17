package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
)

func TestLibraryScanStatusIsAdminOnlyAndScoped(t *testing.T) {
	router, svc, _, libraryID := newLibraryFilterEnv(t)
	svc.Tasks = service.NewTaskTrackerService(svc.Log, nil)
	other := svc.Tasks.Start(service.TaskKindScan, "other", service.TaskUpdate{LibraryID: "other"})
	defer other.Finish(nil, service.TaskUpdate{})
	task := svc.Tasks.Start(service.TaskKindScan, "current", service.TaskUpdate{LibraryID: libraryID, Metrics: map[string]int64{"visited": 12}})
	router.GET("/api/libraries/:id/scan-status", func(c *gin.Context) { c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role")); c.Next() }, middleware.AdminRequired(), libraryScanStatusHandler(svc))
	request := func(role, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/libraries/"+id+"/scan-status", nil)
		req.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := request("user", libraryID); w.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d", w.Code)
	}
	if w := request("admin", "missing"); w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", w.Code)
	}
	var result struct {
		Task service.BackgroundTask `json:"task"`
	}
	w := request("admin", libraryID)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Task.ID != task.ID() || result.Task.Metrics["visited"] != 12 {
		t.Fatalf("active: %s", w.Body.String())
	}
	task.Finish(nil, service.TaskUpdate{Metrics: map[string]int64{"visited": 20}})
	w = request("admin", libraryID)
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Task.Status != service.TaskStatusCompleted || result.Task.Metrics["visited"] != 20 {
		t.Fatalf("finished: %s", w.Body.String())
	}
}

func TestScanResponseIdentifiesFailureTask(t *testing.T) {
	router, svc, _, libraryID := newLibraryFilterEnv(t)
	svc.Tasks = service.NewTaskTrackerService(svc.Log, nil)
	if err := svc.Repo.DB.AutoMigrate(&model.LibraryRoot{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.DB.Model(&model.Library{}).Where("id = ?", libraryID).Update("path", filepath.Join(t.TempDir(), "missing")).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := svc.Repo.DB.DB()
	sqlDB.SetMaxOpenConns(1)
	svc.Scan = service.NewScannerService(&config.Config{}, svc.Log, svc.Repo, service.NewHub(svc.Log), nil, nil)
	router.POST("/api/libraries/:id/scan", scanLibraryHandler(svc))
	req := httptest.NewRequest("POST", "/api/libraries/"+libraryID+"/scan", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var result struct {
		TaskID string `json:"task_id"`
		Queued bool   `json:"queued"`
	}
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.TaskID == "" || !result.Queued {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if task := svc.Tasks.LibraryScan(libraryID); task != nil && task.Status == service.TaskStatusFailed {
			if task.ID != result.TaskID || task.Error == "" {
				t.Fatalf("failure: %#v", task)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("failed scan left the task running")
}

func TestScanDuplicateReturnsExistingTask(t *testing.T) {
	router, svc, _, id := newLibraryFilterEnv(t)
	svc.Tasks = service.NewTaskTrackerService(svc.Log, nil)
	svc.Scan = service.NewScannerService(&config.Config{}, svc.Log, svc.Repo, nil, nil, nil)
	finish, ok := svc.Scan.TryBeginLocalScan(id)
	if !ok {
		t.Fatal("cannot reserve scan")
	}
	defer finish()
	task := svc.Tasks.Start(service.TaskKindScan, "existing", service.TaskUpdate{LibraryID: id})
	router.POST("/api/libraries/:id/scan", scanLibraryHandler(svc))
	req := httptest.NewRequest("POST", "/api/libraries/"+id+"/scan", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var result struct {
		ID      string `json:"task_id"`
		Already bool   `json:"already_running"`
	}
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ID != task.ID() || !result.Already {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
	if len(svc.Tasks.Snapshot().Active) != 1 {
		t.Fatal("duplicate created a second task")
	}
}
