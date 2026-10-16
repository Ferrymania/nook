package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
)

func TestLibraryAutoProbeSettingIsIndependentAndPersistsFalse(t *testing.T) {
	router, svc, _, libraryID := newLibraryFilterEnv(t)
	other := &model.Library{Name: "Other", Path: "/media/other", Type: "movie", Enabled: true}
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
	if err != nil || initial == nil || initial.AutoProbe {
		t.Fatalf("automatic probing must default to off: library=%#v, error=%v", initial, err)
	}
	if w := patch("user", `{"auto_probe":true}`); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin changed probing: status=%d, body=%s", w.Code, w.Body.String())
	}
	for _, step := range []struct {
		body string
		want bool
	}{
		{`{"auto_probe":true}`, true},
		{`{"cover_url":"https://example.test/cover.jpg"}`, true},
		{`{"auto_probe":false}`, false},
	} {
		w := patch("admin", step.body)
		var got model.Library
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.AutoProbe != step.want {
			t.Fatalf("patch %s: status=%d, body=%s", step.body, w.Code, w.Body.String())
		}
		stored, err := svc.Repo.Library.FindByID(t.Context(), libraryID)
		if err != nil || stored == nil || stored.AutoProbe != step.want {
			t.Fatalf("setting was not persisted: library=%#v, error=%v", stored, err)
		}
		untouched, err := svc.Repo.Library.FindByID(t.Context(), other.ID)
		if err != nil || untouched == nil || untouched.AutoProbe {
			t.Fatalf("another library's setting changed: library=%#v, error=%v", untouched, err)
		}
	}
}
