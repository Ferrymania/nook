package cloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"unicode"
)

var (
	ErrOpenListForbidden    = errors.New("OpenList 拒绝访问")
	errOpenListUnauthorized = errors.New("OpenList Token 无效或已过期，请在网盘账号中更新 Token")
)

const (
	openListCreatePermission int32 = 1 << 3 // OpenList 将新建目录与上传设为同一项权限。
	openListDeletePermission int32 = 1 << 7
)

type OpenListAccountFilesProvider interface {
	ListManagedFiles(ctx context.Context, dir string, page, pageSize int) (*AccountFilesPage, error)
	Mkdir(ctx context.Context, parentDir, name string) (*FileEntry, error)
	DeleteManagedFile(ctx context.Context, parentDir, name string) error
}

type AccountFilesPage struct {
	Entries         []FileEntry `json:"entries"`
	HasMore         bool        `json:"has_more"`
	CanCreateFolder bool        `json:"can_create_folder"`
	CanDelete       bool        `json:"can_delete"`
	ReadOnlyReason  string      `json:"read_only_reason,omitempty"`
}

type openListListHTTPError struct {
	status int
	error
}

func (p *openListProvider) ListManagedFiles(ctx context.Context, dir string, page, pageSize int) (*AccountFilesPage, error) {
	listing, err := p.fetchOpenListPage(ctx, dir, page, pageSize)
	if err != nil {
		var responseError *openListListHTTPError
		if errors.As(err, &responseError) {
			return nil, p.openListFileError("list", dir, responseError.status, "")
		}
		return nil, err
	}
	data := listing.Data
	result := &AccountFilesPage{Entries: make([]FileEntry, 0, len(data.Content))}
	if data.Total > 0 {
		result.HasMore = len(data.Content) > 0 && (page-1)*pageSize+len(data.Content) < data.Total
	} else {
		result.HasMore = len(data.Content) == pageSize
	}
	for _, item := range data.Content {
		name := item.Name
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) ||
			strings.IndexFunc(name, unicode.IsControl) >= 0 {
			continue
		}
		result.Entries = append(result.Entries, FileEntry{
			ID: path.Join(dir, name), Name: name, IsDir: item.IsDir, Size: item.Size,
		})
	}
	if data.Write == nil {
		result.ReadOnlyReason = "无法确认当前目录的写入权限"
		return result, nil
	}
	if !*data.Write {
		result.ReadOnlyReason = "当前 OpenList 目录为只读"
		return result, nil
	}
	user, err := p.fetchOpenListUser(ctx)
	if errors.Is(err, errOpenListUnauthorized) {
		return nil, err
	}
	if err != nil || user.Permission == nil {
		result.ReadOnlyReason = "无法读取 OpenList 账号权限"
		return result, nil
	}
	result.CanCreateFolder = *user.Permission&openListCreatePermission != 0 || data.WriteContentBypass
	result.CanDelete = *user.Permission&openListDeletePermission != 0
	return result, nil
}

func (p *openListProvider) DeleteManagedFile(ctx context.Context, parentDir, name string) error {
	return p.openListAPIPost(ctx, "/api/fs/remove", map[string]any{
		"dir":   parentDir,
		"names": []string{name},
	}, "remove")
}

func (p *openListProvider) openListFileError(action, target string, code int, message string) error {
	message = strings.TrimSpace(message)
	switch code {
	case http.StatusUnauthorized:
		return errOpenListUnauthorized
	case http.StatusForbidden:
		if message == "" {
			return ErrOpenListForbidden
		}
		return fmt.Errorf("%w：%s", ErrOpenListForbidden, message)
	default:
		if message == "" {
			return p.openListAPIStatusError(action, target, code)
		}
		return fmt.Errorf("%s: api %s %s failed: %s", p.name, action, target, message)
	}
}
