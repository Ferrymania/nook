package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/gin-gonic/gin"
)

func TestLibraryPlaybackThresholdValidationAndIsolation(t *testing.T) {
	router, svc, _, libraryID := newLibraryFilterEnv(t)
	other := &model.Library{Name: "Other", Path: "/other", Type: "movie", Enabled: true}
	if err := svc.Repo.Library.Create(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	router.PATCH("/api/libraries/:id", func(c *gin.Context) {
		c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role"))
		c.Next()
	}, middleware.AdminRequired(), updateLibraryHandler(svc))
	patch := func(role, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/libraries/"+libraryID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	initial, err := svc.Repo.Library.FindByID(t.Context(), libraryID)
	if err != nil || initial == nil || initial.PlayedThresholdPct != 90 || initial.MinResumePct != 5 || initial.MinResumeDurationSeconds != 180 {
		t.Fatalf("default threshold: library=%#v err=%v", initial, err)
	}
	if w := patch("user", `{"played_threshold_pct":95}`); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin update: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{
		`{"played_threshold_pct":0,"cover_url":"bad"}`,
		`{"played_threshold_pct":101}`,
		`{"played_threshold_pct":-1}`,
		`{"played_threshold_pct":90.5}`,
		`{"min_resume_pct":-1,"cover_url":"bad"}`,
		`{"min_resume_pct":90}`,
		`{"min_resume_pct":5.5}`,
		`{"min_resume_duration_seconds":-1}`,
		`{"min_resume_duration_seconds":180.5}`,
		`{"min_resume_duration_seconds":2147483648}`,
	} {
		if w := patch("admin", body); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid threshold accepted: %s: %d %s", body, w.Code, w.Body.String())
		}
		stored, err := svc.Repo.Library.FindByID(t.Context(), libraryID)
		if err != nil || stored == nil || stored.PlayedThresholdPct != 90 || stored.CoverURL != initial.CoverURL {
			t.Fatalf("invalid request changed library: %#v %v", stored, err)
		}
	}
	for _, step := range []struct {
		body string
		want int
	}{
		{`{"played_threshold_pct":95}`, 95},
		{`{"auto_probe":true}`, 95},
		{`{"played_threshold_pct":1,"min_resume_pct":0,"min_resume_duration_seconds":0}`, 1},
		{`{"played_threshold_pct":100}`, 100},
	} {
		w := patch("admin", step.body)
		var response model.Library
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.PlayedThresholdPct != step.want {
			t.Fatalf("patch %s: %d %s", step.body, w.Code, w.Body.String())
		}
		stored, err := svc.Repo.Library.FindByID(t.Context(), libraryID)
		if err != nil || stored == nil || stored.PlayedThresholdPct != step.want {
			t.Fatalf("threshold not persisted: %#v %v", stored, err)
		}
		untouched, err := svc.Repo.Library.FindByID(t.Context(), other.ID)
		if err != nil || untouched == nil || untouched.PlayedThresholdPct != 90 || untouched.MinResumePct != 5 || untouched.MinResumeDurationSeconds != 180 {
			t.Fatalf("another library changed: %#v %v", untouched, err)
		}
	}
	for _, role := range []string{"admin", "user"} {
		w := patch(role, `{"played_threshold_pct":95,"min_resume_pct":10,"min_resume_duration_seconds":300}`)
		if role == "user" {
			if w.Code != http.StatusForbidden {
				t.Fatalf("non-admin changed resume settings: %d", w.Code)
			}
			continue
		}
		var response model.Library
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.MinResumePct != 10 || response.MinResumeDurationSeconds != 300 {
			t.Fatalf("resume settings: %d %s", w.Code, w.Body.String())
		}
		// Reject a partial update that would conflict with the saved minimum.
		if w := patch(role, `{"played_threshold_pct":5,"cover_url":"bad"}`); w.Code != http.StatusBadRequest {
			t.Fatalf("conflicting update: %d", w.Code)
		}
		stored, _ := svc.Repo.Library.FindByID(t.Context(), libraryID)
		if stored.MinResumePct != 10 || stored.MinResumeDurationSeconds != 300 || stored.PlayedThresholdPct != 95 || stored.CoverURL != initial.CoverURL {
			t.Fatalf("invalid update changed settings: %+v", stored)
		}
	}
}
