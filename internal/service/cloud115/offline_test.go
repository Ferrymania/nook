package cloud115

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOfflineLinksSubmitTogetherAndReturnPerLinkResults(t *testing.T) {
	const first = "ed2k://|file|Movie 01.mkv|123|01234567890123456789012345678901|/"
	const second = "ed2k://|file|Movie 02.mkv|456|11234567890123456789012345678901|/"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/open/offline/add_task_urls" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if err := r.ParseMultipartForm(32 << 10); err != nil || r.Form.Get("urls") != first+"\n"+second || r.Form.Get("wp_path_id") != "99" {
			t.Errorf("unexpected form: %v (%v)", r.Form, err)
		}
		requests.Add(1)
		body := fmt.Sprintf(`{"state":true,"data":[{"state":true,"url":%q,"info_hash":"task-1"},{"state":false,"url":%q,"code":123,"message":"额度不足"}]}`, first, second)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	base := ProAPIBase
	ProAPIBase = server.URL
	t.Cleanup(func() { ProAPIBase = base })
	client := NewOpenClient("test", "test-token", "")
	client.executor = NewQueueExecutor(10, 200, 10000)
	got, err := client.AddOfflineLinks(t.Context(), []string{first, second}, "99")
	if err != nil || len(got) != 2 || requests.Load() != 1 {
		t.Fatalf("expected two results from one request, got %+v (%v), requests=%d", got, err, requests.Load())
	}
	if got[0] != (OfflineLinkResult{Accepted: true, URL: first, TaskID: "task-1"}) ||
		got[1] != (OfflineLinkResult{URL: second, Code: 123, Message: "额度不足"}) {
		t.Fatalf("unexpected per-link results: %+v", got)
	}
}
