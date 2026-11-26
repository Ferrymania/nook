package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

func valid115ID(id string) bool {
	n, err := strconv.ParseUint(id, 10, 64)
	return err == nil && strconv.FormatUint(n, 10) == id
}

func validOpenListDirectory(dir string) bool {
	// OpenList 的列表接口将 /@s 开头的路径解释为分享链接。
	return strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, "/@s") &&
		dir == path.Clean(dir) && !strings.Contains(dir, "\\") &&
		strings.IndexFunc(dir, unicode.IsControl) < 0
}

func validAccountFileName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`) &&
		strings.IndexFunc(name, unicode.IsControl) < 0
}

func (s *StrmService) accountFilesProvider(ctx context.Context, accountID, parentID string) (cloud.OpenAPI115Provider, error) {
	if !valid115ID(parentID) {
		return nil, errors.New("无效的网盘目录 ID")
	}
	provider, err := s.managedAccountProvider(ctx, accountID)
	if err != nil {
		return nil, err
	}
	p, ok := provider.(cloud.OpenAPI115Provider)
	if !ok {
		return nil, errors.New("请选择已启用的 115 网盘账号")
	}
	return p, nil
}

func (s *StrmService) managedAccountProvider(ctx context.Context, accountID string) (cloud.Provider, error) {
	account, err := s.repo.StrmAccount.FindByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, errNotFoundOr(err, "网盘账号不存在")
	}
	if !account.Enabled || (account.Provider != model.StrmProvider115 && account.Provider != model.StrmProviderOpenList) {
		return nil, errors.New("请选择已启用的 115 或 OpenList 网盘账号")
	}
	provider, err := s.providerFor(ctx, account)
	if err != nil {
		return nil, err
	}
	if p, ok := provider.(cloud.OpenAPI115Provider); ok {
		if p.OpenClient() == nil || p.OpenClient().CurrentAccessToken() == "" {
			return nil, errors.New("请先授权 115 网盘账号")
		}
	}
	return provider, nil
}

func (s *StrmService) ListAccountFiles(ctx context.Context, accountID, parentID string, page int) (*cloud.AccountFilesPage, error) {
	const pageSize = 100
	if page < 1 || page > math.MaxInt/pageSize {
		return nil, errors.New("页码无效")
	}
	provider, err := s.managedAccountProvider(ctx, accountID)
	if err != nil {
		return nil, err
	}
	switch p := provider.(type) {
	case cloud.OpenListAccountFilesProvider:
		if parentID == "" {
			parentID = "/"
		}
		if !validOpenListDirectory(parentID) {
			return nil, errors.New("无效的 OpenList 目录路径")
		}
		return p.ListManagedFiles(ctx, parentID, page, pageSize)
	case cloud.OpenAPI115Provider:
		if parentID == "" {
			parentID = "0"
		}
		if !valid115ID(parentID) {
			return nil, errors.New("无效的网盘目录 ID")
		}
		entries, err := p.ListPage(ctx, parentID, (page-1)*pageSize, pageSize+1)
		if err != nil {
			return nil, err
		}
		return &cloud.AccountFilesPage{
			Entries: entries[:min(len(entries), pageSize)], HasMore: len(entries) > pageSize,
			CanCreateFolder: true, CanDelete: true,
		}, nil
	default:
		return nil, cloud.ErrUnsupported
	}
}

func (s *StrmService) CreateAccountFolder(ctx context.Context, accountID, parentID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validAccountFileName(name) || utf8.RuneCountInString(name) > 255 {
		return "", errors.New("请输入有效的文件夹名称，最多 255 个字符，不能包含路径分隔符")
	}
	provider, err := s.managedAccountProvider(ctx, accountID)
	if err != nil {
		return "", err
	}
	switch p := provider.(type) {
	case cloud.OpenListAccountFilesProvider:
		if !validOpenListDirectory(parentID) {
			return "", errors.New("无效的 OpenList 目录路径")
		}
		entry, err := p.Mkdir(ctx, parentID, name)
		if errors.Is(err, cloud.ErrOpenListForbidden) {
			return "", fmt.Errorf("没有在当前目录新建文件夹的权限：%w", err)
		}
		if err != nil {
			return "", err
		}
		return entry.ID, nil
	case cloud.OpenAPI115Provider:
		if !valid115ID(parentID) {
			return "", errors.New("无效的网盘目录 ID")
		}
		return p.OpenClient().MkDir(ctx, parentID, name)
	default:
		return "", cloud.ErrUnsupported
	}
}

func (s *StrmService) DeleteAccountFile(ctx context.Context, accountID, parentID, fileID string) error {
	if !validAccountFileName(fileID) {
		return errors.New("请选择当前目录下的文件或文件夹")
	}
	provider, err := s.managedAccountProvider(ctx, accountID)
	if err != nil {
		return err
	}
	switch p := provider.(type) {
	case cloud.OpenListAccountFilesProvider:
		if !validOpenListDirectory(parentID) {
			return errors.New("无效的 OpenList 目录路径")
		}
		err := p.DeleteManagedFile(ctx, parentID, fileID)
		if errors.Is(err, cloud.ErrOpenListForbidden) {
			return fmt.Errorf("没有在当前目录删除文件的权限：%w", err)
		}
		return err
	case cloud.OpenAPI115Provider:
		if !valid115ID(fileID) || fileID == "0" || fileID == parentID {
			return errors.New("请选择当前目录下的文件或文件夹")
		}
		if !valid115ID(parentID) {
			return errors.New("无效的网盘目录 ID")
		}
		return p.OpenClient().DeleteFiles(ctx, parentID, fileID)
	default:
		return cloud.ErrUnsupported
	}
}
