package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (p *openListProvider) List(ctx context.Context, dir string) ([]FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p.listOpenListAPI(ctx, dir)
}

func (p *openListProvider) firstHTTPRedirectLocation(ctx context.Context, target string, headers map[string]string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", "bytes=0-0")
	if strings.TrimSpace(p.ua) != "" {
		req.Header.Set("User-Agent", p.ua)
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	status := resp.StatusCode
	if status >= 300 && status < 400 {
		rawLocation := strings.TrimSpace(resp.Header.Get("Location"))
		if rawLocation == "" {
			return "", status, fmt.Errorf("%s: upstream returned redirect http %d without Location", p.name, status)
		}
		location, err := resolveHTTPRedirectLocation(target, rawLocation)
		if err != nil {
			return "", status, err
		}
		return location, status, nil
	}
	return "", status, nil
}

func resolveHTTPRedirectLocation(baseURL, rawLocation string) (string, error) {
	rawLocation = strings.TrimSpace(rawLocation)
	if rawLocation == "" {
		return "", fmt.Errorf("empty redirect Location")
	}
	if strings.HasPrefix(rawLocation, "//") {
		base, err := url.Parse(baseURL)
		if err != nil || base.Scheme == "" {
			return "", fmt.Errorf("protocol-relative redirect Location without base scheme")
		}
		rawLocation = base.Scheme + ":" + rawLocation
	}
	location, err := url.Parse(rawLocation)
	if err != nil {
		return "", fmt.Errorf("invalid redirect Location: %w", err)
	}
	if location.IsAbs() {
		if location.Scheme != "http" && location.Scheme != "https" {
			return "", fmt.Errorf("unsupported redirect Location scheme %q", location.Scheme)
		}
		return location.String(), nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid redirect base URL: %w", err)
	}
	return base.ResolveReference(location).String(), nil
}
