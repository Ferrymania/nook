package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ferrymania/nook/internal/helper"
)

const (
	githubLatestReleaseURL = "https://api.github.com/repos/Ferrymania/nook/releases/latest"
	nookReleasesURL        = "https://github.com/Ferrymania/nook/releases"
	releaseCheckCacheTTL   = 30 * time.Minute
	releaseCheckRetryTTL   = time.Minute
)

// ReleaseCheckStatus describes published versions only. It never invokes Docker
// or runs an update command, and works for both container and binary installs.
type ReleaseCheckStatus struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	ReleaseURL     string `json:"release_url"`
	ReleaseNotes   string `json:"release_notes,omitempty"`
	PublishedAt    string `json:"published_at,omitempty"`
	CheckedAt      string `json:"checked_at"`
}

type ReleaseCheckService struct {
	version string
	client  *http.Client
	now     func() time.Time
	mu      sync.Mutex
	last    ReleaseCheckStatus
	checked time.Time
}

func NewReleaseCheckService(version string) *ReleaseCheckService {
	return &ReleaseCheckService{
		version: normalizeSystemUpdateVersion(version),
		client:  helper.NewSiteHTTPClient(8, true),
		now:     time.Now,
	}
}

// Check shares a bounded cache across admins and coalesces concurrent requests.
// Manual refresh bypasses the normal cache after one minute to limit API usage.
func (s *ReleaseCheckService) Check(ctx context.Context, refresh bool) ReleaseCheckStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	ttl := releaseCheckCacheTTL
	if refresh || s.last.Status == "unavailable" {
		ttl = releaseCheckRetryTTL
	}
	if !s.checked.IsZero() && s.now().Sub(s.checked) < ttl {
		return s.last
	}
	status := s.fetch(ctx)
	// A disconnected browser should not poison the cache for other admins.
	if ctx.Err() == nil {
		s.checked = s.now()
		s.last = status
	}
	return status
}

func (s *ReleaseCheckService) fetch(ctx context.Context) ReleaseCheckStatus {
	status := ReleaseCheckStatus{
		CurrentVersion: s.version,
		Status:         "unavailable",
		Message:        "暂时无法连接 GitHub，请稍后重试或前往发布页查看",
		ReleaseURL:     nookReleasesURL,
		CheckedAt:      s.now().UTC().Format(time.RFC3339),
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return status
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Nook-Release-Check")
	resp, err := s.client.Do(req)
	if err != nil {
		return status
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusTooManyRequests:
		status.Message = "GitHub 暂时限制了检查请求，请稍后重试或前往发布页查看"
		return status
	case http.StatusNotFound:
		status.Message = "暂未获取到正式发布版本，请前往发布页查看"
		return status
	default:
		status.Message = fmt.Sprintf("GitHub 返回异常（HTTP %d），请稍后重试", resp.StatusCode)
		return status
	}
	var release struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil || release.Draft || release.Prerelease {
		status.Message = "发布信息暂不可用，请稍后重试或前往发布页查看"
		return status
	}
	latest, latestPre, ok := parseReleaseVersion(release.TagName)
	if !ok || latestPre {
		status.Message = "最新正式版本号无法识别，请前往发布页核对"
		return status
	}
	status.LatestVersion = strings.TrimSpace(release.TagName)
	status.ReleaseURL = nookReleasesURL + "/tag/" + url.PathEscape(status.LatestVersion)
	notes := []rune(strings.TrimSpace(release.Body))
	status.ReleaseNotes = string(notes[:min(len(notes), 16000)])
	status.PublishedAt = release.PublishedAt
	current, currentPre, ok := parseReleaseVersion(s.version)
	if !ok {
		status.Status = "unknown"
		status.Message = "当前为开发版或版本号无法识别，请参考最新正式版本"
		return status
	}
	for i := range latest {
		if latest[i] > current[i] {
			status.Status, status.Message = "update_available", "发现新版本"
			return status
		}
		if latest[i] < current[i] {
			status.Status, status.Message = "ahead", "当前版本高于最新正式版本"
			return status
		}
	}
	if currentPre {
		status.Status, status.Message = "update_available", "对应的正式版本已发布"
	} else {
		status.Status, status.Message = "up_to_date", "已是最新正式版本"
	}
	return status
}

var releaseVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// Only stable latest releases are accepted, so comparison needs the numeric
// core and whether the current build precedes its corresponding stable release.
func parseReleaseVersion(raw string) (core [3]uint64, prerelease bool, ok bool) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "nook-")
	raw = strings.TrimPrefix(raw, "v")
	parts := releaseVersionPattern.FindStringSubmatch(raw)
	if parts == nil {
		return core, false, false
	}
	for i := range core {
		value, err := strconv.ParseUint(parts[i+1], 10, 64)
		if err != nil {
			return core, false, false
		}
		core[i] = value
	}
	for _, id := range strings.Split(parts[4], ".") {
		if len(id) > 1 && id[0] == '0' && strings.Trim(id, "0123456789") == "" {
			return core, false, false
		}
	}
	return core, parts[4] != "", true
}
