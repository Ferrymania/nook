package cloud

import (
	"strings"
)

func (p *openListProvider) urlFor(remotePath string) string {
	u := *p.base
	u.RawPath = ""
	basePath := strings.TrimRight(u.Path, "/")
	remote := strings.Trim(normalizeCloudDAVPath(remotePath), "/")
	switch {
	case basePath == "" || basePath == "/":
		if remote == "" {
			u.Path = "/"
		} else {
			u.Path = "/" + remote
		}
	case remote == "":
		u.Path = basePath
	default:
		u.Path = basePath + "/" + remote
	}
	return u.String()
}
