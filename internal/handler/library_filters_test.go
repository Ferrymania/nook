package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/Ferrymania/nook/internal/testutil"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
)

func newLibraryFilterEnv(t *testing.T) (*gin.Engine, *service.Container, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Library{}, &model.Media{}, &model.PlaybackHistory{}, &model.Setting{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	svc := &service.Container{Repo: repos, Log: zap.NewNop()}
	svc.Media = service.NewMediaService(nil, zap.NewNop(), repos)
	svc.Discovery = service.NewMediaDiscoveryService(zap.NewNop(), repos)

	const userID = "user-1"
	if err := repos.User.Create(context.Background(), &model.User{
		Base: model.Base{ID: userID}, Username: "tester", PasswordHash: "x", Role: "user", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	lib := &model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	authed := router.Group("/api", func(c *gin.Context) {
		c.Set(middleware.CtxUserID, userID)
		c.Set(middleware.CtxUserRole, "user")
		c.Next()
	})
	authed.GET("/libraries/:id/media", listMediaHandler(svc))
	authed.GET("/libraries/:id/facets", libraryFacetsHandler(svc))
	authed.GET("/libraries/:id/random", libraryRandomHandler(svc))
	return router, svc, userID, lib.ID
}

func seedLibraryMedia(t *testing.T, svc *service.Container, rows ...*model.Media) {
	t.Helper()
	for _, row := range rows {
		if err := svc.Repo.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func getJSON(t *testing.T, router *gin.Engine, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func TestLibraryFacetsReturnGenresAndYears(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "A", Genres: "Action,Drama", Year: 1999, Path: "/a.mkv"},
		&model.Media{LibraryID: libID, Title: "B", Genres: "Action", Year: 2021, Path: "/b.mkv"},
	)

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/facets")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	var facets struct {
		Genres []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"genres"`
		YearMin int `json:"year_min"`
		YearMax int `json:"year_max"`
	}
	if err := json.Unmarshal(body, &facets); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if facets.YearMin != 1999 || facets.YearMax != 2021 {
		t.Fatalf("year range = %d..%d, want 1999..2021", facets.YearMin, facets.YearMax)
	}
	if len(facets.Genres) != 2 {
		t.Fatalf("genres = %+v, want 2 entries", facets.Genres)
	}
	if facets.Genres[0].Name != "动作" || facets.Genres[0].Count != 2 {
		t.Fatalf("first genre = %+v, want 动作:2", facets.Genres[0])
	}
}

func TestLibraryGenreAliasesAgreeAcrossFacetsFiltersAndDetail(t *testing.T) {
	router, svc, userID, libID := newLibraryFilterEnv(t)
	const rawGenres = "27,Horror,恐怖,我的类型"
	m := &model.Media{LibraryID: libID, Title: "English", Genres: rawGenres, Path: "/english.mkv"}
	seedLibraryMedia(t, svc, m,
		&model.Media{LibraryID: libID, Title: "Chinese", Genres: "恐怖", Path: "/chinese.mkv"},
		&model.Media{LibraryID: libID, Title: "Hidden", Genres: "Horror", Path: "/hidden.mkv", NSFW: true},
		&model.Media{LibraryID: "other-library", Title: "Other", Genres: "Horror", Path: "/other.mkv"},
	)
	code, body := getJSON(t, router, "/api/libraries/"+libID+"/facets")
	var facets struct{ Genres []service.GenreCount }
	if err := json.Unmarshal(body, &facets); err != nil || code != http.StatusOK {
		t.Fatalf("facets: %d %s, err=%v", code, body, err)
	}
	if len(facets.Genres) != 2 || facets.Genres[0].Name != "恐怖" || facets.Genres[0].Count != 2 || !slices.Contains(facets.Genres[0].Aliases, "Horror") {
		t.Fatalf("facets include duplicates or hidden media: %+v", facets.Genres)
	}
	for _, query := range []string{"恐怖", "Horror", "horror", "27"} {
		for _, grouped := range []string{"0", "1"} {
			code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?group_versions="+grouped+"&genre="+url.QueryEscape(query))
			var page struct {
				Items []model.Media
				Total int64
			}
			if err := json.Unmarshal(body, &page); err != nil || code != http.StatusOK || len(page.Items) != 2 || page.Total != 2 {
				t.Fatalf("filter %q grouped=%s: %d %s, err=%v", query, grouped, code, body, err)
			}
		}
	}
	// Keep source metadata editable: translated labels must not be sent back as
	// raw genres when the metadata editor saves an unrelated field.
	auth := func(c *gin.Context) {
		c.Set(middleware.CtxUserID, userID)
		c.Set(middleware.CtxUserRole, "user")
		c.Next()
	}
	router.GET("/api/media/:id", auth, getMediaHandler(svc))
	router.PUT("/api/media/:id", auth, updateMediaMetadataHandler(svc))
	assertDetail := func(code int, body []byte) {
		t.Helper()
		var detail struct {
			Genres     string   `json:"genres"`
			GenreNames []string `json:"genre_names"`
		}
		if err := json.Unmarshal(body, &detail); err != nil || code != http.StatusOK || detail.Genres != rawGenres || !slices.Equal(detail.GenreNames, []string{"恐怖", "我的类型"}) {
			t.Fatalf("detail lost source or display names: %d %s, err=%v", code, body, err)
		}
	}
	code, body = getJSON(t, router, "/api/media/"+m.ID)
	assertDetail(code, body)
	payload, _ := json.Marshal(map[string]string{"overview": "Updated", "genres": rawGenres})
	req := httptest.NewRequest(http.MethodPut, "/api/media/"+m.ID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertDetail(w.Code, w.Body.Bytes())
	saved, err := svc.Repo.Media.FindByID(t.Context(), m.ID)
	if err != nil || saved == nil || saved.Genres != rawGenres || saved.Overview != "Updated" {
		t.Fatalf("metadata edit changed raw genres: %+v, err=%v", saved, err)
	}
}

// 空库时 facets 必须返回空数组而不是 null，前端无需额外判空。
func TestLibraryFacetsEmptyLibrary(t *testing.T) {
	router, _, _, libID := newLibraryFilterEnv(t)

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/facets")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	if !containsSubstring(string(body), `"genres":[]`) {
		t.Fatalf("body = %s, want genres:[]", body)
	}
}

// 列表筛选：按类型过滤后只返回命中的条目。
func TestListMediaAppliesGenreFilter(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "动作", Genres: "Action", Path: "/a.mkv"},
		&model.Media{LibraryID: libID, Title: "喜剧", Genres: "Comedy", Path: "/b.mkv"},
	)

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?group_versions=0&genre=Action")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	var payload struct {
		Items []model.Media `json:"items"`
		Total int64         `json:"total"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].Title != "动作" {
		t.Fatalf("payload = %+v, want only 动作", payload)
	}
}

func TestListMediaIgnoresRemovedUnmatchedFilter(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	svc.Media.SetRuntimeCache(service.NewRuntimeCacheService(nil))
	seedLibraryMedia(t, svc,
		&model.Media{Base: model.Base{ID: "matched"}, LibraryID: libID, Title: "Matched without poster", Path: "/matched.mkv", ScrapeStatus: "matched", Genres: "Comedy"},
		&model.Media{Base: model.Base{ID: "failed"}, LibraryID: libID, Title: "Failed", Path: "/failed.mkv", ScrapeStatus: "no_match"},
		&model.Media{Base: model.Base{ID: "pending"}, LibraryID: libID, Title: "Pending", Path: "/pending.mkv", ScrapeStatus: "pending"},
		&model.Media{Base: model.Base{ID: "legacy"}, LibraryID: libID, Title: "Legacy", Path: "/legacy.mkv"},
		&model.Media{LibraryID: "other-library", Title: "Other", Path: "/other.mkv", ScrapeStatus: "no_match", Genres: "Comedy"},
		&model.Media{LibraryID: libID, Title: "Hidden", Path: "/hidden.mkv", ScrapeStatus: "no_match", Genres: "Comedy", NSFW: true},
	)
	if err := svc.Repo.DB.Model(&model.Media{}).Where("id = ?", "legacy").UpdateColumn("scrape_status", nil).Error; err != nil {
		t.Fatal(err)
	}
	read := func(query string, wantTotal int64) []string {
		t.Helper()
		code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?"+query)
		var payload struct {
			Items []model.Media `json:"items"`
			Total int64         `json:"total"`
		}
		if code != http.StatusOK || json.Unmarshal(body, &payload) != nil || payload.Total != wantTotal {
			t.Fatalf("query %s: status=%d body=%s, want total %d", query, code, body, wantTotal)
		}
		ids := make([]string, len(payload.Items))
		for i, item := range payload.Items {
			ids[i] = item.ID
		}
		return ids
	}
	for _, grouping := range []string{"group_versions=0&", ""} {
		all := read(grouping+"page_size=10", 4)
		legacy := read(grouping+"unmatched=1&page_size=10", 4)
		if !slices.Equal(all, legacy) || !slices.Contains(legacy, "matched") {
			t.Fatalf("removed filter changed visible media: baseline=%v legacy=%v", all, legacy)
		}
		page := read(grouping+"page_size=1&page=2", 4)
		legacyPage := read(grouping+"unmatched=1&page_size=1&page=2", 4)
		if len(page) != 1 || !slices.Equal(page, legacyPage) {
			t.Fatalf("removed filter changed pagination: baseline=%v legacy=%v", page, legacyPage)
		}
		filtered := read(grouping+"unmatched=1&genre=Comedy", 1)
		if !slices.Equal(filtered, []string{"matched"}) {
			t.Fatalf("remaining genre and visibility filters changed: %v", filtered)
		}
	}
}

// 筛选条件必须进入缓存键：先请求未筛选列表、再筛选时不能命中旧缓存。
func TestListMediaFilterBypassesUnfilteredCache(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "动作", Genres: "Action", Path: "/a.mkv"},
		&model.Media{LibraryID: libID, Title: "喜剧", Genres: "Comedy", Path: "/b.mkv"},
	)

	// 先拉全量（可能写缓存），再拉筛选结果。
	if code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?group_versions=0"); code != http.StatusOK {
		t.Fatalf("unfiltered status = %d body=%s", code, body)
	}
	code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?group_versions=0&genre=Comedy")
	if code != http.StatusOK {
		t.Fatalf("filtered status = %d body=%s", code, body)
	}
	var payload struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 1 {
		t.Fatalf("total = %d, want 1 (filtered response must not be served from the unfiltered cache)", payload.Total)
	}
}

// 未观看筛选：已看完的不出现，看了一半的仍出现。
func TestListMediaUnwatchedFilter(t *testing.T) {
	router, svc, userID, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{Base: model.Base{ID: "m-done"}, LibraryID: libID, Title: "看完", Path: "/a.mkv"},
		&model.Media{Base: model.Base{ID: "m-half"}, LibraryID: libID, Title: "看一半", Path: "/b.mkv"},
	)
	for _, h := range []*model.PlaybackHistory{
		{UserID: userID, MediaID: "m-done", Completed: true},
		{UserID: userID, MediaID: "m-half", Completed: false},
	} {
		if err := svc.Repo.DB.Create(h).Error; err != nil {
			t.Fatal(err)
		}
	}

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?group_versions=0&unwatched=1")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	var payload struct {
		Items []model.Media `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Title != "看一半" {
		t.Fatalf("items = %+v, want only 看一半", payload.Items)
	}
}

func TestLibraryRandomReturnsMedia(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "唯一", Genres: "Action", Path: "/a.mkv"},
	)

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/random")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	var media model.Media
	if err := json.Unmarshal(body, &media); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if media.Title != "唯一" {
		t.Fatalf("title = %q, want 唯一", media.Title)
	}
}

// 筛选后没有命中时返回 404，前端据此提示「没有符合条件的媒体」。
func TestLibraryRandomEmptyResultIs404(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "动作", Genres: "Action", Path: "/a.mkv"},
	)

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/random?genre=Nonexistent")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s, want 404", code, body)
	}
}

// axios 默认把数组序列化为 genre[]=Action 格式；后端必须把它当作 genre=Action 处理。
func TestListMediaAcceptsBracketGenreParam(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "动作", Genres: "Action", Path: "/a.mkv"},
		&model.Media{LibraryID: libID, Title: "喜剧", Genres: "Comedy", Path: "/b.mkv"},
	)

	// genre[]=Action — axios bracket format without custom paramsSerializer
	code, body := getJSON(t, router, "/api/libraries/"+libID+"/media?group_versions=0&genre[]=Action")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	var payload struct {
		Items []model.Media `json:"items"`
		Total int64         `json:"total"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].Title != "动作" {
		t.Fatalf("payload = %+v, want only 动作 for genre[]=Action", payload)
	}
}

// 随机也遵守筛选：只命中 Action 时，带 Comedy 筛选必须 404。
func TestLibraryRandomHonoursFilters(t *testing.T) {
	router, svc, _, libID := newLibraryFilterEnv(t)
	seedLibraryMedia(t, svc,
		&model.Media{LibraryID: libID, Title: "A", Genres: "Action", Year: 2001, Path: "/a.mkv"},
		&model.Media{LibraryID: libID, Title: "B", Genres: "Comedy", Year: 2002, Path: "/b.mkv"},
	)

	code, body := getJSON(t, router, "/api/libraries/"+libID+"/random?genre=Comedy&year_min=2002")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, body)
	}
	var media model.Media
	if err := json.Unmarshal(body, &media); err != nil {
		t.Fatal(err)
	}
	if media.Title != "B" {
		t.Fatalf("title = %q, want B", media.Title)
	}
}
