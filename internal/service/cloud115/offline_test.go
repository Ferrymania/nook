package cloud115

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOfflineLinkCreatesTaskAndReportsPerLinkError(t *testing.T) {
	const link = "ed2k://|file|Movie 01.mkv|123|01234567890123456789012345678901|/"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/open/offline/add_task_urls" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if err := r.ParseMultipartForm(32 << 10); err != nil || r.Form.Get("urls") != link || r.Form.Get("wp_path_id") != "99" {
			t.Errorf("unexpected form: %v (%v)", r.Form, err)
		}
		attempts++
		body := `{"state":true,"data":[{"state":true,"info_hash":"task-1"}]}`
		if attempts == 2 {
			body = `{"state":true,"data":[{"state":false,"code":123,"message":"额度不足"}]}`
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	base := ProAPIBase
	ProAPIBase = server.URL
	t.Cleanup(func() { ProAPIBase = base })
	client := NewOpenClient("test", "test-token", "")
	client.executor = NewQueueExecutor(10, 200, 10000)
	if got, err := client.AddOfflineLink(t.Context(), link, "99"); err != nil || got != "task-1" {
		t.Fatalf("added task %q: %v", got, err)
	}
	_, err := client.AddOfflineLink(t.Context(), link, "99")
	var apiErr *OpenAPIError
	if !errors.As(err, &apiErr) || apiErr.Code != 123 || apiErr.Message != "额度不足" {
		t.Fatalf("failed task error = %v", err)
	}
}
