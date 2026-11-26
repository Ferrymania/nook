package service

import (
	"encoding/json"
	"errors"
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
	if err != nil || !first.HasMore || !first.CanCreateFolder || !first.CanDelete || len(first.Entries) != 100 || !first.Entries[0].IsDir {
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
	for _, id := range []string{"0", "00", "", "-1", "1,2", "31"} {
		if err := s.DeleteAccountFile(ctx, account.ID, "31", id); err == nil {
			t.Errorf("accepted 115 delete target %q", id)
		}
	}
	if err := s.repo.DB.Model(&account).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccountFile(ctx, account.ID, "31", id); err == nil {
		t.Fatal("disabled account could delete")
	}
}

func TestStrmAccountFilesRejectInvalidFolderNames(t *testing.T) {
	s := &StrmService{}
	for _, name := range []string{"", "..", "a/b", strings.Repeat("字", 256)} {
		if _, err := s.CreateAccountFolder(t.Context(), "account", "0", name); err == nil {
			t.Errorf("accepted folder name %q", name)
		}
	}
}

func TestOpenListAccountFiles(t *testing.T) {
	s := testStrmService(t)
	ctx := t.Context()
	config, err := s.strmAccountConfigJSON(map[string]string{"server": "https://openlist.example/mount", "token": "account-token"})
	if err != nil {
		t.Fatal(err)
	}
	account := model.StrmAccount{Name: "OpenList", Provider: model.StrmProviderOpenList, Enabled: true, Config: config}
	if err := s.repo.DB.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	const parent = "/影视 "
	const filename = " 电影+100%?.mkv "
	const allowed = `{"code":200}`
	const forbidden = `{"code":403,"message":"Permission denied"}`
	listData, userData := "", ""
	mutationResponse := allowed
	mkdirCalls, deleteCalls := 0, 0
	s.http = &http.Client{Transport: imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "account-token" {
			t.Fatal("missing OpenList account authorization")
		}
		switch r.URL.Path {
		case "/mount/api/me":
			return remoteDeleteResponse(userData), nil
		case "/mount/api/fs/list":
			var req struct {
				Path    string `json:"path"`
				Page    int    `json:"page"`
				PerPage int    `json:"per_page"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Path != parent || req.Page != 2 || req.PerPage != 100 {
				t.Fatalf("unexpected OpenList listing: %+v", req)
			}
			return remoteDeleteResponse(listData), nil
		case "/mount/api/fs/mkdir":
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req["path"] != parent+"/新目录" {
				t.Fatalf("unexpected OpenList mkdir %v: %v", req, err)
			}
			mkdirCalls++
			return remoteDeleteResponse(mutationResponse), nil
		case "/mount/api/fs/remove":
			var req struct {
				Dir   string   `json:"dir"`
				Names []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Dir != parent || len(req.Names) != 1 || req.Names[0] != filename {
				t.Fatalf("unexpected OpenList delete %+v: %v", req, err)
			}
			deleteCalls++
			return remoteDeleteResponse(mutationResponse), nil
		default:
			return nil, fmt.Errorf("unexpected OpenList request %s", r.URL.Path)
		}
	})}

	for _, tc := range []struct {
		name, directory, accountPermission string
		create, remove, readOnlyReason     bool
	}{
		{"read only directory", `,"write":false`, `"permission":-1`, false, false, true},
		{"missing directory permission", "", `"permission":-1`, false, false, true},
		{"read only account", `,"write":true`, `"permission":0`, false, false, false},
		{"create only", `,"write":true`, `"permission":8`, true, false, false},
		{"delete only", `,"write":true`, `"permission":128`, false, true, false},
		{"directory upload permission", `,"write":true,"write_content_bypass":true`, `"permission":0`, true, false, false},
		{"missing account permission", `,"write":true`, `"username":"demo"`, false, false, true},
		{"admin permissions", `,"write":true`, `"permission":-1`, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listData = fmt.Sprintf(`{"code":200,"data":{"total":102,"content":[{"name":%q}]%s}}`, filename, tc.directory)
			userData = fmt.Sprintf(`{"code":200,"data":{%s}}`, tc.accountPermission)
			files, err := s.ListAccountFiles(ctx, account.ID, parent, 2)
			if err != nil {
				t.Fatal(err)
			}
			if files.CanCreateFolder != tc.create || files.CanDelete != tc.remove || (files.ReadOnlyReason != "") != tc.readOnlyReason {
				t.Fatalf("unexpected permissions %+v", files)
			}
			if !files.HasMore || len(files.Entries) != 1 || files.Entries[0].ID != parent+"/"+filename {
				t.Fatalf("unexpected page %+v", files)
			}
		})
	}
	id, err := s.CreateAccountFolder(ctx, account.ID, parent, "新目录")
	if err != nil || id != parent+"/新目录" {
		t.Fatalf("created folder = %q, %v", id, err)
	}
	if err := s.DeleteAccountFile(ctx, account.ID, parent, filename); err != nil {
		t.Fatal(err)
	}
	mutationResponse = forbidden
	if _, err := s.CreateAccountFolder(ctx, account.ID, parent, "新目录"); !errors.Is(err, cloud.ErrOpenListForbidden) {
		t.Fatalf("OpenList mkdir rejection: %v", err)
	}
	if err := s.DeleteAccountFile(ctx, account.ID, parent, filename); !errors.Is(err, cloud.ErrOpenListForbidden) {
		t.Fatalf("OpenList delete rejection: %v", err)
	}
	for _, name := range []string{"..", "../sibling", "/folder", `a\b`} {
		if err := s.DeleteAccountFile(ctx, account.ID, parent, name); err == nil {
			t.Errorf("accepted OpenList delete target %q", name)
		}
	}
	if err := s.DeleteAccountFile(ctx, account.ID, "/safe/../elsewhere", filename); err == nil {
		t.Error("accepted a non-canonical OpenList parent")
	}
	if mkdirCalls != 2 || deleteCalls != 2 {
		t.Fatalf("unexpected count of mkdir and delete calls: %d, %d", mkdirCalls, deleteCalls)
	}
	listData = `{"code":401,"message":"token is expired"}`
	if _, err := s.ListAccountFiles(ctx, account.ID, parent, 2); err == nil || !strings.Contains(err.Error(), "Token") || errors.Is(err, cloud.ErrOpenListForbidden) {
		t.Fatalf("expired OpenList token: %v", err)
	}
}
