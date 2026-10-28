package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

// GenreRepairPlan is an explicit, reviewable repair of one library. It is never
// run during startup or an ordinary scan. Before values also serve as a backup.
type GenreRepairPlan struct {
	Version     int                 `json:"version"`
	LibraryID   string              `json:"library_id"`
	LibraryName string              `json:"library_name"`
	CreatedAt   time.Time           `json:"created_at"`
	Changes     []GenreRepairChange `json:"changes"`
	Skipped     []GenreRepairSkip   `json:"skipped"`
}

type GenreRepairChange struct {
	MediaID   string    `json:"media_id"`
	Path      string    `json:"path"`
	NFOPath   string    `json:"nfo_path"`
	Before    string    `json:"before"`
	After     string    `json:"after"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GenreRepairSkip struct {
	MediaID string `json:"media_id"`
	Path    string `json:"path"`
	Reason  string `json:"reason"`
}

type genreRepairMedia struct {
	ID            string `gorm:"primaryKey"`
	Path          string
	Genres        string
	LibraryRootID string
	SeasonNum     int
	EpisodeNum    int
	UpdatedAt     time.Time
}

// BuildGenreRepairPlan only reads database rows and local NFO files. A change
// requires non-empty <genre> data AND an exact match to the former mixed import.
// Unknown/custom database values, incomplete NFOs and remote paths are skipped.
func BuildGenreRepairPlan(ctx context.Context, db *gorm.DB, libraryID string) (*GenreRepairPlan, error) {
	if db == nil || strings.TrimSpace(libraryID) == "" {
		return nil, errors.New("必须指定媒体库 ID")
	}
	var library model.Library
	if err := db.WithContext(ctx).Preload("Roots").First(&library, "id = ?", libraryID).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库: %w", err)
	}
	plan := &GenreRepairPlan{Version: 1, LibraryID: library.ID, LibraryName: library.Name, CreatedAt: time.Now().UTC(),
		Changes: []GenreRepairChange{}, Skipped: []GenreRepairSkip{}}
	lastID := ""
	for {
		// Use an explicit cursor: GORM FindInBatches extracts the primary key
		// using Media's embedded Base indexes, which do not match this projection.
		var rows []genreRepairMedia
		query := db.WithContext(ctx).Model(&model.Media{}).
			Select("id, path, genres, library_root_id, season_num, episode_num, updated_at").
			Where("library_id = ? AND genres IS NOT NULL AND TRIM(genres) <> ''", library.ID).
			Order("id").Limit(200)
		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			root := genreRepairRoot(library, row)
			var change *GenreRepairChange
			var err error
			if root == "" {
				err = errors.New("不是可读取的媒体库本地路径")
			} else {
				seriesLike := library.Type == "tv" || library.Type == "anime" || row.SeasonNum > 0 || row.EpisodeNum > 0
				change, err = planNFOSourcedGenres(row, root, seriesLike)
			}
			if err != nil {
				plan.Skipped = append(plan.Skipped, GenreRepairSkip{MediaID: row.ID, Path: row.Path, Reason: err.Error()})
			} else if change != nil {
				plan.Changes = append(plan.Changes, *change)
			}
		}
		if len(rows) < 200 {
			return plan, nil
		}
		lastID = rows[len(rows)-1].ID
	}
}

func genreRepairRoot(library model.Library, row genreRepairMedia) string {
	if !filepath.IsAbs(row.Path) {
		return ""
	}
	root := ""
	for _, candidate := range append(library.Roots, model.LibraryRoot{Path: library.Path}) {
		if row.LibraryRootID != "" && candidate.ID != row.LibraryRootID {
			continue
		}
		if !filepath.IsAbs(candidate.Path) {
			continue
		}
		rel, err := filepath.Rel(candidate.Path, row.Path)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(candidate.Path) > len(root) {
			root = candidate.Path
		}
	}
	return root
}

func planNFOSourcedGenres(row genreRepairMedia, root string, seriesLike bool) (*GenreRepairChange, error) {
	var paths []string
	if seriesLike {
		_, path, _, err := findShowNFO(row.Path, root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if path != "" {
			paths = append(paths, path)
		}
		paths = append(paths, nfoPath(row.Path))
	} else {
		_, path, err := findMovieNFO(row.Path, root)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	for _, path := range paths {
		body, err := os.ReadFile(path) // #nosec G304 -- discovered local NFO under the selected library root.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var doc nfoDocument
		if err := xml.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("NFO 不完整或无效: %s", path)
		}
		if doc.XMLName.Local != "movie" && doc.XMLName.Local != "tvshow" && doc.XMLName.Local != "episodedetails" {
			return nil, fmt.Errorf("不支持的 NFO 根节点: %s", path)
		}
		after := joinNFOValues(doc.Genres)
		if after == "" {
			// A show NFO with old tag-only metadata took precedence over episode
			// genres in the former reader; do not guess a replacement for it.
			if legacyMixedNFOGenres(&doc) != "" {
				return nil, fmt.Errorf("NFO 没有提供 genre，保留现值: %s", path)
			}
			continue
		}
		if genreValuesEqual(row.Genres, after) {
			return nil, nil
		}
		if !genreValuesEqual(row.Genres, legacyMixedNFOGenres(&doc)) {
			return nil, fmt.Errorf("现值与旧导入结果不一致，可能已被手动修改: %s", path)
		}
		return &GenreRepairChange{MediaID: row.ID, Path: row.Path, NFOPath: path, Before: row.Genres, After: after, UpdatedAt: row.UpdatedAt}, nil
	}
	return nil, errors.New("没有找到包含 genre 的完整 NFO，保留现值")
}

// Only used to prove historical contamination before a manual repair. Ordinary
// imports must never combine these distinct NFO fields again.
func legacyMixedNFOGenres(doc *nfoDocument) string {
	values := append([]string{}, doc.Genres...)
	values = append(values, doc.Tags...)
	values = append(values, doc.Studio, doc.Maker, doc.Publisher, doc.Label)
	return joinNFOValues(values)
}

func genreValuesEqual(a, b string) bool {
	left, right := genreSet(a), genreSet(b)
	if len(left) != len(right) {
		return false
	}
	for name := range left {
		if _, ok := right[name]; !ok {
			return false
		}
	}
	return true
}

// ApplyGenreRepairPlan updates genres only, atomically. Any changed/deleted row
// aborts the entire plan, so a reviewed report cannot overwrite newer metadata.
// The caller should restart Nook afterwards to discard cached metadata payloads.
func ApplyGenreRepairPlan(ctx context.Context, db *gorm.DB, plan *GenreRepairPlan) error {
	if db == nil || plan == nil || plan.Version != 1 || strings.TrimSpace(plan.LibraryID) == "" {
		return errors.New("无效的类型修复报告")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seen := map[string]bool{}
		for _, change := range plan.Changes {
			if change.MediaID == "" || change.Path == "" || strings.TrimSpace(change.After) == "" || change.UpdatedAt.IsZero() || seen[change.MediaID] {
				return errors.New("修复报告包含无效或重复条目")
			}
			seen[change.MediaID] = true
			result := tx.Model(&model.Media{}).
				Where("id = ? AND library_id = ? AND path = ? AND genres = ? AND updated_at = ?", change.MediaID, plan.LibraryID, change.Path, change.Before, change.UpdatedAt).
				UpdateColumn("genres", change.After)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("媒体 %s 已变化或被删除，请重新生成报告；本次修改已回滚", change.MediaID)
			}
		}
		return nil
	})
}
