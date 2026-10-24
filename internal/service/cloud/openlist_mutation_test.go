package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenListMutableProviderUsesWebDAVWithoutAPICredentials(t *testing.T) {
	var mkcolSeen bool
	var destinations []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "MKCOL" && r.URL.Path == "/dav/TV":
			mkcolSeen = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == "MOVE" && r.URL.Path == "/dav/TV":
			destinations = append(destinations, r.Header.Get("Destination"))
			if r.Header.Get("Overwrite") != "F" {
				t.Fatalf("Overwrite = %q, want F", r.Header.Get("Overwrite"))
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == "MOVE" && r.URL.Path == "/dav/Inbox/Movie.mkv":
			destinations = append(destinations, r.Header.Get("Destination"))
			if r.Header.Get("Overwrite") != "F" {
				t.Fatalf("Overwrite = %q, want F", r.Header.Get("Overwrite"))
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/dav"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	mutable, ok := p.(MutableProvider)
	if !ok {
		t.Fatal("openlist should support mutable provider")
	}
	if _, err := mutable.Mkdir(context.Background(), "", "TV"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := mutable.Rename(context.Background(), "/TV", "电视剧"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	moved, err := mutable.(MovableProvider).Move(context.Background(), "/Inbox/Movie.mkv", "/电影/欧美电影/Movie (2026)", "Movie (2026).mkv")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !mkcolSeen || len(destinations) != 2 {
		t.Fatalf("mkcol=%v destinations=%#v, want mkdir and two MOVE calls", mkcolSeen, destinations)
	}
	if destinations[0] != srv.URL+"/dav/%E7%94%B5%E8%A7%86%E5%89%A7" {
		t.Fatalf("rename Destination = %q", destinations[0])
	}
	if destinations[1] != srv.URL+"/dav/%E7%94%B5%E5%BD%B1/%E6%AC%A7%E7%BE%8E%E7%94%B5%E5%BD%B1/Movie%20%282026%29/Movie%20%282026%29.mkv" {
		t.Fatalf("move Destination = %q", destinations[1])
	}
	if moved.ID != "/电影/欧美电影/Movie (2026)/Movie (2026).mkv" {
		t.Fatalf("moved entry = %#v", moved)
	}
}
