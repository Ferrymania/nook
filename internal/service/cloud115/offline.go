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
	OfflineLinkBatchLimit  = 115
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

type OfflineLinkResult struct {
	Accepted RespState `json:"state"`
	Code     int       `json:"code"`
	Message  string    `json:"message"`
	TaskID   string    `json:"info_hash"`
	URL      string    `json:"url"`
}

// AddOfflineLinks submits one request for the entire batch. The caller
// reconciles any missing or ambiguous results through the task list.
func (c *OpenClient) AddOfflineLinks(ctx context.Context, links []string, folderID string) ([]OfflineLinkResult, error) {
	if len(links) == 0 || len(links) > OfflineLinkBatchLimit {
		return nil, fmt.Errorf("115 云下载每次接受 1–%d 条链接", OfflineLinkBatchLimit)
	}
	resp, err := c.postAuthMultipart(ctx, ProAPIBase+"/open/offline/add_task_urls", map[string]string{
		"urls": strings.Join(links, "\n"), "wp_path_id": folderID,
	})
	if err != nil {
		return nil, err
	}
	var rows []OfflineLinkResult
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("解析 115 云下载提交结果：%w", err)
	}
	return rows, nil
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
