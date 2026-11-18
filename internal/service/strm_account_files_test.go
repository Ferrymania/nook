package service

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

func TestStrmAccountFiles(t *testing.T) {
	s := testStrmService(t)
	ctx := t.Context()
	account := model.StrmAccount{Name: "文件管理", Provider: model.StrmProvider115, Enabled: true}
	if err := s.repo.DB.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	provider := cloud.NewOpenAPI115("app", "account-token", "")
	s.provider115Cache[account.ID] = provider
	const fileID = "3073323042143855813"
	created, deleted := false, false
	provider.OpenClient().HTTP = &http.Client{Transport: imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer account-token" {
			t.Fatal("missing account authorization")
		}
		switch r.URL.Path {
		case "/open/ufile/files":
			q := r.URL.Query()
			if q.Get("cid") != "31" || q.Get("limit") != "101" || q.Get("cur") != "1" {
				t.Fatalf("unexpected listing query %v", q)
			}
			switch q.Get("offset") {
			case "":
				var rows []string
				for i := 1; i <= 101; i++ {
					rows = append(rows, fmt.Sprintf(`{"fid":"%d","fc":"0","fn":"Folder %d"}`, i, i))
				}
				return remoteDeleteResponse(`{"state":true,"data":[` + strings.Join(rows, ",") + `]}`), nil
			case "100":
				return remoteDeleteResponse(`{"state":true,"data":[{"fid":"101","fc":"1","fn":"Movie.mkv"}]}`), nil
			}
		case "/open/folder/add":
			if r.PostFormValue("pid") != "31" || r.PostFormValue("file_name") != "新目录" || created {
				t.Fatal("unexpected folder creation")
			}
			created = true
			return remoteDeleteResponse(`{"state":true,"data":{"file_id":"` + fileID + `","file_name":"新目录"}}`), nil
		case "/open/ufile/delete":
			if r.PostFormValue("file_ids") != fileID || r.PostFormValue("parent_id") != "31" || deleted {
				t.Fatal("unexpected deletion")
			}
			deleted = true
			return remoteDeleteResponse(`{"state":true,"data":[]}`), nil
		}
		return nil, fmt.Errorf("unexpected request %s", r.URL)
	})}
	first, err := s.ListAccountFiles(ctx, account.ID, "31", 1)
	if err != nil || !first.HasMore || len(first.Entries) != 100 || !first.Entries[0].IsDir {
		t.Fatalf("first listing: %v, %v", first, err)
	}
	last, err := s.ListAccountFiles(ctx, account.ID, "31", 2)
	if err != nil || last.HasMore || len(last.Entries) != 1 || last.Entries[0].ID != "101" || last.Entries[0].IsDir {
		t.Fatalf("last listing: %v, %v", last, err)
	}
	id, err := s.CreateAccountFolder(ctx, account.ID, "31", "新目录")
	if err != nil || id != fileID || !created {
		t.Fatalf("folder: %s, %v", id, err)
	}
	if err := s.DeleteAccountFile(ctx, account.ID, "31", id); err != nil || !deleted {
		t.Fatalf("delete: %v", err)
	}
	if err := s.repo.DB.Model(&account).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccountFile(ctx, account.ID, "31", id); err == nil {
		t.Fatal("disabled account could delete")
	}
}

func TestStrmAccountFilesRejectInvalidTargets(t *testing.T) {
	s := &StrmService{}
	for _, id := range []string{"0", "00", "", "-1", "1,2", "31"} {
		if err := s.DeleteAccountFile(t.Context(), "account", "31", id); err == nil {
			t.Errorf("accepted delete target %q", id)
		}
	}
	for _, name := range []string{"", "..", "a/b", strings.Repeat("字", 256)} {
		if _, err := s.CreateAccountFolder(t.Context(), "account", "0", name); err == nil {
			t.Errorf("accepted folder name %q", name)
		}
	}
}
