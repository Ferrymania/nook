package cloud115

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	OfflineTaskFailed      = -1
	OfflineTaskDownloading = 1
	OfflineTaskComplete    = 2
)

type OfflineTask struct {
	ID       string      `json:"info_hash"`
	FolderID string      `json:"wp_path_id"`
	URL      string      `json:"url"`
	AddedAt  json.Number `json:"add_time"`
	Progress json.Number `json:"percentDone"`
	Status   json.Number `json:"status"`
}

type OfflineTaskPage struct {
	PageCount int           `json:"page_count"`
	Tasks     []OfflineTask `json:"tasks"`
}

// AddOfflineLink submits a single link. Transport and ambiguous response errors
// leave the caller responsible for reconciliation through the task list.
func (c *OpenClient) AddOfflineLink(ctx context.Context, link, folderID string) (string, error) {
	resp, err := c.postAuthMultipart(ctx, ProAPIBase+"/open/offline/add_task_urls", map[string]string{
		"urls": link, "wp_path_id": folderID,
	})
	if err != nil {
		return "", err
	}
	var rows []struct {
		State    RespState `json:"state"`
		Code     int       `json:"code"`
		Message  string    `json:"message"`
		InfoHash string    `json:"info_hash"`
	}
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return "", fmt.Errorf("解析 115 云下载提交结果：%w", err)
	}
	if len(rows) != 1 {
		return "", errors.New("115 未返回唯一的云下载提交结果")
	}
	row := rows[0]
	if !row.State {
		return "", NewOpenAPIError(row.Code, row.Message)
	}
	if strings.TrimSpace(row.InfoHash) == "" {
		return "", errors.New("115 未返回云下载任务 ID")
	}
	return row.InfoHash, nil
}

func (c *OpenClient) ListOfflineTasks(ctx context.Context, page int) (OfflineTaskPage, error) {
	resp, err := c.doAuthJSON(ctx, http.MethodGet, ProAPIBase+"/open/offline/get_task_list", map[string]string{
		"page": strconv.Itoa(page),
	}, 0)
	if err != nil {
		return OfflineTaskPage{}, err
	}
	var data OfflineTaskPage
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return OfflineTaskPage{}, fmt.Errorf("解析 115 云下载列表：%w", err)
	}
	if data.Tasks == nil {
		return OfflineTaskPage{}, errors.New("115 云下载列表缺少任务字段")
	}
	return data, nil
}
