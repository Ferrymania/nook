package service

import (
	"context"
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
	task.CheckAfter = checkAfter
	if task.Status == model.CloudDownloadQueued && !task.Reported {
		s.reportCloudDownload(ctx, token, task, now)
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
	latest, err := s.repo.TelegramCloudDownload.Find(ctx, task.ID)
	if err == nil && latest != nil && !latest.Reported {
		s.reportCloudDownload(ctx, token, latest, now)
	}
	return err
}

func (s *TelegramService) reportCloudDownload(ctx context.Context, token string, task *model.TelegramCloudDownload, now time.Time) {
	if !s.adminControlAllowed(ctx, token, telegramChat{ID: task.ChatID, Type: "private"}, task.ChatID) {
		return
	}
	text := cloudDownloadMessage(task)
	fields := map[string]any{"reported": true}
	if err := s.editCloudDownloadMessage(ctx, token, task, text, nil); err != nil {
		var apiErr *telegramAPIError
		switch {
		case errors.As(err, &apiErr) && apiErr.RetryAfter > 0:
			// A messaging retry must not bring the next 115 lookup forward.
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
	if _, err := s.repo.TelegramCloudDownload.Update(ctx, task, fields); err != nil {
		s.warn("save cloud download notification status failed", "admin", err)
	}
}

func (s *TelegramService) updateCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, fields map[string]any) error {
	_, err := s.repo.TelegramCloudDownload.Update(ctx, task, fields)
	return err
}

func (s *TelegramService) failCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, detail string) error {
	return s.updateCloudDownload(ctx, task, map[string]any{
		"status": model.CloudDownloadFailed, "detail": detail, "encrypted_link": "", "reported": false,
	})
}

func (s *TelegramService) submitCloudDownload(ctx context.Context, task *model.TelegramCloudDownload, now time.Time) error {
	if !valid115ID(task.StagingFolderID) || task.StagingFolderID == "0" {
		return s.failCloudDownload(ctx, task, "请为该账号选择独立的云下载暂存目录。")
	}
	provider, err := s.strm.accountFilesProvider(ctx, task.AccountID, task.StagingFolderID)
	if err != nil {
		return s.failCloudDownload(ctx, task, "115 账号不可用，请检查账号和开放平台授权。")
	}
	if err := s.checkCloudDownloadStaging(ctx, task, provider.OpenClient()); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.failCloudDownload(ctx, task, err.Error())
	}
	link, err := s.strm.crypto.Decrypt(task.EncryptedLink)
	if err != nil || link == "" {
		return s.failCloudDownload(ctx, task, "无法读取原始下载链接，请重新发送 /download。")
	}
	if !validCloudDownloadED2K(link) {
		return s.failCloudDownload(ctx, task, "云下载只接受 ED2K 单文件链接，请重新发送 /download。")
	}
	// Record intent before the POST. A crash or timeout is reconciled by
	// looking for the exact link and directory instead of repeating the POST.
	claimed, err := s.repo.TelegramCloudDownload.Update(ctx, task, map[string]any{
		"status": model.CloudDownloadSubmitting, "submitted_at": now, "reported": false,
	})
	if err != nil || !claimed {
		return err
	}
	task.Status = model.CloudDownloadSubmitting
	id, err := provider.OpenClient().AddOfflineLink(ctx, link, task.StagingFolderID)
	var apiErr *cloud115.OpenAPIError
	if errors.As(err, &apiErr) {
		return s.failCloudDownload(ctx, task, cloudDownloadProviderError(apiErr, link))
	}
	if err != nil {
		return nil
	}
	return s.updateCloudDownload(ctx, task, map[string]any{
		"status": model.CloudDownloadTracking, "remote_task_id": id, "encrypted_link": "", "progress": -1, "reported": false,
	})
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
		// A temporary account or database problem does not stop an accepted task.
		return s.updateCloudDownloadNotice(ctx, task, "暂时无法查询 115，请检查网盘账号和授权。")
	}
	if pages[task.AccountID] == nil {
		pages[task.AccountID] = &cloudDownloadTaskPages{}
	}
	link := ""
	if task.Status == model.CloudDownloadSubmitting {
		link, err = s.strm.crypto.Decrypt(task.EncryptedLink)
		if err != nil || link == "" || task.SubmittedAt == nil {
			return s.failCloudDownload(ctx, task, "提交结果无法确认，请在 115 中查看。")
		}
	}
	remote, err := pages[task.AccountID].find(ctx, provider.OpenClient(), func(remote cloud115.OfflineTask) bool {
		if remote.ID == "" {
			return false
		}
		if task.RemoteTaskID != "" {
			return strings.EqualFold(task.RemoteTaskID, remote.ID)
		}
		addedAt, err := remote.AddedAt.Int64()
		return err == nil && remote.URL == link && remote.FolderID == task.StagingFolderID &&
			addedAt >= task.SubmittedAt.Add(-time.Minute).Unix()
	})
	if err != nil {
		detail := "暂时无法查询 115 云下载进度，将继续重试。"
		var apiErr *cloud115.OpenAPIError
		if errors.As(err, &apiErr) {
			detail = cloudDownloadProviderError(apiErr, link)
		}
		return s.updateCloudDownloadNotice(ctx, task, detail)
	}
	if remote == nil {
		if task.MissingSince == nil {
			return s.updateCloudDownload(ctx, task, map[string]any{"missing_since": now})
		}
		if now.Sub(*task.MissingSince) > cloudDownloadMissingTimeout {
			detail := "连续查询不到此任务，请在 115 云下载列表查看。"
			if task.Status == model.CloudDownloadSubmitting {
				detail = "未能确认 115 已接受提交，请在 115 中核对后再提交。"
			}
			return s.failCloudDownload(ctx, task, detail)
		}
		return nil
	}
	status, err := remote.Status.Int64()
	if err != nil {
		return s.updateCloudDownloadNotice(ctx, task, "115 暂未返回可识别的任务状态。")
	}
	if status == cloud115.OfflineTaskFailed {
		return s.failCloudDownload(ctx, task, "115 报告下载失败，可在 115 云下载列表查看详情。")
	}
	next := model.CloudDownloadTracking
	progressValue, _ := remote.Progress.Float64()
	progress := -1
	if status == cloud115.OfflineTaskDownloading {
		progress = max(0, min(100, int(progressValue)))
	}
	if status == cloud115.OfflineTaskComplete {
		if remote.FolderID != "" && remote.FolderID != task.StagingFolderID {
			return s.failCloudDownload(ctx, task, "115 报告的保存目录与暂存目录不一致，请在网盘中核对文件。")
		}
		next, progress = model.CloudDownloadDone, 100
	}
	if next == task.Status && progress == task.Progress && task.MissingSince == nil && task.Detail == "" {
		return nil
	}
	return s.updateCloudDownload(ctx, task, map[string]any{
		"status": next, "remote_task_id": remote.ID, "encrypted_link": "", "progress": progress,
		"missing_since": nil, "detail": "", "reported": false,
	})
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

func cloudDownloadProviderError(err *cloud115.OpenAPIError, link string) string {
	detail := err.Error()
	if link != "" {
		detail = strings.ReplaceAll(detail, link, "[链接]")
	}
	return redactSensitiveError(errors.New(detail)).Error()
}

func (s *TelegramService) updateCloudDownloadNotice(ctx context.Context, task *model.TelegramCloudDownload, detail string) error {
	if task.Detail == detail {
		return nil
	}
	return s.updateCloudDownload(ctx, task, map[string]any{"detail": detail, "reported": false})
}
