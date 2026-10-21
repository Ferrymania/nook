// Package helper provides shared HTTP client utilities.
package helper

import (
	"net/http"
	"net/url"
	"time"
)

// NewSiteHTTPClient builds an http.Client honoring per-site policies:
//   - timeout (seconds, defaults to 15)
//   - proxy via HTTP(S)_PROXY environment variables when site.UseProxy is on
//
// When useProxy is false, the client is created without proxy plumbing so
// the request goes out direct, matching the user's checkbox intent.
func NewSiteHTTPClient(timeoutSeconds int, useProxy bool) *http.Client {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 15
	}
	tr := &http.Transport{
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if useProxy {
		tr.Proxy = func(r *http.Request) (*url.URL, error) {
			return ProxyFromEnvironmentOrSystem(r)
		}
	}
	return &http.Client{
		Timeout:   time.Duration(timeoutSeconds) * time.Second,
		Transport: tr,
	}
}
