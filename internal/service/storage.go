// Package service — media statistics from saved metadata only.
package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// StorageService reports known source-video sizes, not server disk usage.
// Computing statistics never scans files, resolves URLs or starts a media probe.
type StorageService struct {
	log  *zap.Logger
	repo *repository.Container
}

func NewStorageService(log *zap.Logger, repo *repository.Container) *StorageService {
	return &StorageService{log: log, repo: repo}
}

// Breakdown is what /api/storage returns. TotalBytes only includes known source
// sizes; UnknownSizeCount distinguishes missing metadata from a zero total.
type Breakdown struct {
	MediaCount       int64           `json:"media_count"`
	KnownSizeCount   int64           `json:"known_size_count"`
	UnknownSizeCount int64           `json:"unknown_size_count"`
	TotalBytes       int64           `json:"total_bytes"`
	TotalSeconds     int64           `json:"total_seconds"`
	ByLibrary        []LibraryUsage  `json:"by_library"`
	ByContainer      []ContainerStat `json:"by_container"`
}

// LibraryUsage counts media records (including individual episodes/versions).
type LibraryUsage struct {
	LibraryID        string `json:"library_id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Path             string `json:"path"`
	MediaCount       int64  `json:"media_count"`
	KnownSizeCount   int64  `json:"known_size_count"`
	UnknownSizeCount int64  `json:"unknown_size_count"`
	TotalBytes       int64  `json:"total_bytes"`
	TotalSeconds     int64  `json:"total_seconds"`
}

// ContainerStat retains the API's format breakdown, with the same library scope
// and known-source-size rules as the summary. STRM text bytes are never included.
type ContainerStat struct {
	Container string `json:"container"`
	Count     int64  `json:"count"`
	Bytes     int64  `json:"bytes"`
}

func (s *StorageService) Compute(ctx context.Context) (*Breakdown, error) {
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &Breakdown{ByLibrary: make([]LibraryUsage, 0, len(libs)), ByContainer: []ContainerStat{}}
	ids := make([]string, 0, len(libs))
	byID := make(map[string]int, len(libs))
	for _, l := range libs {
		byID[l.ID] = len(out.ByLibrary)
		ids = append(ids, l.ID)
		out.ByLibrary = append(out.ByLibrary, LibraryUsage{LibraryID: l.ID, Name: l.Name, Type: l.Type, Path: l.Path})
	}
	if len(ids) == 0 {
		return out, nil
	}

	// Stream one scoped query rather than loading full media/probe models or
	// performing one probe lookup per item. Decode only size_bytes from Payload;
	// no SQL JSON cast, so malformed legacy payloads simply remain unknown.
	rows, err := s.repo.DB.WithContext(ctx).Table("media AS m").
		Select(`m.library_id, m.path, COALESCE(m.strm_url, ''), COALESCE(m.container, ''),
			COALESCE(m.size_bytes, 0), COALESCE(m.duration_sec, 0),
			COALESCE(p.signature, ''), COALESCE(p.source, ''), COALESCE(p.payload, '')`).
		Joins("LEFT JOIN media_probes AS p ON p.media_id = m.id AND p.deleted_at IS NULL").
		Where("m.library_id IN ? AND m.deleted_at IS NULL", ids).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	containers := make(map[string]ContainerStat)
	for rows.Next() {
		var m model.Media
		var probe model.MediaProbe
		if err := rows.Scan(&m.LibraryID, &m.Path, &m.STRMURL, &m.Container, &m.SizeBytes, &m.DurationSec,
			&probe.Signature, &probe.Source, &probe.Payload); err != nil {
			return nil, err
		}
		size := storageSourceSize(&m, &probe)
		usage := &out.ByLibrary[byID[m.LibraryID]]
		usage.MediaCount++
		usage.TotalBytes += size
		if size > 0 {
			usage.KnownSizeCount++
		} else {
			usage.UnknownSizeCount++
		}
		if m.DurationSec > 0 {
			usage.TotalSeconds += int64(m.DurationSec)
		}
		container := strings.TrimSpace(m.Container)
		if container == "" {
			container = "unknown"
		}
		c := containers[container]
		c.Container = container
		c.Count++
		c.Bytes += size
		containers[container] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, usage := range out.ByLibrary {
		out.MediaCount += usage.MediaCount
		out.KnownSizeCount += usage.KnownSizeCount
		out.UnknownSizeCount += usage.UnknownSizeCount
		out.TotalBytes += usage.TotalBytes
		out.TotalSeconds += usage.TotalSeconds
	}
	for _, c := range containers {
		out.ByContainer = append(out.ByContainer, c)
	}
	sort.Slice(out.ByContainer, func(i, j int) bool { return out.ByContainer[i].Container < out.ByContainer[j].Container })
	return out, nil
}

func storageSourceSize(m *model.Media, probe *model.MediaProbe) int64 {
	if !isStrmMediaRow(m) {
		if m.SizeBytes > 0 {
			return m.SizeBytes
		}
		return 0
	}
	// media.size_bytes is ambiguous for STRM: a scan writes the pointer file's
	// size, while probing writes the source size. Never guess by a byte threshold.
	if probe.Payload == "" || (probe.Source != "" && probe.Source != "strm") {
		return 0
	}
	if probe.Signature != "" {
		// Only validate against the saved target. mediaProbeSignature otherwise
		// reads the STRM file when its target is absent, which statistics must not do.
		if strings.TrimSpace(m.STRMURL) == "" || probe.Signature != mediaProbeSignature(m) {
			return 0
		}
	}
	var payload struct {
		SizeBytes int64 `json:"size_bytes"`
	}
	if json.Unmarshal([]byte(probe.Payload), &payload) != nil || payload.SizeBytes <= 0 {
		return 0
	}
	return payload.SizeBytes
}
