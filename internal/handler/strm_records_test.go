package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStrmSyncRecordPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.StrmSyncRecord{}); err != nil {
		t.Fatal(err)
	}
	// 同一创建时间也必须稳定排序，翻页不能重复或漏掉记录。
	created := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 63; i++ {
		pathID := "movies"
		if i > 61 {
			pathID = "series"
		}
		record := model.StrmSyncRecord{
			Base:       model.Base{ID: fmt.Sprintf("sync-%03d", i), CreatedAt: created},
			SyncPathID: pathID, Status: model.StrmSyncRecordDone,
		}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	repos := repository.New(db)
	svc := &service.Container{Strm: service.NewStrmService(nil, zap.NewNop(), repos, nil)}
	router := gin.New()
	router.GET("/records", listStrmSyncRecordsHandler(svc))
	router.DELETE("/records/:id", deleteStrmSyncRecordHandler(svc))
	router.DELETE("/records", clearStrmSyncRecordsHandler(svc))

	request := func(method, target string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, target, nil))
		return w
	}
	page := func(query string, wantPage int, wantTotal int64, wantCount int) service.StrmSyncRecordPage {
		t.Helper()
		w := request(http.MethodGet, "/records?"+query)
		if w.Code != http.StatusOK {
			t.Fatalf("get page: %d %s", w.Code, w.Body.String())
		}
		var got service.StrmSyncRecordPage
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Page != wantPage || got.Total != wantTotal || got.PageSize != 10 || len(got.Items) != wantCount || got.Items == nil {
			t.Fatalf("page=%d total=%d size=%d items=%d, want page=%d total=%d items=%d", got.Page, got.Total, got.PageSize, len(got.Items), wantPage, wantTotal, wantCount)
		}
		return got
	}

	page("page=1", 1, 63, 10)
	seen := map[string]bool{}
	for p := 1; p <= 7; p++ {
		wantCount := 10
		if p == 7 {
			wantCount = 1
		}
		got := page(fmt.Sprintf("path_id=movies&page=%d&page_size=10", p), p, 61, wantCount)
		for i, record := range got.Items {
			wantID := fmt.Sprintf("sync-%03d", 61-(p-1)*10-i)
			if record.ID != wantID || seen[record.ID] {
				t.Fatalf("unexpected or repeated record %s, want %s", record.ID, wantID)
			}
			seen[record.ID] = true
		}
	}
	if len(seen) != 61 {
		t.Fatalf("only read %d records, want 61", len(seen))
	}
	page("path_id=missing&page=3", 1, 0, 0)
	page("path_id=movies&page=100", 7, 61, 1)

	// 旧调用仍接收最多 50 条的数组。
	w := request(http.MethodGet, "/records?path_id=movies")
	var legacy []model.StrmSyncRecord
	if err := json.Unmarshal(w.Body.Bytes(), &legacy); err != nil || w.Code != http.StatusOK || len(legacy) != 50 {
		t.Fatalf("legacy response: %d %s (%v)", w.Code, w.Body.String(), err)
	}
	for _, query := range []string{"page=-1", "page=bad", "page=1000001", "page_size=-1", "page_size=201"} {
		if w := request(http.MethodGet, "/records?"+query); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %s returned %d", query, w.Code)
		}
	}

	if w := request(http.MethodDelete, "/records/sync-001"); w.Code != http.StatusOK {
		t.Fatalf("delete last row: %d %s", w.Code, w.Body.String())
	}
	page("path_id=movies&page=7", 6, 60, 10)
	// 清空必须包含未显示页的数据。
	w = request(http.MethodDelete, "/records")
	var cleared struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cleared); err != nil || w.Code != http.StatusOK || cleared.Deleted != 62 {
		t.Fatalf("clear records: %d %s (%v)", w.Code, w.Body.String(), err)
	}
	page("page=6", 1, 0, 0)
}
