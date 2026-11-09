package service

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
)

func TestExternalRatingsBackfillDeduplicatesAndPreservesPartialResults(t *testing.T) {
	ctx := t.Context()
	db := newServiceTestDB(t)
	repo := repository.New(db)
	log := zap.NewNop()
	api := NewAPIConfigService(log, repo, NewCryptoService("ratings-test-secret", log))
	key, limit := "omdb-fixture-key", "1"
	if _, err := api.Update(ctx, "omdb", APIConfigPatch{APIKey: &key, Extra: &limit}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "tmdb-fixture-key"
	tmdb := NewTMDbProvider(cfg, log, nil)
	var tmdbRequests []string
	tmdb.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		tmdbRequests = append(tmdbRequests, r.URL.Path)
		return ratingTestResponse(http.StatusOK, `{"imdb_id":"tt1234567"}`), nil
	})
	svc := newExternalRatingService(repo, api, tmdb, log)
	var omdbRequests int
	svc.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		omdbRequests++
		if q := r.URL.Query(); q.Get("i") != "tt1234567" || q.Get("apikey") != key || q.Get("type") != "series" {
			return nil, fmt.Errorf("unexpected OMDb request")
		}
		return ratingTestResponse(http.StatusOK, `{"Response":"True","imdbID":"tt1234567","Type":"series","Ratings":[{"Source":"Internet Movie Database","Value":"7.6/10"},{"Source":"Rotten Tomatoes","Value":"N/A"}]}`), nil
	})

	lib := model.Library{Name: "剧集", Path: "/media/shows", Type: "tv"}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := []model.Media{
		{LibraryID: lib.ID, Title: "A", Path: "/media/shows/A/S01E01.strm", TMDbID: 42, TMDbMediaType: "tv", ScrapeStatus: "matched", Rating: 8.2},
		{LibraryID: lib.ID, Title: "A", Path: "/media/shows/A/S01E02.strm", TMDbID: 42, TMDbMediaType: "tv", ScrapeStatus: "matched"},
		{LibraryID: lib.ID, Title: "B", Path: "/media/shows/B/S01E01.strm", TMDbID: 99, TMDbMediaType: "tv", ScrapeStatus: "matched"},
		{LibraryID: lib.ID, Title: "C", Path: "/media/shows/C/S01E01.strm", TMDbID: 77, ScrapeStatus: "matched"},
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	status, err := svc.LibraryStatus(ctx, lib.ID)
	if err != nil || status.Total != 2 || status.Unqueried != 2 {
		t.Fatalf("initial status = %+v, %v", status, err)
	}
	if count, err := svc.EnqueueLibrary(ctx, lib.ID); err != nil || count != 2 {
		t.Fatalf("enqueue = %d, %v", count, err)
	}
	if count, err := svc.EnqueueLibrary(ctx, lib.ID); err != nil || count != 0 {
		t.Fatalf("duplicate enqueue = %d, %v", count, err)
	}
	if ok, err := svc.processNext(ctx); err != nil || !ok {
		t.Fatalf("process = %v, %v", ok, err)
	}
	if ok, err := svc.processNext(ctx); err != nil || ok {
		t.Fatalf("process beyond budget = %v, %v", ok, err)
	}
	status, err = svc.LibraryStatus(ctx, lib.ID)
	if err != nil || status.Partial != 1 || status.Pending != 1 || status.PauseReason != "quota" ||
		omdbRequests != 1 || len(tmdbRequests) != 1 || !strings.HasSuffix(tmdbRequests[0], "/tv/42/external_ids") {
		t.Fatalf("status = %+v, TMDb calls = %v, OMDb calls = %d, error = %v", status, tmdbRequests, omdbRequests, err)
	}
	scores, err := svc.ForMedia(ctx, &media[1])
	if err != nil || scores.IMDb == nil || *scores.IMDb != 7.6 || scores.RottenTomatoes != nil ||
		scores.Metacritic != nil || scores.CheckedAt == nil || scores.IMDbID != "tt1234567" {
		t.Fatalf("second episode scores = %+v, %v", scores, err)
	}
	original, err := repo.Media.FindByID(ctx, media[0].ID)
	if err != nil || original == nil || original.Rating != 8.2 {
		t.Fatalf("media rating overwritten: %+v, %v", original, err)
	}
	limit = "2"
	if _, err := api.Update(ctx, "omdb", APIConfigPatch{Extra: &limit}); err != nil {
		t.Fatal(err)
	}
	svc.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return ratingTestResponse(http.StatusOK, `{"Response":"False","Error":"Movie not found!"}`), nil
	})
	if ok, err := svc.processNext(ctx); err != nil || !ok {
		t.Fatalf("process missing title = %v, %v", ok, err)
	}
	status, err = svc.LibraryStatus(ctx, lib.ID)
	if err != nil || status.Missing != 1 || status.Unqueried != 0 || status.Pending != 0 {
		t.Fatalf("after missing title lookup: %+v, %v", status, err)
	}
	if count, err := svc.EnqueueLibrary(ctx, lib.ID); err != nil || count != 0 {
		t.Fatalf("requeued a fresh negative result: %d, %v", count, err)
	}

}

func TestExternalRatingsQuotaAndInvalidKeyAreDistinct(t *testing.T) {
	svc := newExternalRatingService(nil, nil, nil, zap.NewNop())
	for _, fixture := range []struct{ message, reason string }{
		{"Request limit reached!", "quota"},
		{"Invalid API key!", "invalid_key"},
		{"Upstream blocked request", "omdb_error"},
	} {
		svc.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return ratingTestResponse(http.StatusUnauthorized, fmt.Sprintf(`{"Response":"False","Error":%q}`, fixture.message)), nil
		})
		if _, reason := svc.requestOMDb(t.Context(), "fixture", "tt1234567", "movie"); reason != fixture.reason {
			t.Errorf("%s => %s, want %s", fixture.message, reason, fixture.reason)
		}
	}
}

func TestExternalRatingsBudgetRestartsOnNextUTCDay(t *testing.T) {
	repo := repository.New(newServiceTestDB(t))
	svc := newExternalRatingService(repo, nil, nil, zap.NewNop())
	now := time.Date(2026, 1, 1, 23, 59, 59, 0, time.UTC)
	for _, step := range []struct {
		elapsed time.Duration
		allowed bool
	}{
		{0, true}, {0, false}, {time.Second, true}, {time.Second, false},
	} {
		ok, err := svc.reserveOMDbRequest(t.Context(), "fixture", 1, now.Add(step.elapsed))
		if err != nil || ok != step.allowed {
			t.Fatalf("reserve after %s = %v, %v", step.elapsed, ok, err)
		}
	}
}

func ratingTestResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}
}
