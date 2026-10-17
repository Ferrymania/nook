package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func newSeriesArtworkTestEnv(t *testing.T, kind string) (*ScraperService, model.Library) {
	t.Helper()
	db := newServiceTestDB(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	repo := repository.New(db)
	cfg := &config.Config{}
	cfg.Cache.CacheDir = t.TempDir()
	s := &ScraperService{cfg: cfg, repo: repo, log: zap.NewNop(), images: NewImageProxy(cfg, zap.NewNop())}
	lib := model.Library{Name: "Library", Type: kind, Path: t.TempDir()}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	s.images.SetLibraryRootsProvider(func() []string { return []string{lib.Path} })
	return s, lib
}

func writeArtworkTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func createArtworkTestEpisode(t *testing.T, s *ScraperService, lib model.Library, show string, season, episode int) model.Media {
	t.Helper()
	m := model.Media{LibraryID: lib.ID, Title: show, SeasonNum: season, EpisodeNum: episode,
		Path: filepath.Join(lib.Path, show, fmt.Sprintf("Season %d", season), fmt.Sprintf("%s.S%02dE%02d.strm", show, season, episode))}
	writeArtworkTestFile(t, m.Path, []byte("https://video.example/test"))
	if err := s.repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSharedSeriesArtworkAcrossEpisodesSeasonsAndNFO(t *testing.T) {
	for _, kind := range []string{"tv", "anime", "variety"} {
		t.Run(kind, func(t *testing.T) {
			s, lib := newSeriesArtworkTestEnv(t, kind)
			poster := filepath.Join(t.TempDir(), "poster.jpg")
			backdrop := filepath.Join(t.TempDir(), "backdrop.jpg")
			writeArtworkTestFile(t, poster, testJPEG)
			writeArtworkTestFile(t, backdrop, append(append([]byte{}, testJPEG...), 'b'))
			rows := []model.Media{
				createArtworkTestEpisode(t, s, lib, "Show", 1, 1),
				createArtworkTestEpisode(t, s, lib, "Show", 1, 2),
				createArtworkTestEpisode(t, s, lib, "Show", 2, 1),
			}
			series := model.Series{Base: model.Base{ID: seriesMetadataStorageID(scrapedSeriesPublicID(&rows[0], "Show"))}, LibraryID: lib.ID, Title: "Show", PosterURL: poster, BackdropURL: backdrop}
			if err := s.repo.DB.Create(&series).Error; err != nil {
				t.Fatal(err)
			}
			var sharedPoster, sharedBackdrop string
			for i := range rows {
				m := &rows[i]
				if err := s.repo.DB.Model(m).Updates(map[string]any{"poster_url": poster, "backdrop_url": backdrop}).Error; err != nil {
					t.Fatal(err)
				}
				s.writeScrapedMetadataFiles(t.Context(), m, &lib, skipEpisodeArtworkOptions(false))
				got, _ := s.repo.Media.FindByID(t.Context(), m.ID)
				if i == 0 {
					sharedPoster, sharedBackdrop = got.PosterURL, got.BackdropURL
				}
				if !isSharedSeriesArtwork(got.PosterURL) || got.PosterURL != sharedPoster || got.BackdropURL != sharedBackdrop {
					t.Fatalf("episode did not share artwork: %+v", got)
				}
				// Old loose artwork must not override an explicit shared NFO path.
				writeArtworkTestFile(t, filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg"), testJPEG)
				local, err := ReadLocalMetadata(m.Path, lib.Path, true)
				if err != nil || local == nil || local.PosterURL != sharedPoster || local.BackdropURL != sharedBackdrop {
					t.Fatalf("shared NFO artwork cannot be rescanned: %+v %v", local, err)
				}
				body, _ := os.ReadFile(nfoPath(m.Path))
				if !bytes.Contains(body, []byte("../nook-artwork/")) || bytes.Contains(body, []byte(lib.Path)) {
					t.Fatalf("NFO must use portable shared paths: %s", body)
				}
			}
			entries, _ := os.ReadDir(sharedSeriesArtworkDir(rows[0].Path))
			if len(entries) != 2 {
				t.Fatalf("shared files = %d, want one poster and one backdrop", len(entries))
			}
			stored, _ := s.repo.Series.FindByID(t.Context(), series.ID)
			if stored.PosterURL != sharedPoster || stored.BackdropURL != sharedBackdrop {
				t.Fatal("show metadata still references a different source")
			}
			emby := NewEmbyService(s.cfg, s.log, s.repo)
			image, err := emby.ImageURL(t.Context(), scrapedSeriesPublicID(&rows[0], "Show"), "Primary")
			if err != nil || image != sharedPoster {
				t.Fatalf("Emby/SenPlayer did not resolve shared artwork: %q %v", image, err)
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/image", nil)
			if err := s.images.Serve(t.Context(), response, request, image); err != nil || !bytes.Equal(response.Body.Bytes(), testJPEG) {
				t.Fatalf("shared image was not served to client: %v", err)
			}
		})
	}
}

func TestSharedSeriesArtworkKeepsDifferentSeasonImagesAndMovieBehavior(t *testing.T) {
	s, lib := newSeriesArtworkTestEnv(t, "tv")
	a := createArtworkTestEpisode(t, s, lib, "Show", 1, 1)
	b := createArtworkTestEpisode(t, s, lib, "Show", 2, 1)
	first := s.storeSharedSeriesArtwork(a.Path, testJPEG, "image/jpeg")
	changed := append(append([]byte{}, testJPEG...), 'x')
	second := s.storeSharedSeriesArtwork(b.Path, changed, "image/jpeg")
	if first == second || !artworkFileUnchanged(first, testJPEG) || !artworkFileUnchanged(second, changed) {
		t.Fatalf("different season images overwrote each other: first=%q second=%q", first, second)
	}
	// Even dirty episode numbers in a movie library must retain base sidecars.
	movieLib := model.Library{Type: "movie", Path: t.TempDir(), Name: "Movies"}
	if err := s.repo.DB.Create(&movieLib).Error; err != nil {
		t.Fatal(err)
	}
	movie := createArtworkTestEpisode(t, s, movieLib, "Movie", 1, 1)
	if err := s.repo.DB.Model(&movie).Update("poster_url", first).Error; err != nil {
		t.Fatal(err)
	}
	s.writeMediaArtworkFilesAfterScrape(t.Context(), &movie, &movieLib, ScrapeOptions{})
	want := filepath.Join(filepath.Dir(movie.Path), mediaSidecarBase(movie.Path)+"-poster.jpg")
	if !artworkFileUnchanged(want, testJPEG) {
		t.Fatal("movie no longer uses its base-scoped sidecar")
	}
	if usesSharedSeriesArtwork(&model.Media{Path: "/anime/Show/剧场版/Feature.mkv"}, &lib) {
		t.Fatal("theatrical feature was treated as a shared episode")
	}
}

func TestSharedSeriesArtworkHonorsDisabledStillsAndConcurrentWrites(t *testing.T) {
	s, lib := newSeriesArtworkTestEnv(t, "tv")
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(testJPEG)
	}))
	defer upstream.Close()
	s.images.SetAllowedRemoteHostsProvider(func() []string { return []string{"127.0.0.1"} })
	m := createArtworkTestEpisode(t, s, lib, "Show", 1, 1)
	if err := s.repo.DB.Model(&m).Updates(map[string]any{
		"episode_still_url": upstream.URL + "/still.jpg",
		"poster_url":        upstream.URL + "/still.jpg", "backdrop_url": upstream.URL + "/still.jpg",
	}).Error; err != nil {
		t.Fatal(err)
	}
	s.writeMediaArtworkFilesAfterScrape(t.Context(), &m, &lib, skipEpisodeArtworkOptions(false))
	if calls.Load() != 0 {
		t.Fatal("disabled episode images downloaded a previously saved still URL")
	}
	s.writeMediaArtworkFilesAfterScrape(t.Context(), &m, &lib, ScrapeOptions{})
	if calls.Load() != 1 || !fileExists(filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-thumb.jpg")) {
		t.Fatal("enabled individual still was not written")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { s.storeSharedSeriesArtwork(m.Path, testJPEG, "image/jpeg") })
	}
	wg.Wait()
	entries, _ := os.ReadDir(sharedSeriesArtworkDir(m.Path))
	if len(entries) != 1 {
		t.Fatalf("concurrent shared writes created %d files", len(entries))
	}
}

func TestSharedSeriesArtworkRemoteSourceIsFetchedAndStoredOnce(t *testing.T) {
	s, lib := newSeriesArtworkTestEnv(t, "tv")
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(testJPEG)
	}))
	defer upstream.Close()
	s.images.SetAllowedRemoteHostsProvider(func() []string { return []string{"127.0.0.1"} })
	var shared string
	for i := 1; i <= 4; i++ {
		m := createArtworkTestEpisode(t, s, lib, "Show", 1, i)
		if err := s.repo.DB.Model(&m).Updates(map[string]any{"poster_url": upstream.URL + "/art.jpg", "backdrop_url": upstream.URL + "/art.jpg"}).Error; err != nil {
			t.Fatal(err)
		}
		s.writeScrapedMetadataFiles(t.Context(), &m, &lib, skipEpisodeArtworkOptions(false))
		got, _ := s.repo.Media.FindByID(t.Context(), m.ID)
		if i == 1 {
			shared = got.PosterURL
		}
		if !isSharedSeriesArtwork(shared) || got.PosterURL != shared || got.BackdropURL != shared {
			t.Fatalf("remote artwork did not share a local file: %+v", got)
		}
		if fileExists(filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg")) {
			t.Fatal("remote scrape still wrote a per-episode poster")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream fetched %d times, want one cached source", calls.Load())
	}
	entries, _ := os.ReadDir(filepath.Dir(shared))
	if len(entries) != 1 {
		t.Fatalf("identical poster/backdrop bytes generated %d copies", len(entries))
	}
}

func TestConsolidateSeriesArtworkPreservesReferencesUniqueStillsAndMovies(t *testing.T) {
	s, lib := newSeriesArtworkTestEnv(t, "tv")
	var legacy []string
	for i := 1; i <= 3; i++ {
		m := createArtworkTestEpisode(t, s, lib, "Show", 1, i)
		path := filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg")
		writeArtworkTestFile(t, path, testJPEG)
		legacy = append(legacy, path)
		nfo := `<episodedetails><thumb>https://example.test/poster.jpg</thumb></episodedetails>`
		if i == 1 {
			if err := s.repo.DB.Model(&m).Update("poster_url", path).Error; err != nil {
				t.Fatal(err)
			}
		}
		if i == 2 {
			nfo = `<episodedetails><thumb>` + filepath.Base(path) + `</thumb></episodedetails>`
		}
		writeArtworkTestFile(t, nfoPath(m.Path), []byte(nfo))
		writeArtworkTestFile(t, filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-thumb.jpg"), testJPEG)
		if i == 3 {
			writeArtworkTestFile(t, filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-backdrop.jpg"), append(append([]byte{}, testJPEG...), 'u'))
		}
	}
	// A malformed NFO makes reference discovery uncertain: keep that directory.
	for i := 1; i <= 2; i++ {
		m := createArtworkTestEpisode(t, s, lib, "Broken", 1, i)
		writeArtworkTestFile(t, nfoPath(m.Path), []byte(`<episodedetails><thumb>`))
		writeArtworkTestFile(t, filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg"), testJPEG)
	}
	movieLib := model.Library{Name: "Movies", Type: "movie", Path: t.TempDir()}
	if err := s.repo.DB.Create(&movieLib).Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		m := createArtworkTestEpisode(t, s, movieLib, "Movie", 1, i)
		writeArtworkTestFile(t, nfoPath(m.Path), []byte(`<movie/>`))
		writeArtworkTestFile(t, filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg"), testJPEG)
	}
	result, err := s.ConsolidateSeriesArtwork(t.Context())
	if err != nil || result.Removed != 1 || result.FreedBytes != int64(len(testJPEG)) {
		t.Fatalf("unsafe/incomplete cleanup: %+v %v", result, err)
	}
	if !fileExists(legacy[0]) || !fileExists(legacy[1]) || fileExists(legacy[2]) {
		t.Fatal("reference preservation/removal is incorrect")
	}
	ledger, err := os.ReadFile(filepath.Join(sharedSeriesArtworkDir(legacy[0]), "cleanup.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var entry struct{ Original, Retained string }
	if err := json.Unmarshal(bytes.TrimSpace(ledger), &entry); err != nil || entry.Original != legacy[2] || !artworkFileUnchanged(entry.Retained, testJPEG) {
		t.Fatalf("removed file cannot be restored from ledger: %+v %v", entry, err)
	}
	var posters, thumbs, backdrops int
	_ = filepath.WalkDir(lib.Path, func(path string, entry os.DirEntry, err error) error {
		if strings.HasSuffix(path, "-poster.jpg") {
			posters++
		}
		if strings.HasSuffix(path, "-thumb.jpg") {
			thumbs++
		}
		if strings.HasSuffix(path, "-backdrop.jpg") {
			backdrops++
		}
		return nil
	})
	if posters != 4 || thumbs != 3 || backdrops != 1 {
		t.Fatalf("unique/referenced/broken-NFO images changed: posters=%d thumbs=%d backdrops=%d", posters, thumbs, backdrops)
	}
	second, err := s.ConsolidateSeriesArtwork(t.Context())
	if err != nil || second.Removed != 0 {
		t.Fatalf("cleanup is not idempotent: %+v %v", second, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.ConsolidateSeriesArtwork(ctx); err == nil {
		t.Fatal("canceled cleanup continued")
	}
}

func TestConsolidateSeriesArtworkOfflineOnceKeepsRestorableSharedOriginal(t *testing.T) {
	s, lib := newSeriesArtworkTestEnv(t, "anime")
	var old []string
	for i := 1; i <= 4; i++ {
		m := createArtworkTestEpisode(t, s, lib, "Show", (i+1)/2, (i+1)%2+1)
		writeArtworkTestFile(t, nfoPath(m.Path), []byte(`<episodedetails><thumb>https://offline.invalid/poster.jpg</thumb></episodedetails>`))
		path := filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg")
		writeArtworkTestFile(t, path, testJPEG)
		old = append(old, path)
	}
	result, err := s.consolidateSeriesArtworkOnce(t.Context())
	if err != nil || result.Removed != 4 || result.FreedBytes != int64(3*len(testJPEG)) {
		t.Fatalf("offline consolidation failed: %+v %v", result, err)
	}
	for _, path := range old {
		if fileExists(path) {
			t.Fatalf("redundant copy still exists: %s", path)
		}
	}
	ledger, err := os.ReadFile(filepath.Join(sharedSeriesArtworkDir(old[0]), "cleanup.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(ledger), []byte{'\n'}) {
		var record struct{ Original, Retained string }
		if err := json.Unmarshal(line, &record); err != nil || !artworkFileUnchanged(record.Retained, testJPEG) {
			t.Fatalf("missing restoration data: %+v %v", record, err)
		}
	}
	done, _ := s.repo.Setting.Get(t.Context(), sharedSeriesArtworkCleanupKey)
	if done != "true" {
		t.Fatal("successful startup migration was not recorded")
	}
	// Later boots must not repeat a whole-library filesystem scan.
	second, err := s.consolidateSeriesArtworkOnce(t.Context())
	if err != nil || second.Removed != 0 {
		t.Fatalf("one-time cleanup ran again: %+v %v", second, err)
	}
}

func TestConsolidateSeriesArtworkSkipsLinkedDirectories(t *testing.T) {
	s, lib := newSeriesArtworkTestEnv(t, "tv")
	outside := t.TempDir()
	linkedShow := filepath.Join(lib.Path, "Linked")
	if err := os.Symlink(outside, linkedShow); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for i := 1; i <= 2; i++ {
		m := createArtworkTestEpisode(t, s, lib, "Linked", 1, i)
		// Rows can use the real path while a library root uses a symlink.
		m.Path = filepath.Join(outside, "Season 1", filepath.Base(m.Path))
		if err := s.repo.DB.Model(&m).Update("path", m.Path).Error; err != nil {
			t.Fatal(err)
		}
		writeArtworkTestFile(t, nfoPath(m.Path), []byte(`<episodedetails><thumb>https://example.test/poster.jpg</thumb></episodedetails>`))
		writeArtworkTestFile(t, filepath.Join(filepath.Dir(m.Path), mediaSidecarBase(m.Path)+"-poster.jpg"), testJPEG)
	}
	result, err := s.ConsolidateSeriesArtwork(t.Context())
	if err != nil || result.Removed != 0 {
		t.Fatalf("cleaned images without traversing their linked NFO directory: %+v %v", result, err)
	}
}
