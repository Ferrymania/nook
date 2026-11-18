package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

type StrmAccountFilesPage struct {
	Entries []cloud.FileEntry `json:"entries"`
	HasMore bool              `json:"has_more"`
}

func valid115ID(id string) bool {
	n, err := strconv.ParseUint(id, 10, 64)
	return err == nil && strconv.FormatUint(n, 10) == id
}

func (s *StrmService) accountFilesProvider(ctx context.Context, accountID, parentID string) (cloud.OpenAPI115Provider, error) {
	if !valid115ID(parentID) {
		return nil, errors.New("无效的网盘目录 ID")
	}
	account, err := s.repo.StrmAccount.FindByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, errNotFoundOr(err, "网盘账号不存在")
	}
	if account.Provider != model.StrmProvider115 || !account.Enabled {
		return nil, errors.New("请选择已启用的 115 网盘账号")
	}
	provider, err := s.providerFor(ctx, account)
	if err != nil {
		return nil, err
	}
	p, ok := provider.(cloud.OpenAPI115Provider)
	if !ok || p.OpenClient() == nil || p.OpenClient().CurrentAccessToken() == "" {
		return nil, errors.New("请先授权 115 网盘账号")
	}
	return p, nil
}

func (s *StrmService) ListAccountFiles(ctx context.Context, accountID, parentID string, page int) (*StrmAccountFilesPage, error) {
	const pageSize = 100
	if page < 1 || page > math.MaxInt/pageSize {
		return nil, errors.New("页码无效")
	}
	p, err := s.accountFilesProvider(ctx, accountID, parentID)
	if err != nil {
		return nil, err
	}
	entries, err := p.ListPage(ctx, parentID, (page-1)*pageSize, pageSize+1)
	if err != nil {
		return nil, err
	}
	return &StrmAccountFilesPage{Entries: entries[:min(len(entries), pageSize)], HasMore: len(entries) > pageSize}, nil
}

func (s *StrmService) CreateAccountFolder(ctx context.Context, accountID, parentID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 || utf8.RuneCountInString(name) > 255 {
		return "", errors.New("请输入有效的文件夹名称，最多 255 个字符，不能包含路径分隔符")
	}
	p, err := s.accountFilesProvider(ctx, accountID, parentID)
	if err != nil {
		return "", err
	}
	return p.OpenClient().MkDir(ctx, parentID, name)
}

func (s *StrmService) DeleteAccountFile(ctx context.Context, accountID, parentID, fileID string) error {
	if !valid115ID(fileID) || fileID == "0" || fileID == parentID {
		return errors.New("请选择当前目录下的文件或文件夹")
	}
	p, err := s.accountFilesProvider(ctx, accountID, parentID)
	if err != nil {
		return err
	}
	return p.OpenClient().DeleteFiles(ctx, parentID, fileID)
}
