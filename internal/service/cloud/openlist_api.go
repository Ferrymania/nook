package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// FetchUsername 使用已配置的 Token 查询 OpenList 当前用户，不调用登录接口。
func (p *openListProvider) FetchUsername(ctx context.Context) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.openListAPIURL("/api/me"), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", p.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.ua)
	resp, err := p.client.Do(req)
	if err != nil {
		return "", decorateDAVTransportError(p.name, req.URL.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", p.openListAPIStatusError("user info", "", resp.StatusCode)
	}
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", fmt.Errorf("openlist: decode user info: %w", err)
	}
	if result.Code != http.StatusOK || strings.TrimSpace(result.Data.Username) == "" {
		return "", fmt.Errorf("openlist: 获取用户名失败：%s", firstNonEmpty(result.Message, "接口未返回用户名"))
	}
	return strings.TrimSpace(result.Data.Username), nil
}

func (p *openListProvider) listOpenListAPI(ctx context.Context, dir string) ([]FileEntry, error) {
	const pageSize = 500
	target := normalizeCloudDAVPath(dir)
	out := make([]FileEntry, 0, pageSize)
	for pageNum := 1; ; pageNum++ {
		payload := map[string]any{
			"path":     target,
			"password": "",
			"page":     pageNum,
			"per_page": pageSize,
			"refresh":  false,
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL("/api/fs/list"), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", p.ua)
		req.Header.Set("Authorization", p.token)
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, decorateDAVTransportError(p.name, p.openListAPIURL("/api/fs/list"), err)
		}
		var decoded openListListResponse
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&decoded)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, p.openListAPIStatusError("list", target, resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("%s: decode api list: %w", p.name, decodeErr)
		}
		if decoded.Code != 0 && decoded.Code != 200 {
			msg := strings.TrimSpace(decoded.Message)
			if msg == "" {
				msg = fmt.Sprintf("code %d", decoded.Code)
			}
			return nil, fmt.Errorf("%s: api list %s failed: %s", p.name, target, msg)
		}
		for _, item := range decoded.Data.Content {
			name := strings.TrimSpace(item.Name)
			if name == "" || name == "." || name == "/" {
				continue
			}
			out = append(out, FileEntry{
				ID:    joinOpenListAPIPath(target, name),
				Name:  name,
				IsDir: item.IsDir,
				Size:  item.Size,
			})
		}
		total := decoded.Data.Total
		if total > 0 {
			if len(out) >= total || len(decoded.Data.Content) == 0 {
				break
			}
			continue
		}
		if len(decoded.Data.Content) == 0 || len(decoded.Data.Content) < pageSize {
			break
		}
	}
	return out, nil
}

func (p *openListProvider) resolveOpenListAPIDirect(ctx context.Context, fileRef string) (*DirectLink, error) {
	payload, _ := json.Marshal(map[string]string{"path": normalizeCloudDAVPath(fileRef), "password": ""})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL("/api/fs/get"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.ua)
	req.Header.Set("Authorization", p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, decorateDAVTransportError(p.name, p.openListAPIURL("/api/fs/get"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, p.openListAPIStatusError("get", fileRef, resp.StatusCode)
	}
	var decoded openListGetResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%s: decode api get: %w", p.name, err)
	}
	if decoded.Code != 0 && decoded.Code != 200 {
		msg := strings.TrimSpace(decoded.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", decoded.Code)
		}
		return nil, fmt.Errorf("%s: api get %s failed: %s", p.name, fileRef, msg)
	}
	raw := firstNonEmpty(decoded.Data.RawURL, decoded.Data.URL)
	if raw == "" {
		return nil, fmt.Errorf("%s: api get %s returned empty raw_url", p.name, fileRef)
	}
	resolved, err := p.resolveOpenListPlaybackURL(raw)
	if err != nil {
		return nil, err
	}
	headers := normalizeOpenListPlaybackHeaders(decoded.Data.Header)
	if len(headers) > 0 {
		return nil, fmt.Errorf("%s: api get %s returned raw_url that requires headers (%s); refusing WebDAV/proxy fallback for pure 302 playback", p.name, fileRef, strings.Join(sortedHeaderNames(headers), ","))
	}
	resolved, err = p.resolveOpenListCDNRedirect(ctx, fileRef, resolved)
	if err != nil {
		return nil, err
	}
	return &DirectLink{URL: resolved, Headers: nil, Proxy: false}, nil
}

func (p *openListProvider) resolveOpenListCDNRedirect(ctx context.Context, fileRef, rawURL string) (string, error) {
	if p.apiBase == nil || !sameURLHost(rawURL, p.apiBase) {
		return rawURL, nil
	}
	location, status, err := p.firstHTTPRedirectLocation(ctx, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("%s: probe raw_url %s failed: %w", p.name, fileRef, err)
	}
	if location != "" {
		return location, nil
	}
	return "", fmt.Errorf("%s: api get %s returned an OpenList-hosted raw_url with http %d and no CDN Location; refusing OpenList/WebDAV proxy fallback for pure 302 playback", p.name, fileRef, status)
}

func (p *openListProvider) openListAPIStatusError(action, target string, status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%s: api %s %s returned http %d；请检查或更换 OpenList Token，并确认填写的是 OpenList 服务地址而不是 /dav 地址", p.name, action, target, status)
	}
	return fmt.Errorf("%s: api %s %s returned http %d", p.name, action, target, status)
}

func (p *openListProvider) resolveOpenListPlaybackURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%s: empty playback URL", p.name)
	}
	if strings.HasPrefix(raw, "//") {
		if p.apiBase == nil || p.apiBase.Scheme == "" {
			return "", fmt.Errorf("%s: protocol-relative playback URL without API base", p.name)
		}
		raw = p.apiBase.Scheme + ":" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: invalid playback URL: %w", p.name, err)
	}
	if u.IsAbs() {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", fmt.Errorf("%s: unsupported playback URL scheme %q", p.name, u.Scheme)
		}
		return u.String(), nil
	}
	if p.apiBase == nil {
		return "", fmt.Errorf("%s: relative playback URL without API base", p.name)
	}
	base := *p.apiBase
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return base.ResolveReference(u).String(), nil
}
