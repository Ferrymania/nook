package service

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

func TestRemoteMediaDeleteRequiresMatchingPreview(t *testing.T) {
	svc := testStrmService(t)
	db := svc.repo.DB
	if err := db.AutoMigrate(&model.Media{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	account := model.StrmAccount{Base: model.Base{ID: "account"}, Name: "115 test", Provider: model.StrmProvider115, Enabled: true}
	path := model.StrmSyncPath{AccountID: account.ID, Provider: account.Provider, RemotePath: "123", LocalPath: root, Enabled: true}
	const link = "https://nook.example/api/strm/play/cloud115/video.mkv?acct=account&pickcode=pick"
	episode := model.Media{Base: model.Base{ID: "episode"}, Title: "Episode", Path: filepath.Join(root, "Episode.strm"), STRMURL: link}
	openList := model.Media{Base: model.Base{ID: "openlist"}, Title: "OpenList", Path: filepath.Join(root, "OpenList.strm"),
		STRMURL: "https://nook.example/api/strm/play/openlist/video.mkv?acct=other&ref=%2Fother.mkv"}
	for _, row := range []any{&account, &path, &episode, &openList} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, episode.Path, link)
	writeFile(t, nfoPath(episode.Path), "episode NFO")
	writeFile(t, openList.Path, openList.STRMURL)

	provider := cloud.NewOpenAPI115("app", "token", "")
	svc.provider115Cache[account.ID] = provider
	fileID := "456"
	present, deletes := true, 0
	provider.OpenClient().HTTP = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/open/ufile/files":
			if req.URL.Query().Get("cid") != "123" {
				t.Fatalf("listed wrong directory: %s", req.URL)
			}
			if present {
				return remoteDeleteResponse(fmt.Sprintf(`{"state":true,"data":[{"fid":%q,"fc":"1","fn":"Episode.mkv","pc":"pick"}]}`, fileID)), nil
			}
			return remoteDeleteResponse(`{"state":true,"data":[]}`), nil
		case "/open/ufile/delete":
			if err := req.ParseForm(); err != nil || req.PostForm.Get("file_ids") != fileID || req.PostForm.Get("parent_id") != "123" {
				t.Fatalf("invalid deletion form: %v (%v)", req.PostForm, err)
			}
			deletes++
			present = false
			return remoteDeleteResponse(`{"state":true,"data":[]}`), nil
		default:
			t.Fatalf("unexpected upstream request: %s", req.URL)
			return nil, nil
		}
	})}

	ids := []string{episode.ID, openList.ID}
	preview, err := svc.PreviewRemoteMediaDelete(t.Context(), ids)
	if err != nil || len(preview) != 2 || preview[0].Status != "ready" || preview[1].Status != "skipped" || deletes != 0 {
		t.Fatalf("unexpected preview: %+v, err=%v, deletes=%d", preview, err, deletes)
	}
	media := NewMediaService(&config.Config{}, zap.NewNop(), svc.repo)
	fileID = "789"
	stale, err := svc.DeleteRemoteMedia(t.Context(), media, ids, map[string]string{episode.ID: preview[0].Fingerprint})
	if err != nil || len(stale) != 2 || stale[0].Status != "skipped" || deletes != 0 {
		t.Fatalf("stale confirmation was not rejected: %+v, err=%v, deletes=%d", stale, err, deletes)
	}
	fileID = "456"
	result, err := svc.DeleteRemoteMedia(t.Context(), media, ids, map[string]string{episode.ID: preview[0].Fingerprint})
	if err != nil || len(result) != 2 || result[0].Status != "deleted" || result[1].Status != "skipped" || deletes != 1 {
		t.Fatalf("unexpected deletion: %+v, err=%v, deletes=%d", result, err, deletes)
	}
	for _, name := range []string{episode.Path, nfoPath(episode.Path)} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be deleted: %v", name, err)
		}
	}
	if _, err := os.Stat(openList.Path); err != nil {
		t.Fatalf("OpenList STRM should remain: %v", err)
	}
	var count int64
	if err := db.Model(&model.Media{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("wrong number of remaining media: %d (%v)", count, err)
	}
}

func TestRemoteMediaDeleteUnconfirmedKeepsLocalFiles(t *testing.T) {
	provider := cloud.NewOpenAPI115("app", "token", "")
	path := filepath.Join(t.TempDir(), "Episode.strm")
	writeFile(t, path, "https://nook.example/api/strm/play/cloud115/video.mkv?acct=account&pickcode=pick")
	writeFile(t, nfoPath(path), "episode NFO")
	provider.OpenClient().HTTP = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/open/ufile/delete" {
			t.Fatalf("unexpected upstream request: %s", req.URL)
		}
		return remoteDeleteResponse(`{"state":false,"code":400,"message":"denied"}`), nil
	})}
	targets := []remoteDeleteTarget{{
		localPath: path, accountID: "account", pickCode: "pick", parentID: "123", fileID: "456", provider: provider,
	}}
	(&StrmService{}).deleteRemoteMediaBatch(t.Context(), nil, targets, []int{0})
	if targets[0].Status != "pending" {
		t.Fatalf("unexpected status: %+v", targets[0])
	}
	for _, name := range []string{path, nfoPath(path)} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected %s to be kept: %v", name, err)
		}
	}
}

func TestRemoteMediaDeleteKeepsNFOSharedWithAnotherVersion(t *testing.T) {
	dir := t.TempDir()
	strm := filepath.Join(dir, "Episode.mkv.strm")
	other := filepath.Join(dir, "Episode.mp4.strm")
	writeFile(t, strm, "115 STRM")
	writeFile(t, other, "OpenList STRM")
	writeFile(t, nfoPath(strm), "shared NFO")
	if err := deleteRemoteMediaLocalFiles(strm); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(strm); !os.IsNotExist(err) {
		t.Fatalf("expected selected STRM to be deleted: %v", err)
	}
	for _, name := range []string{other, nfoPath(strm)} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected %s to be kept: %v", name, err)
		}
	}
}

func remoteDeleteResponse(json string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(json))}
}
