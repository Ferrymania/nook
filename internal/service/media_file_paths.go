package service

import (
	"path/filepath"
	"strings"
)

// sanitizeFilename removes characters not safe for filesystem names across Windows, Linux, and macOS.
func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		"/", " ", "\\", " ", ":", " ", "*", "", "?", "",
		"\"", "", "<", "", ">", "", "|", "",
	)
	replaced := r.Replace(s)
	var sb strings.Builder
	for _, ch := range replaced {
		// Strip ASCII control characters (0-31, 127)
		if ch < 32 || ch == 127 {
			continue
		}
		sb.WriteRune(ch)
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

func pathWithin(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if strings.EqualFold(cleanPath, cleanRoot) {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isSeriesLibraryType(mediaType string) bool {
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}
