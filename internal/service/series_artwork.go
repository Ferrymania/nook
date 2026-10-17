package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
)

const sharedSeriesArtworkFolder = "nook-artwork"

func usesSharedSeriesArtwork(m *model.Media, lib *model.Library) bool {
	// In particular, do not migrate movies carrying old, erroneous episode
	// numbers, or theatrical features stored in an anime library.
	return m != nil && librarySupportsSeasons(lib) && mediaIsEpisodic(m, lib)
}

func sharedSeriesArtworkDir(mediaPath string) string {
	showDir := showDirFromEpisodePath(mediaPath)
	if showDir == "" || showDir == "." {
		return ""
	}
	return filepath.Join(showDir, sharedSeriesArtworkFolder)
}

func isSharedSeriesArtwork(raw string) bool {
	return !isHTTPish(raw) && filepath.Base(filepath.Dir(raw)) == sharedSeriesArtworkFolder
}

func sharedArtworkNFOReference(mediaPath, raw string) string {
	if isSharedSeriesArtwork(raw) {
		if relative, err := filepath.Rel(filepath.Dir(mediaPath), raw); err == nil {
			return filepath.ToSlash(relative)
		}
	}
	return raw
}

func existingSharedSeriesArtwork(raw string) bool {
	return strings.TrimSpace(raw) != "" && isSharedSeriesArtwork(raw) && fileExists(resolveMappedDestinationPath(raw))
}

// The content hash keeps different season artwork (and unrelated shows in a
// flat library) separate, while identical artwork across episodes/seasons is
// stored once. NFOs explicitly reference these files, so no filename guessing
// or new database mapping is needed.
func (s *ScraperService) storeSharedSeriesArtwork(mediaPath string, data []byte, ctype string) string {
	if len(data) == 0 || !isImageContentType(ctype) || isTransparentPlaceholderData(data) {
		return ""
	}
	logicalDir := sharedSeriesArtworkDir(mediaPath)
	if logicalDir == "" {
		return ""
	}
	physicalDir := sanitizeLocalPath(resolveMappedDestinationPath(logicalDir))
	s.sharedArtworkMu.Lock()
	defer s.sharedArtworkMu.Unlock()
	if info, err := os.Lstat(physicalDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	name := fmt.Sprintf("%x", sha256.Sum256(data))
	if dst := s.writeArtworkDataToPath(physicalDir, name, ctype, data); dst != "" {
		return filepath.Join(logicalDir, filepath.Base(dst))
	}
	return ""
}

func (s *ScraperService) writeSharedSeriesArtwork(ctx context.Context, m *model.Media, options ScrapeOptions) {
	updates := map[string]any{}
	seriesChanged := false
	for _, art := range []struct{ column, source string }{
		{"poster_url", m.PosterURL}, {"backdrop_url", m.BackdropURL},
	} {
		if art.source == "" || (m.EpisodeStillURL != "" && art.source == m.EpisodeStillURL) {
			continue
		}
		data, ctype := s.loadArtworkData(ctx, art.source)
		dst := s.storeSharedSeriesArtwork(m.Path, data, ctype)
		if dst == "" || dst == art.source {
			continue
		}
		updates[art.column] = dst
		// Only replace the same source in the show record. A season-specific
		// poster must not replace a separately selected show poster.
		id := seriesMetadataStorageID(scrapedSeriesPublicID(m, m.Title))
		result := s.repo.DB.WithContext(ctx).Model(&model.Series{}).
			Where("id = ? AND "+art.column+" = ?", id, art.source).Update(art.column, dst)
		if result.Error != nil {
			s.log.Warn("save shared series artwork failed", zap.String("media_id", m.ID), zap.Error(result.Error))
		}
		seriesChanged = seriesChanged || result.RowsAffected > 0
	}
	// Disabling episode images also prevents downloading an already-known
	// remote still during a later scrape. Existing stills remain untouched.
	if options.episodeArtworkEnabled() && m.EpisodeStillURL != "" {
		dir := filepath.Dir(resolveMappedDestinationPath(m.Path))
		if dst := s.downloadArtworkToPath(ctx, dir, mediaSidecarBase(m.Path)+"-thumb", m.EpisodeStillURL); dst != "" && !isHTTPish(m.EpisodeStillURL) {
			updates["episode_still_url"] = filepath.Join(filepath.Dir(m.Path), filepath.Base(dst))
		}
	}
	if len(updates) > 0 {
		if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", m.ID).Updates(updates).Error; err != nil {
			s.log.Warn("save shared episode artwork paths failed", zap.String("media_id", m.ID), zap.Error(err))
		}
	}
	if seriesChanged && s.onSeriesMetadataChanged != nil {
		s.onSeriesMetadataChanged()
	}
}
