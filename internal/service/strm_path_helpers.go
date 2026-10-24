package service

import (
	"path"
	"strings"
)

// normalizeRemotePath normalizes a STRM source path.
func normalizeRemotePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" || p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}
