package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

const maxRemoteDeleteSTRMSize = 16 << 10

type RemoteDeleteItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Account     string `json:"account,omitempty"`
	RemotePath  string `json:"remote_path,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

type remoteDeleteTarget struct {
	RemoteDeleteItem
	localPath, accountID, pickCode, parentID, fileID string
	provider                                         cloud.OpenAPI115Provider
}

type remoteDeleteDir struct {
	files []cloud.FileEntry
	err   error
}

func (s *StrmService) PreviewRemoteMediaDelete(ctx context.Context, ids []string) ([]RemoteDeleteItem, error) {
	targets, err := s.planRemoteMediaDelete(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]RemoteDeleteItem, 0, len(targets))
	for _, target := range targets {
		items = append(items, target.RemoteDeleteItem)
	}
	return items, nil
}

func (s *StrmService) planRemoteMediaDelete(ctx context.Context, ids []string) ([]remoteDeleteTarget, error) {
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).Select("id", "title", "path", "strm_url").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	mediaByID := make(map[string]model.Media, len(rows))
	for _, row := range rows {
		mediaByID[row.ID] = row
	}
	paths, err := s.ListSyncPaths(ctx)
	if err != nil {
		return nil, err
	}
	// One file can back several indexed STRM entries. Reject a shared target
	// even when all its references were selected in this request.
	var references []struct {
		ID, Path, STRMURL string
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("id", "path", "strm_url").Where("LOWER(path) LIKE ?", "%.strm").Find(&references).Error; err != nil {
		return nil, err
	}
	owners := make(map[string][]string)
	for _, ref := range references {
		_, pick := remoteDelete115Ref(ref.STRMURL)
		if pick != "" {
			owners[pick] = append(owners[pick], ref.ID)
		}
		// A sync may update the on-disk target before the index catches up.
		if raw, err := readRemoteDeleteSTRM(ref.Path); err == nil {
			if _, current := remoteDelete115Ref(raw); current != "" && current != pick {
				owners[current] = append(owners[current], ref.ID)
			}
		}
	}

	dirs := make(map[[2]string]remoteDeleteDir)
	targets := make([]remoteDeleteTarget, 0, len(ids))
	for _, id := range ids {
		row := mediaByID[id]
		target := remoteDeleteTarget{RemoteDeleteItem: RemoteDeleteItem{ID: id, Title: row.Title, Status: "skipped"}}
		if row.ID == "" {
			target.Message = "媒体条目已不存在"
		} else if err := s.resolveRemoteDeleteTarget(ctx, row, paths, owners, dirs, &target); err != nil {
			target.Message = err.Error()
		} else {
			target.Status = "ready"
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func remoteDelete115Ref(raw string) (account, pick string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	path := strings.ToLower(u.Path)
	if !strings.HasPrefix(path, "/api/strm/play/"+model.StrmProvider115+"/video.") &&
		path != "/api/cloud/play/"+model.StrmProvider115 {
		return "", ""
	}
	q := u.Query()
	if len(q["acct"]) != 1 || len(q["pickcode"]) != 1 {
		return "", ""
	}
	return strings.TrimSpace(q.Get("acct")), strings.TrimSpace(q.Get("pickcode"))
}

func (s *StrmService) resolveRemoteDeleteTarget(ctx context.Context, row model.Media, paths []model.StrmSyncPath, owners map[string][]string, dirs map[[2]string]remoteDeleteDir, target *remoteDeleteTarget) error {
	if !strings.EqualFold(filepath.Ext(row.Path), ".strm") {
		return errors.New("仅支持 115 STRM 媒体")
	}
	raw, err := readRemoteDeleteSTRM(row.Path)
	if err != nil {
		return errors.New("无法读取本地 STRM 文件")
	}
	accountID, pick := remoteDelete115Ref(raw)
	if accountID == "" || pick == "" {
		return errors.New("来源不是 115 或缺少原片标识")
	}
	indexedAccount, indexedPick := remoteDelete115Ref(row.STRMURL)
	if row.STRMURL != "" && (indexedAccount != accountID || indexedPick != pick) {
		return errors.New("STRM 链接与媒体库记录不同，请先扫描媒体库")
	}
	for _, owner := range owners[pick] {
		if owner != row.ID {
			return errors.New("原片仍被其他媒体条目引用")
		}
	}

	localPath, err := filepath.EvalSymlinks(row.Path)
	if err != nil {
		return errors.New("无法核验 STRM 文件路径")
	}
	var syncPath *model.StrmSyncPath
	var relative string
	for i := range paths {
		path := &paths[i]
		if path.Provider != model.StrmProvider115 || path.AccountID != accountID || path.LocalPath == "" {
			continue
		}
		root, err := filepath.EvalSymlinks(path.LocalPath)
		if err != nil || !pathWithin(localPath, root) {
			continue
		}
		if syncPath != nil {
			return errors.New("STRM 文件匹配多个同步目录")
		}
		relative, err = filepath.Rel(root, localPath)
		if err != nil {
			return errors.New("无法核验 STRM 所在目录")
		}
		syncPath = path
	}
	if syncPath == nil {
		return errors.New("未找到对应的 115 同步目录")
	}
	account, err := s.repo.StrmAccount.FindByID(ctx, accountID)
	if err != nil || account == nil || !account.Enabled || account.Provider != model.StrmProvider115 {
		return errors.New("115 账号不可用")
	}
	provider, err := s.providerFor(ctx, account)
	if err != nil {
		return errors.New("无法连接 115 账号")
	}
	open115, ok := provider.(cloud.OpenAPI115Provider)
	if !ok || open115.OpenClient() == nil {
		return errors.New("115 账号不支持删除")
	}

	parent := strings.TrimSpace(syncPath.RemotePath)
	if parent == "" {
		return errors.New("缺少 115 根目录 ID")
	}
	dir := filepath.Dir(relative)
	if dir != "." {
		for _, name := range strings.Split(filepath.ToSlash(dir), "/") {
			entries, err := remoteDeleteList(ctx, open115, accountID, parent, dirs)
			if err != nil {
				return err
			}
			var matches []cloud.FileEntry
			for _, entry := range entries {
				if entry.IsDir && entry.Name == name && entry.ID != "" {
					matches = append(matches, entry)
				}
			}
			if len(matches) != 1 {
				return errors.New("无法核实 115 原片所在目录")
			}
			parent = matches[0].ID
		}
	}
	entries, err := remoteDeleteList(ctx, open115, accountID, parent, dirs)
	if err != nil {
		return err
	}
	var matches []cloud.FileEntry
	for _, entry := range entries {
		if !entry.IsDir && entry.PickCode == pick && entry.ID != "" && entry.Name != "" {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 {
		return errors.New("无法唯一核实 115 原片")
	}
	file := matches[0]
	// An empty or nonnumeric ID must never reach the upstream delete endpoint.
	if strings.Trim(file.ID, "0123456789") != "" || strings.Trim(parent, "0123456789") != "" {
		return errors.New("115 返回了无效的文件或目录 ID")
	}
	target.Account = account.Name
	rootDisplay := syncPath.RemoteDisplayPath
	if rootDisplay == "" {
		rootDisplay = "目录 ID " + syncPath.RemotePath
	}
	target.RemotePath = filepath.ToSlash(filepath.Join(rootDisplay, dir, file.Name))
	target.localPath, target.accountID, target.pickCode = row.Path, accountID, pick
	target.parentID, target.fileID, target.provider = parent, file.ID, open115
	identity, _ := json.Marshal([]string{row.ID, row.Path, syncPath.ID, accountID, account.Name, pick, parent, file.ID, target.RemotePath})
	target.Fingerprint = fmt.Sprintf("%x", sha256.Sum256(identity))
	return nil
}

func remoteDeleteList(ctx context.Context, provider cloud.OpenAPI115Provider, account, parent string, dirs map[[2]string]remoteDeleteDir) ([]cloud.FileEntry, error) {
	key := [2]string{account, parent}
	if cached, ok := dirs[key]; ok {
		return cached.files, cached.err
	}
	files, err := provider.List(ctx, parent)
	if err != nil {
		err = fmt.Errorf("读取 115 目录失败：%w", err)
	}
	dirs[key] = remoteDeleteDir{files: files, err: err}
	return files, err
}

func (s *StrmService) DeleteRemoteMedia(ctx context.Context, media *MediaService, ids []string, approved map[string]string) ([]RemoteDeleteItem, error) {
	s.mu.Lock()
	if len(s.running) != 0 || s.deletingRemoteMedia {
		s.mu.Unlock()
		return nil, errors.New("有 STRM 同步或网盘删除正在进行，请结束后再试")
	}
	s.deletingRemoteMedia = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.deletingRemoteMedia = false
		s.mu.Unlock()
	}()

	targets, err := s.planRemoteMediaDelete(ctx, ids)
	if err != nil {
		return nil, err
	}
	type groupKey struct{ account, parent string }
	groups := make(map[groupKey][]int)
	var order []groupKey
	for i := range targets {
		target := &targets[i]
		if target.Status != "ready" {
			continue
		}
		if approved[target.ID] != target.Fingerprint {
			target.Status, target.Message = "skipped", "文件与确认清单不一致，请重新确认"
			continue
		}
		key := groupKey{target.accountID, target.parentID}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}

	for _, key := range order {
		for batch := range slices.Chunk(groups[key], 100) {
			s.deleteRemoteMediaBatch(ctx, media, targets, batch)
		}
	}
	items := make([]RemoteDeleteItem, 0, len(targets))
	for _, target := range targets {
		items = append(items, target.RemoteDeleteItem)
	}
	return items, nil
}

func (s *StrmService) deleteRemoteMediaBatch(ctx context.Context, media *MediaService, targets []remoteDeleteTarget, batch []int) {
	first := targets[batch[0]]
	pending := make(map[string]int, len(batch))
	fileIDs := make([]string, 0, len(batch))
	for _, i := range batch {
		target := &targets[i]
		if !target.matchesLocalLink() {
			target.Status, target.Message = "skipped", "本地 STRM 已变更"
			continue
		}
		fileIDs = append(fileIDs, target.fileID)
		pending[target.fileID] = i
		target.Status, target.Message = "pending", "尚未确认 115 删除结果，本地记录已保留"
	}
	if len(fileIDs) == 0 {
		return
	}
	if err := ctx.Err(); err != nil {
		for _, i := range pending {
			targets[i].Status, targets[i].Message = "failed", "请求已结束，未提交删除"
		}
		return
	}
	// A network error can follow an accepted mutation. Leave local state in
	// place when the API did not return an acceptance response.
	if err := first.provider.OpenClient().DeleteFiles(ctx, first.parentID, fileIDs...); err != nil {
		for _, i := range pending {
			targets[i].Message = "115 删除请求返回错误，本地记录已保留：" + err.Error()
		}
		return
	}
	for attempt := 0; attempt < 4 && len(pending) > 0 && ctx.Err() == nil; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
		entries, err := first.provider.List(ctx, first.parentID)
		if err != nil {
			continue
		}
		present := make(map[string]bool, len(entries))
		for _, entry := range entries {
			present[entry.ID] = true
		}
		for id, i := range pending {
			if present[id] {
				continue
			}
			target := &targets[i]
			target.Status, target.Message = "deleted", ""
			if !target.matchesLocalLink() {
				target.Status, target.Message = "local_failed", "115 原片已删除，本地 STRM 已变更，保留本地记录"
			} else if err := media.deleteMedia(ctx, target.ID, true, target.localPath); err != nil {
				target.Status, target.Message = "local_failed", "115 原片已删除，本地清理失败："+err.Error()
			}
			delete(pending, id)
		}
	}
}

func readRemoteDeleteSTRM(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxRemoteDeleteSTRMSize {
		return "", errors.New("无法核验本地 STRM 文件")
	}
	return readLocalSTRMTarget(path)
}

func (t remoteDeleteTarget) matchesLocalLink() bool {
	raw, err := readRemoteDeleteSTRM(t.localPath)
	if err != nil {
		return false
	}
	account, pick := remoteDelete115Ref(raw)
	return account == t.accountID && pick == t.pickCode
}
