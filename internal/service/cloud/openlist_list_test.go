package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenListListUsesConfiguredToken(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/api/fs/list" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "configured-token" {
			t.Error("configured Token was not used")
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Path != "/" {
			t.Errorf("path = %q, want root", body.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"content":[{"name":"Movies","is_dir":true}],"total":1}}`))
	}))
	defer srv.Close()
	p, err := New(TypeOpenList, map[string]any{
		"server": srv.URL, "token": " configured-token ", "username": "legacy", "password": "ignored",
	}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		entries, err := p.List(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].ID != "/Movies" {
			t.Fatalf("entries = %#v", entries)
		}
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want just two listing requests", requests)
	}
}

func TestOpenListMutableProviderUsesAPI(t *testing.T) {
	var mkdirPath, renamePath, renameName, moveSrcDir, moveDstDir string
	var moveNames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/mkdir":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode mkdir body: %v", err)
			}
			mkdirPath = body["path"]
			if r.Header.Get("Authorization") != "alist-token" {
				t.Fatalf("mkdir Authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/rename":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rename body: %v", err)
			}
			renamePath = body["path"]
			renameName = body["name"]
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/move":
			var body struct {
				SrcDir string   `json:"src_dir"`
				DstDir string   `json:"dst_dir"`
				Names  []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode move body: %v", err)
			}
			moveSrcDir = body.SrcDir
			moveDstDir = body.DstDir
			moveNames = body.Names
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	mutable, ok := p.(MutableProvider)
	if !ok {
		t.Fatal("openlist should support mutable provider")
	}
	created, err := mutable.Mkdir(context.Background(), "/电视剧", "欧美剧")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if mkdirPath != "/电视剧/欧美剧" || created.ID != "/电视剧/欧美剧" || !created.IsDir {
		t.Fatalf("mkdir path=%q entry=%#v", mkdirPath, created)
	}
	renamed, err := mutable.Rename(context.Background(), "/电视剧/欧美剧", "美剧")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamePath != "/电视剧/欧美剧" || renameName != "美剧" || renamed.ID != "/电视剧/美剧" {
		t.Fatalf("rename path=%q name=%q entry=%#v", renamePath, renameName, renamed)
	}
	moved, err := mutable.(MovableProvider).Move(context.Background(), "/待整理/Show.S01E01.mkv", "/动漫/国漫/Show/Season 01", "Show - S01E01.mkv")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moveSrcDir != "/待整理" || moveDstDir != "/动漫/国漫/Show/Season 01" || len(moveNames) != 1 || moveNames[0] != "Show.S01E01.mkv" {
		t.Fatalf("move src=%q dst=%q names=%#v", moveSrcDir, moveDstDir, moveNames)
	}
	if renamePath != "/动漫/国漫/Show/Season 01/Show.S01E01.mkv" || renameName != "Show - S01E01.mkv" {
		t.Fatalf("post-move rename path=%q name=%q", renamePath, renameName)
	}
	if moved.ID != "/动漫/国漫/Show/Season 01/Show - S01E01.mkv" {
		t.Fatalf("moved entry = %#v", moved)
	}
}

func TestOpenListInvalidTokenDoesNotLoginRetryOrUseWebDAV(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/api/fs/list" {
					t.Errorf("unexpected fallback request: %s", r.URL.Path)
				}
				w.WriteHeader(status)
			}))
			defer srv.Close()
			p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "bad", "username": "legacy", "password": "ignored"}, srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "更换 OpenList Token") {
				t.Fatalf("unexpected test result: %v", err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want one without login or retry", requests)
			}
		})
	}
}

func TestOpenListURLForKeepsNonASCIIPathSingleEncoded(t *testing.T) {
	p := newOpenList(map[string]any{"url": "http://example.test:5244/dav/"}, nil)
	got := p.urlFor("/动画电影/爱宠大机密2 (2019) {tmdb-412117}")
	if strings.Contains(got, "%25E") {
		t.Fatalf("url is double-escaped: %s", got)
	}
	want := "http://example.test:5244/dav/%E5%8A%A8%E7%94%BB%E7%94%B5%E5%BD%B1/%E7%88%B1%E5%AE%A0%E5%A4%A7%E6%9C%BA%E5%AF%862%20%282019%29%20%7Btmdb-412117%7D"
	if got != want {
		t.Fatalf("url = %s, want %s", got, want)
	}
}

func TestOpenListFetchUsernameUsesToken(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"username", `{"code":200,"data":{"id":1,"username":"  家庭用户  "}}`, 200, "家庭用户"},
		{"expired token", `{"code":401,"message":"expired"}`, 401, ""},
		{"business failure", `{"code":401,"message":"expired"}`, 200, ""},
		{"missing username", `{"code":200,"data":{}}`, 200, ""},
		{"invalid response", `not-json`, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" || r.URL.Path != "/api/me" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "configured-token" {
					t.Error("user info did not use saved Token")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			p := newOpenList(map[string]any{"server": srv.URL, "token": "configured-token"}, srv.Client())
			username, err := p.FetchUsername(context.Background())
			if username != tc.want || (err == nil) != (tc.want != "") {
				t.Fatalf("username = %q, error = %v", username, err)
			}
			if requests != 1 {
				t.Errorf("requests = %d, want only user-info lookup", requests)
			}
		})
	}
}
