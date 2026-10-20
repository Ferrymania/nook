package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/gin-gonic/gin"
)

func TestLibraryScrapeStatusIsAdminOnlyAndTracksActiveTasks(t *testing.T) {
	router, svc, _, libraryID := newLibraryFilterEnv(t)
	if err := svc.Repo.DB.AutoMigrate(&model.ScrapeTask{}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []model.ScrapeTask{
		{LibraryID: libraryID, Status: model.ScrapeTaskPending},
		{LibraryID: libraryID, Status: model.ScrapeTaskRunning},
		{LibraryID: libraryID, Status: model.ScrapeTaskDone},
		{LibraryID: libraryID, Status: model.ScrapeTaskFailed},
		{LibraryID: "other-library", Status: model.ScrapeTaskRunning},
	} {
		if err := svc.Repo.ScrapeTask.Create(t.Context(), &task); err != nil {
			t.Fatal(err)
		}
	}
	router.GET("/api/libraries/:id/scrape-status", func(c *gin.Context) { c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role")); c.Next() }, middleware.AdminRequired(), libraryScrapeStatusHandler(svc))
	request := func(role, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/libraries/"+id+"/scrape-status", nil)
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
	var counts struct{ Pending, Running int64 }
	if w := request("admin", libraryID); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &counts) != nil || counts.Pending != 1 || counts.Running != 1 {
		t.Fatalf("current library status=%d body=%s", w.Code, w.Body.String())
	}
	if err := svc.Repo.DB.Model(&model.ScrapeTask{}).Where("library_id = ?", libraryID).Update("status", model.ScrapeTaskCanceled).Error; err != nil {
		t.Fatal(err)
	}
	if w := request("admin", libraryID); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &counts) != nil || counts.Pending != 0 || counts.Running != 0 {
		t.Fatalf("finished library must not inherit another library's task: %s", w.Body.String())
	}
	if counts, err := svc.Repo.ScrapeTask.CountActiveForLibraries(t.Context(), nil); err != nil || len(counts) != 0 {
		t.Fatalf("empty scope leaked active tasks: %+v %v", counts, err)
	}
}
