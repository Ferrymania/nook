package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// openListProvider uses the OpenList API and its WebDAV endpoint.
type openListProvider struct {
	name     string
	base     *url.URL
	username string
	password string
	token    string // 配置的静态令牌（构造后只读）
	ua       string
	apiBase  *url.URL
	client   *http.Client

	// tokenMu / loginToken / loginTokenSeen 保护 OpenList 用户名密码登录的
	// token 缓存：多 worker 并发时单飞登录，缓存有效期内直接复用，
	// 401 时清缓存重登（见 openlist_api.go 的 openListAPIToken）。
	tokenMu        sync.Mutex
	loginToken     string
	loginTokenSeen time.Time
}

func newOpenList(cfg map[string]any, client *http.Client) *openListProvider {
	const defaultDAVPath = "/dav"
	rawURL := webDAVURLFromConfig(cfg, defaultDAVPath)
	u, _ := url.Parse(strings.TrimRight(rawURL, "/"))
	apiBase := openListAPIBaseFromConfig(cfg, rawURL, defaultDAVPath)
	ua := str(cfg["ua"])
	if ua == "" {
		ua = defaultUA
	}
	return &openListProvider{
		name:     TypeOpenList,
		base:     u,
		username: str(cfg["username"]),
		password: str(cfg["password"]),
		token:    str(cfg["token"]),
		ua:       ua,
		apiBase:  apiBase,
		client:   client,
	}
}

func (p *openListProvider) Type() string { return TypeOpenList }

func (p *openListProvider) Ping(ctx context.Context) error {
	_, err := p.List(ctx, "")
	return err
}

func (p *openListProvider) Resolve(ctx context.Context, fileRef string) (*DirectLink, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	ref := normalizeCloudDAVPath(fileRef)
	if ref == "/" {
		return nil, fmt.Errorf("%s: file reference required", p.name)
	}
	if p.apiBase != nil {
		link, err := p.resolveOpenListAPIDirect(ctx, ref)
		if err == nil {
			return link, nil
		}
		// API 获取直链失败：非视频文件（元数据）回退到 WebDAV；视频文件报错
		if isCloudVideoPlaybackCandidate(ref) {
			return nil, fmt.Errorf("%s: resolve download URL for %s via API failed: %w", p.name, ref, err)
		}
	}
	headers := map[string]string{
		"User-Agent": p.ua,
	}
	if p.token != "" {
		headers["Authorization"] = p.token
	} else if p.username != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(p.username+":"+p.password))
	}
	return &DirectLink{URL: p.urlFor(ref), Headers: headers, Proxy: true}, nil
}

func (p *openListProvider) validate() error {
	if p.base == nil || p.base.Scheme == "" || p.base.Host == "" {
		return fmt.Errorf("%s: missing WebDAV URL", p.name)
	}
	return nil
}

func webDAVURLFromConfig(cfg map[string]any, defaultDAVPath string) string {
	rawURL := str(cfg["url"])
	if rawURL == "" {
		rawURL = str(cfg["webdav_url"])
	}
	if rawURL != "" {
		return ensureDefaultDAVPath(rawURL, defaultDAVPath)
	}
	return defaultWebDAVURL(str(cfg["server"]), defaultDAVPath)
}

func defaultWebDAVURL(server, defaultDAVPath string) string {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return ""
	}
	davPath := strings.TrimSpace(defaultDAVPath)
	if davPath == "" {
		return server
	}
	if !strings.HasPrefix(davPath, "/") {
		davPath = "/" + davPath
	}
	return server + davPath
}

func openListAPIBaseFromConfig(cfg map[string]any, webDAVURL, defaultDAVPath string) *url.URL {
	raw := str(cfg["server"])
	if raw == "" {
		raw = firstNonEmpty(str(cfg["api_url"]), webDAVURL)
	}
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	davPath := strings.Trim(strings.TrimSpace(defaultDAVPath), "/")
	if davPath != "" {
		pathParts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
		if len(pathParts) > 0 && strings.EqualFold(pathParts[len(pathParts)-1], davPath) {
			u.Path = strings.Join(pathParts[:len(pathParts)-1], "/")
			if u.Path == "" {
				u.Path = "/"
			}
		}
	}
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u
}

func (p *openListProvider) openListAPIURL(apiPath string) string {
	if p.apiBase == nil {
		return ""
	}
	u := *p.apiBase
	u.RawPath = ""
	basePath := strings.TrimRight(u.Path, "/")
	apiPath = "/" + strings.TrimLeft(apiPath, "/")
	if basePath == "" || basePath == "/" {
		u.Path = apiPath
	} else {
		u.Path = basePath + apiPath
	}
	return u.String()
}

func ensureDefaultDAVPath(rawURL, defaultDAVPath string) string {
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rawURL
	}
	if strings.TrimSpace(defaultDAVPath) == "" {
		return rawURL
	}
	if u.Path == "" || u.Path == "/" {
		davPath := strings.TrimSpace(defaultDAVPath)
		if !strings.HasPrefix(davPath, "/") {
			davPath = "/" + davPath
		}
		u.Path = davPath
		u.RawPath = ""
		return strings.TrimRight(u.String(), "/")
	}
	return rawURL
}

func normalizeCloudDAVPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" || p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func sameCloudDAVPath(a, b string) bool {
	return strings.TrimRight(normalizeCloudDAVPath(a), "/") == strings.TrimRight(normalizeCloudDAVPath(b), "/")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
