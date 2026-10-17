package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

type SeriesArtworkCleanupResult struct {
	Removed    int
	FreedBytes int64
}

const sharedSeriesArtworkCleanupKey = "media.shared_series_artwork_v1_done"

func (s *ScraperService) consolidateSeriesArtworkOnce(ctx context.Context) (SeriesArtworkCleanupResult, error) {
	done, err := s.repo.Setting.Get(ctx, sharedSeriesArtworkCleanupKey)
	if err != nil || done == "true" {
		return SeriesArtworkCleanupResult{}, err
	}
	result, err := s.ConsolidateSeriesArtwork(ctx)
	if err == nil {
		err = s.repo.Setting.Set(ctx, sharedSeriesArtworkCleanupKey, "true")
	}
	return result, err
}

// Keep a compact restoration ledger beside the retained image. Removing a
// redundant filename does not lose its bytes or the information to restore it.
func recordSeriesArtworkRemoval(mediaPath, oldPath, retainedPath string) error {
	dir := resolveMappedDestinationPath(sharedSeriesArtworkDir(mediaPath))
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("shared artwork directory is a symlink")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	record, err := json.Marshal(struct {
		Original string `json:"original"`
		Retained string `json:"retained"`
	}{oldPath, retainedPath})
	if err != nil {
		return err
	}
	ledger := filepath.Join(dir, "cleanup.jsonl")
	if info, err := os.Lstat(ledger); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("artwork cleanup ledger is not a regular file")
	}
	file, err := os.OpenFile(ledger, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(record, '\n'))
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

// ConsolidateSeriesArtwork is offline and idempotent. It only removes old,
// base-scoped episode poster/backdrop files with an identical retained shared
// copy. Database/NFO references, unique artwork, stills and movies are kept.
func (s *ScraperService) ConsolidateSeriesArtwork(ctx context.Context) (SeriesArtworkCleanupResult, error) {
	var result SeriesArtworkCleanupResult
	libraries, err := s.repo.Library.List(ctx)
	if err != nil {
		return result, err
	}
	byID := map[string]*model.Library{}
	for i := range libraries {
		byID[libraries[i].ID] = &libraries[i]
	}
	var media []model.Media
	if err := s.repo.DB.WithContext(ctx).Find(&media).Error; err != nil {
		return result, err
	}
	refs, blocked, err := s.seriesArtworkReferences(ctx, media, libraries)
	if err != nil {
		return result, err // Incomplete reference information must never delete files.
	}
	type copyFile struct{ path, mediaPath string }
	groups := map[string][]copyFile{}
	seen := map[string]bool{}
	for i := range media {
		m := &media[i]
		if !usesSharedSeriesArtwork(m, byID[m.LibraryID]) || isCloudMediaPath(m.Path) {
			continue
		}
		physicalMedia := resolveMappedDestinationPath(m.Path)
		// Without an episode NFO these files may be the only artwork a local
		// scanner can discover. Leave them until a normal scrape writes the NFO.
		if _, err := os.Stat(nfoPath(physicalMedia)); err != nil {
			continue
		}
		for _, base := range mediaSidecarBaseVariants(physicalMedia) {
			for _, kind := range []string{"poster", "backdrop"} {
				for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".img"} {
					if err := ctx.Err(); err != nil {
						return result, err
					}
					path := filepath.Join(filepath.Dir(physicalMedia), base+"-"+kind+ext)
					keyPath := artworkReferencePath(path, "")
					if seen[keyPath] {
						continue
					}
					seen[keyPath] = true
					info, err := os.Lstat(path)
					if err != nil || !info.Mode().IsRegular() || info.Size() > 32*1024*1024 {
						continue
					}
					file, err := os.Open(path)
					if err != nil {
						return result, err
					}
					hash := sha256.New()
					_, err = io.Copy(hash, file)
					_ = file.Close()
					if err != nil {
						return result, err
					}
					key := sharedSeriesArtworkDir(physicalMedia) + fmt.Sprintf("|%x", hash.Sum(nil))
					groups[key] = append(groups[key], copyFile{path, m.Path})
				}
			}
		}
	}
	for _, copies := range groups {
		if len(copies) < 2 {
			continue
		}
		data, err := os.ReadFile(copies[0].path)
		if err != nil {
			return result, err
		}
		ctype := detectContentType(data)
		var dst string
		// An explicitly referenced copy already provides a retained original.
		// Reuse it rather than creating an unnecessary additional shared file.
		for _, copy := range copies {
			if refs[artworkReferencePath(copy.path, "")] || pathUnderAnyDirectory(copy.path, blocked) {
				dst = copy.path
				break
			}
		}
		for _, copy := range copies {
			if refs[artworkReferencePath(copy.path, "")] || pathUnderAnyDirectory(copy.path, blocked) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if dst == "" {
				sharedPath := filepath.Join(sharedSeriesArtworkDir(copy.mediaPath), fmt.Sprintf("%x%s", sha256.Sum256(data), imageExtForContentType(ctype)))
				existed := fileExists(resolveMappedDestinationPath(sharedPath))
				dst = s.storeSharedSeriesArtwork(copy.mediaPath, data, ctype)
				if dst != "" && !existed {
					result.FreedBytes -= int64(len(data))
				}
			}
			if dst == "" {
				break
			}
			// Recheck both files immediately before removal. Different/changed
			// content and symlinks must not be treated as generated duplicates.
			s.sharedArtworkMu.Lock()
			info, statErr := os.Lstat(copy.path)
			if statErr == nil && info.Mode().IsRegular() && artworkFileUnchanged(copy.path, data) && artworkFileUnchanged(resolveMappedDestinationPath(dst), data) {
				if err := recordSeriesArtworkRemoval(copy.mediaPath, copy.path, dst); err != nil {
					s.sharedArtworkMu.Unlock()
					return result, err
				}
				if err := os.Remove(copy.path); err != nil {
					s.sharedArtworkMu.Unlock()
					return result, err
				}
				result.Removed++
				result.FreedBytes += info.Size()
			}
			s.sharedArtworkMu.Unlock()
		}
	}
	return result, nil
}

func pathUnderAnyDirectory(path string, dirs []string) bool {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	for _, dir := range dirs {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func artworkReferencePath(raw, base string) string {
	raw = strings.TrimSpace(raw)
	if !isLocalImagePath(raw) {
		return ""
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(base, raw)
	}
	path := filepath.Clean(resolveMappedDestinationPath(raw))
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

func (s *ScraperService) seriesArtworkReferences(ctx context.Context, media []model.Media, libraries []model.Library) (map[string]bool, []string, error) {
	refs := map[string]bool{}
	add := func(raw, base string) {
		if path := artworkReferencePath(raw, base); path != "" {
			refs[path] = true
		}
	}
	for _, m := range media {
		base := filepath.Dir(resolveMappedDestinationPath(m.Path))
		add(m.PosterURL, base)
		add(m.BackdropURL, base)
		add(m.EpisodeStillURL, base)
	}
	var series []model.Series
	if err := s.repo.DB.WithContext(ctx).Find(&series).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range series {
		add(row.PosterURL, "")
		add(row.BackdropURL, "")
	}
	var blocked []string
	visited := map[string]bool{}
	for _, lib := range libraries {
		roots := []string{lib.Path}
		for _, root := range lib.Roots {
			roots = append(roots, root.Path)
		}
		for _, root := range roots {
			if root == "" || isCloudMediaPath(root) {
				continue
			}
			root = resolveMappedDestinationPath(root)
			err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					if errors.Is(err, os.ErrNotExist) {
						return nil
					}
					return err
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if entry.Type()&os.ModeSymlink != 0 {
					// WalkDir does not follow directory links, so reference
					// discovery under them is incomplete. Do not clean them.
					blocked = append(blocked, path)
					if strings.EqualFold(filepath.Ext(path), ".nfo") {
						blocked = append(blocked, filepath.Dir(path))
					}
					return nil
				}
				if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".nfo") || visited[path] {
					return nil
				}
				visited[path] = true
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()
				decoder := xml.NewDecoder(file)
				for {
					token, err := decoder.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						blocked = append(blocked, filepath.Dir(path))
						break
					}
					switch token := token.(type) {
					case xml.CharData:
						add(string(token), filepath.Dir(path))
					case xml.StartElement:
						for _, attr := range token.Attr {
							add(attr.Value, filepath.Dir(path))
						}
					}
				}
				return nil
			})
			if err != nil {
				return nil, nil, err
			}
		}
	}
	return refs, blocked, nil
}
