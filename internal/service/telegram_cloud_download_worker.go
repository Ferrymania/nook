package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud115"
)

const cloudDownloadMissingTimeout = 10 * time.Minute

var cloudDownloadPollDelays = [...]time.Duration{
	time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	time.Hour,
	6 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
}

func (s *TelegramService) telegramCloudDownloadLoop(ctx context.Context) {
	ticker := time.NewTicker(cloudDownloadWorkerTick)
	defer ticker.Stop()
	var nextCleanup time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.cloudDownloadWake:
		}
		cfg := s.config(ctx)
		if !cfg.Enabled || cfg.BotToken == "" || telegramPrivateAdminID(cfg.AdminID) == 0 {
			continue
		}
		now := time.Now()
		if now.After(nextCleanup) {
			if err := s.repo.TelegramCloudDownload.DeleteExpiredDrafts(ctx, now.Add(-cloudDownloadDraftLifetime)); err != nil {
				s.warn("clean expired cloud download drafts failed", "admin", err)
			}
			nextCleanup = now.Add(time.Hour)
		}
		s.runCloudDownloads(ctx, cfg.BotToken, telegramPrivateAdminID(cfg.AdminID), now)
	}
}

func (s *TelegramService) runCloudDownloads(ctx context.Context, token string, chatID int64, now time.Time) {
	tasks, err := s.repo.TelegramCloudDownload.Due(ctx, cloudDownloadBotKey(token), chatID, now)
	if err != nil {
		s.warn("load cloud download tasks failed", "admin", err)
		return
	}
	pages := make(map[string]*cloudDownloadTaskPages)
	for _, task := range tasks {
		if ctx.Err() != nil || !s.adminControlAllowed(ctx, token, telegramChat{ID: chatID, Type: "private"}, chatID) {
			return
		}
		jobCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		err := s.runCloudDownload(jobCtx, token, &task, now, pages)
		cancel()
		if err != nil && ctx.Err() == nil {
			s.warn("process cloud download failed", "admin", err)
		}
	}
}

func (s *TelegramService) runCloudDownload(ctx context.Context, token string, task *model.TelegramCloudDownload, now time.Time, pages map[string]*cloudDownloadTaskPages) error {
	delay := cloudDownloadWorkerTick
	switch task.Status {
	case model.CloudDownloadQueued:
		delay = cloudDownloadPollDelays[0]
	case model.CloudDownloadSubmitting, model.CloudDownloadTracking:
		delay = cloudDownloadPollDelays[len(cloudDownloadPollDelays)-1]
		for _, interval := range cloudDownloadPollDelays[1:] {
			if int(interval/time.Second) > task.PollDelaySeconds {
				delay = interval
				break
			}
		}
	}
	// Store the next check before querying 115 so retries and restarts use the same schedule.
	checkAfter := now.Add(delay)
	updated, err := s.repo.TelegramCloudDownload.Update(ctx, task, map[string]any{
		"check_after": checkAfter, "poll_delay_seconds": int(delay / time.Second),
	})
	if err != nil || !updated {
		return err
	}
	if task.Status == model.CloudDownloadQueued && !task.Reported {
		s.reportCloudDownload(ctx, token, task.ID, now)
	}
	switch task.Status {
	case model.CloudDownloadQueued:
		err = s.submitCloudDownload(ctx, task, now)
	case model.CloudDownloadSubmitting, model.CloudDownloadTracking:
		err = s.trackCloudDownload(ctx, task, now, pages)
	}
	if err != nil {
		return err
	}
	s.reportCloudDownload(ctx, token, task.ID, now)
	return nil
}

func (s *TelegramService) reportCloudDownload(ctx context.Context, token, taskID string, now time.Time) {
	// Navigation and progress share the same message. Read the latest page and
	// results after the previous edit finishes so an older edit cannot arrive last.
	s.cloudDownloadReportMu.Lock()
	defer s.cloudDownloadReportMu.Unlock()
	task, err := s.repo.TelegramCloudDownload.Find(ctx, taskID)
	if err != nil {
		s.warn("load cloud download notification failed", "admin", err)
		return
	}
	if task == nil || task.Reported {
		return
	}
	if !s.adminControlAllowed(ctx, token, telegramChat{ID: task.ChatID, Type: "private"}, task.ChatID) {
		return
	}
	text, keyboard := cloudDownloadMessage(task)
	fields := map[string]any{"reported": true}
	if err := s.editCloudDownloadMessage(ctx, token, task, text, keyboard); err != nil {
		var apiErr *telegramAPIError
		switch {
		case errors.As(err, &apiErr) && apiErr.RetryAfter > 0:
			if task.Status != model.CloudDownloadDone && task.Status != model.CloudDownloadFailed {
				return
			}
			retryAt := now.Add(time.Duration(apiErr.RetryAfter) * time.Second)
			if !retryAt.After(task.CheckAfter) {
				return
			}
			fields = map[string]any{"check_after": retryAt}
		case errors.As(err, &apiErr) && apiErr.Status == 400:
			// A deleted message or an already-applied edit cannot benefit from retries.
		default:
			s.warn("update Telegram cloud download message failed", "admin", err)
			return
		}
	}
	if err := s.repo.TelegramCloudDownload.UpdateReport(ctx, task, fields); err != nil {
		s.warn("save cloud download notification status failed", "admin", err)
	}
}

func (s *TelegramService) updateCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, fields map[string]any) error {
	_, err := s.repo.TelegramCloudDownload.Update(ctx, task, fields)
	return err
}

func (s *TelegramService) saveCloudDownloadItems(ctx context.Context, task *model.TelegramCloudDownload, items []model.TelegramCloudDownloadItem, now time.Time) (bool, error) {
	var completed, failed, tracking int
	for _, item := range items {
		switch item.Status {
		case model.CloudDownloadDone:
			completed++
		case model.CloudDownloadFailed:
			failed++
		case model.CloudDownloadTracking:
			tracking++
		}
	}
	status := model.CloudDownloadSubmitting
	switch {
	case failed == len(items):
		status = model.CloudDownloadFailed
	case completed+failed == len(items):
		status = model.CloudDownloadDone
	case tracking > 0:
		status = model.CloudDownloadTracking
	}

	data, err := json.Marshal(items)
	if err != nil {
		return false, err
	}
	fields := map[string]any{
		"status": status, "items": string(data), "encrypted_link": "", "detail": "", "reported": false,
	}
	if task.Status == model.CloudDownloadQueued && status == model.CloudDownloadSubmitting {
		fields["submitted_at"] = now
	}
	if status == model.CloudDownloadDone || status == model.CloudDownloadFailed {
		fields["check_after"] = now.Add(cloudDownloadWorkerTick)
	}
	return s.repo.TelegramCloudDownload.Update(ctx, task, fields)
}

func failCloudDownloadItem(item *model.TelegramCloudDownloadItem, detail string) {
	item.Status, item.Detail, item.EncryptedLink = model.CloudDownloadFailed, detail, ""
}

func (s *TelegramService) failCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, now time.Time, detail string) error {
	items := cloudDownloadItems(task)
	for i := range items {
		failCloudDownloadItem(&items[i], detail)
	}
	_, err := s.saveCloudDownloadItems(ctx, task, items, now)
	return err
}

func (s *TelegramService) submitCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, now time.Time) error {
	if !valid115ID(task.StagingFolderID) || task.StagingFolderID == "0" {
		return s.failCloudDownload(ctx, task, now, "请为该账号选择独立的云下载暂存目录。")
	}
	provider, err := s.strm.accountFilesProvider(ctx, task.AccountID, task.StagingFolderID)
	if err != nil {
		return s.failCloudDownload(ctx, task, now, "115 账号不可用，请检查账号和开放平台授权。")
	}
	if err := s.checkCloudDownloadStaging(ctx, task, provider.OpenClient()); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.failCloudDownload(ctx, task, now, err.Error())
	}
	items := cloudDownloadItems(task)
	links := make([]string, len(items))
	for i := range items {
		link, err := s.strm.crypto.Decrypt(items[i].EncryptedLink)
		if err != nil || !validCloudDownloadED2K(link) {
			return s.failCloudDownload(ctx, task, now, "无法读取已保存的 ED2K 链接，请重新发送 /download。")
		}
		links[i] = link
		items[i].Status, items[i].Progress = model.CloudDownloadSubmitting, -1
	}
	// Persist the submission before making the POST. A lost response is
	// reconciled against the task list, so a restart never resends a batch.
	claimed, err := s.saveCloudDownloadItems(ctx, task, items, now)
	if err != nil || !claimed {
		return err
	}
	task.Status = model.CloudDownloadSubmitting
	task.Items = items
	results, err := provider.OpenClient().AddOfflineLinks(ctx, links, task.StagingFolderID)
	var apiErr *cloud115.OpenAPIError
	if errors.As(err, &apiErr) {
		return s.failCloudDownload(ctx, task, now, cloudDownloadProviderError(apiErr, links...))
	}
	if err != nil {
		return nil
	}

	// 115 returns each submitted URL. Correlate by the ED2K file hash to
	// tolerate normalized URLs and different response order.
	byKey := make(map[string]cloud115.OfflineLinkResult, len(results))
	ambiguous := make(map[string]bool)
	for _, result := range results {
		var key string
		url := strings.TrimSpace(result.URL)
		switch {
		case url == "" && len(links) == 1 && len(results) == 1:
			key = cloudDownloadLinkKey(links[0])
		case validCloudDownloadED2K(url):
			key = cloudDownloadLinkKey(url)
		default:
			continue
		}
		if _, exists := byKey[key]; exists {
			ambiguous[key] = true
		}
		byKey[key] = result
	}
	for i, link := range links {
		key := cloudDownloadLinkKey(link)
		result, ok := byKey[key]
		if !ok || ambiguous[key] {
			continue
		}
		if !result.Accepted {
			detail := cloudDownloadProviderError(cloud115.NewOpenAPIError(result.Code, result.Message), append(links, result.URL)...)
			failCloudDownloadItem(&items[i], detail)
		} else if id := strings.TrimSpace(result.TaskID); id != "" {
			items[i].Status, items[i].RemoteTaskID, items[i].EncryptedLink = model.CloudDownloadTracking, id, ""
		}
	}
	_, err = s.saveCloudDownloadItems(ctx, task, items, now)
	return err
}

// Fetch each page at most once per 115 account in one polling round.
type cloudDownloadTaskPages struct {
	pages []cloud115.OfflineTaskPage
	err   error
}

func (p *cloudDownloadTaskPages) find(ctx context.Context, client *cloud115.OpenClient, match func(cloud115.OfflineTask) bool) (*cloud115.OfflineTask, error) {
	for page := 1; page <= 25; page++ {
		if page > len(p.pages) {
			if p.err != nil {
				return nil, p.err
			}
			data, err := client.ListOfflineTasks(ctx, page)
			if err != nil {
				p.err = err
				return nil, err
			}
			p.pages = append(p.pages, data)
		}
		data := p.pages[page-1]
		for _, remote := range data.Tasks {
			if match(remote) {
				return &remote, nil
			}
		}
		if page >= data.PageCount {
			return nil, nil
		}
	}
	return nil, errors.New("115 云下载列表超出单次查询范围")
}

func (s *TelegramService) trackCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, now time.Time, pages map[string]*cloudDownloadTaskPages) error {
	provider, err := s.strm.accountFilesProvider(ctx, task.AccountID, task.StagingFolderID)
	if err != nil {
		return s.updateCloudDownloadNotice(ctx, task, "暂时无法查询 115，请检查网盘账号和授权。")
	}
	if pages[task.AccountID] == nil {
		pages[task.AccountID] = &cloudDownloadTaskPages{}
	}
	items := cloudDownloadItems(task)
	changed := task.Detail != ""
	for i := range items {
		if items[i].Status == model.CloudDownloadDone || items[i].Status == model.CloudDownloadFailed {
			continue
		}
		item := s.checkCloudDownloadItem(ctx, task, items[i], now, pages[task.AccountID], provider.OpenClient())
		if item != items[i] {
			items[i] = item
			changed = true
		}
	}
	if !changed {
		return nil
	}
	_, err = s.saveCloudDownloadItems(ctx, task, items, now)
	return err
}

func (s *TelegramService) checkCloudDownloadItem(ctx context.Context, task *model.TelegramCloudDownload, item model.TelegramCloudDownloadItem, now time.Time, pages *cloudDownloadTaskPages, client *cloud115.OpenClient) model.TelegramCloudDownloadItem {
	var link, linkKey string
	if item.RemoteTaskID == "" {
		var err error
		link, err = s.strm.crypto.Decrypt(item.EncryptedLink)
		if err != nil || !validCloudDownloadED2K(link) || task.SubmittedAt == nil {
			failCloudDownloadItem(&item, "提交结果无法确认，请在 115 中查看。")
			return item
		}
		linkKey = cloudDownloadLinkKey(link)
	}
	remote, err := pages.find(ctx, client, func(remote cloud115.OfflineTask) bool {
		if remote.ID == "" {
			return false
		}
		if item.RemoteTaskID != "" {
			return strings.EqualFold(item.RemoteTaskID, remote.ID)
		}
		if remote.FolderID != task.StagingFolderID || !validCloudDownloadED2K(remote.URL) {
			return false
		}
		addedAt, err := remote.AddedAt.Int64()
		return err == nil && cloudDownloadLinkKey(remote.URL) == linkKey &&
			addedAt >= task.SubmittedAt.Add(-time.Minute).Unix()
	})
	if err != nil {
		item.Detail = "暂时无法查询 115 云下载进度，将继续重试。"
		var apiErr *cloud115.OpenAPIError
		if errors.As(err, &apiErr) {
			item.Detail = cloudDownloadProviderError(apiErr, link)
		}
		return item
	}
	if remote == nil {
		if item.MissingSince == nil {
			item.MissingSince = &now
		} else if now.Sub(*item.MissingSince) > cloudDownloadMissingTimeout {
			detail := "连续查询不到此任务，请在 115 云下载列表查看。"
			if item.Status == model.CloudDownloadSubmitting {
				detail = "未能确认 115 已接受提交，请在 115 中核对后再提交。"
			}
			failCloudDownloadItem(&item, detail)
		}
		return item
	}

	item.Status, item.RemoteTaskID, item.EncryptedLink = model.CloudDownloadTracking, remote.ID, ""
	item.MissingSince, item.Detail = nil, ""
	status, err := remote.Status.Int64()
	if err != nil {
		item.Detail = "115 暂未返回可识别的任务状态。"
		return item
	}
	item.Progress = -1
	switch status {
	case cloud115.OfflineTaskFailed:
		failCloudDownloadItem(&item, "115 报告下载失败，可在 115 云下载列表查看详情。")
	case cloud115.OfflineTaskDownloading:
		progress, _ := remote.Progress.Float64()
		item.Progress = max(0, min(100, int(progress)))
	case cloud115.OfflineTaskComplete:
		if remote.FolderID != "" && remote.FolderID != task.StagingFolderID {
			failCloudDownloadItem(&item, "115 报告的保存目录与暂存目录不一致，请在网盘中核对文件。")
		} else {
			item.Status, item.Progress = model.CloudDownloadDone, 100
		}
	}
	return item
}

func (s *TelegramService) checkCloudDownloadStaging(ctx context.Context, task *model.TelegramCloudDownload, client *cloud115.OpenClient) error {
	dir, err := client.GetFsDetailByCid(ctx, task.StagingFolderID)
	if err != nil || dir == nil || dir.FileId != task.StagingFolderID || dir.Category != cloud115.TypeDir {
		return errors.New("无法读取 115 暂存目录，请检查目录和账号授权。")
	}
	ancestors := make(map[string]bool, len(dir.Paths)+2)
	ancestors["0"], ancestors[task.StagingFolderID] = true, true
	for _, part := range dir.Paths {
		ancestors[part.FileId] = true
	}
	paths, err := s.strm.ListSyncPaths(ctx)
	if err != nil {
		return errors.New("无法读取 STRM 同步目录，请稍后重试。")
	}
	for _, path := range paths {
		if path.Enabled && path.Provider == model.StrmProvider115 && path.AccountID == task.AccountID && ancestors[path.RemotePath] {
			return errors.New("暂存目录位于 STRM 同步范围内，请为该账号选择同步范围之外的目录。")
		}
	}
	return nil
}

func cloudDownloadProviderError(err *cloud115.OpenAPIError, links ...string) string {
	detail := err.Error()
	for _, link := range links {
		if link != "" {
			detail = strings.ReplaceAll(detail, link, "[链接]")
		}
	}
	return redactSensitiveError(errors.New(detail)).Error()
}

func (s *TelegramService) updateCloudDownloadNotice(ctx context.Context, task *model.TelegramCloudDownload, detail string) error {
	if task.Detail == detail {
		return nil
	}
	return s.updateCloudDownload(ctx, task, map[string]any{"detail": detail, "reported": false})
}
