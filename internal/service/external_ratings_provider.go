package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
)

const omdbURL = "https://www.omdbapi.com/"

var imdbIDPattern = regexp.MustCompile(`^tt[0-9]{7,}$`)

func (t *TMDbProvider) ratingIMDbID(ctx context.Context, work ratingWork) (string, error) {
	cfg := t.runtimeConfig(ctx)
	if !cfg.Enabled || cfg.APIKey == "" {
		return "", errors.New("tmdb unavailable")
	}
	params := url.Values{"api_key": {cfg.APIKey}}
	var response struct {
		IMDbID string `json:"imdb_id"`
	}
	u := fmt.Sprintf("%s/%s/%d/external_ids?%s", cfg.BaseURL, work.Kind, work.TMDbID, params.Encode())
	if err := t.getJSON(ctx, u, &response); err != nil {
		// getJSON errors can include the TMDb key in the request URL.
		return "", errors.New("tmdb external ID lookup failed")
	}
	return strings.TrimSpace(response.IMDbID), nil
}

func omdbKeyDigest(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func (s *ExternalRatingService) pauseReason(ctx context.Context, key string, limit int, now time.Time) (string, error) {
	var budget model.ExternalRatingBudget
	err := s.repo.DB.WithContext(ctx).Take(&budget, "key_digest = ?", omdbKeyDigest(key)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if budget.Paused == "invalid_key" {
		return budget.Paused, nil
	}
	if budget.Day == now.UTC().Format(time.DateOnly) && (budget.Used >= limit || budget.Paused == "quota") {
		return "quota", nil
	}
	return "", nil
}

func (s *ExternalRatingService) reserveOMDbRequest(ctx context.Context, key string, limit int, now time.Time) (bool, error) {
	row := model.ExternalRatingBudget{KeyDigest: omdbKeyDigest(key), Day: now.UTC().Format(time.DateOnly), Used: 1}
	result := s.repo.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key_digest"}},
		DoUpdates: clause.Assignments(map[string]any{
			"day":    row.Day,
			"used":   gorm.Expr("CASE WHEN external_rating_budgets.day = ? THEN external_rating_budgets.used + 1 ELSE 1 END", row.Day),
			"paused": "",
		}),
		Where: clause.Where{Exprs: []clause.Expression{clause.Expr{
			SQL:  "external_rating_budgets.paused <> ? AND (external_rating_budgets.day < ? OR (external_rating_budgets.day = ? AND external_rating_budgets.used < ? AND external_rating_budgets.paused = ''))",
			Vars: []any{"invalid_key", row.Day, row.Day, limit},
		}}},
	}).Create(&row)
	return result.RowsAffected > 0, result.Error
}

func (s *ExternalRatingService) fetchRatings(ctx context.Context, cfg Resolved, limit int, row model.ExternalRating, now time.Time) (map[string]any, error) {
	if !imdbIDPattern.MatchString(row.IMDbID) {
		imdbID, err := s.tmdb.ratingIMDbID(ctx, ratingWork{row.Kind, row.TMDbID})
		if err != nil {
			return ratingRetryUpdates("tmdb_error", row.IMDbID, now), nil
		}
		if !imdbIDPattern.MatchString(imdbID) {
			return ratingOutcome(row, "no_imdb_id", now), nil
		}
		row.IMDbID = imdbID
	}
	requestTime := time.Now()
	reserved, err := s.reserveOMDbRequest(ctx, cfg.APIKey, limit, requestTime)
	if err != nil {
		return nil, err
	}
	if !reserved {
		return map[string]any{"imdb_id": row.IMDbID, "next_attempt_at": now}, nil
	}

	result, reason := s.requestOMDb(ctx, cfg.APIKey, row.IMDbID, row.Kind)
	switch reason {
	case "invalid_key", "quota":
		db := s.repo.DB.WithContext(ctx).Model(&model.ExternalRatingBudget{}).
			Where("key_digest = ?", omdbKeyDigest(cfg.APIKey))
		if reason == "quota" {
			db = db.Where("day = ? AND paused <> ?", requestTime.UTC().Format(time.DateOnly), "invalid_key")
		}
		if err := db.Update("paused", reason).Error; err != nil {
			return nil, err
		}
		return map[string]any{"imdb_id": row.IMDbID, "next_attempt_at": now}, nil
	case "omdb_error":
		return ratingRetryUpdates(reason, row.IMDbID, now), nil
	case "not_found":
		return ratingOutcome(row, reason, now), nil
	default:
		if result.IMDb != nil {
			row.IMDb = result.IMDb
		}
		if result.RottenTomatoes != nil {
			row.RottenTomatoes = result.RottenTomatoes
		}
		if result.Metacritic != nil {
			row.Metacritic = result.Metacritic
		}
		return ratingOutcome(row, "", now), nil
	}
}

func ratingRetryUpdates(reason, imdbID string, now time.Time) map[string]any {
	return map[string]any{
		"status": ratingRetry, "reason": reason, "imdb_id": imdbID,
		"next_attempt_at": now.Add(time.Hour),
	}
}

func ratingOutcome(row model.ExternalRating, reason string, now time.Time) map[string]any {
	status := ratingPartial
	switch {
	case row.IMDb != nil && row.RottenTomatoes != nil && row.Metacritic != nil:
		status = ratingFound
	case row.IMDb == nil && row.RottenTomatoes == nil && row.Metacritic == nil:
		status = ratingMissing
		if reason == "" {
			reason = "no_scores"
		}
	}
	return map[string]any{
		"status": status, "reason": reason, "imdb_id": row.IMDbID,
		"imdb": row.IMDb, "rotten_tomatoes": row.RottenTomatoes, "metacritic": row.Metacritic,
		"checked_at": now, "next_attempt_at": now.Add(30 * 24 * time.Hour),
	}
}

type omdbTitleResponse struct {
	Response   string
	Error      string
	IMDbID     string `json:"imdbID"`
	IMDbRating string `json:"imdbRating"`
	Metascore  string
	Type       string
	Ratings    []struct{ Source, Value string }
}

func (s *ExternalRatingService) requestOMDb(ctx context.Context, key, id, kind string) (model.ExternalRating, string) {
	var ratings model.ExternalRating
	titleType := kind
	if kind == "tv" {
		titleType = "series"
	}
	params := url.Values{"apikey": {key}, "i": {id}, "type": {titleType}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, omdbURL+"?"+params.Encode(), nil)
	if err != nil {
		return ratings, "omdb_error"
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return ratings, "omdb_error"
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ratings, "quota"
	}
	var result omdbTitleResponse
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
	if decodeErr == nil && strings.EqualFold(result.Response, "false") {
		message := strings.ToLower(strings.TrimSpace(result.Error))
		switch {
		case strings.Contains(message, "request limit"):
			return ratings, "quota"
		case strings.Contains(message, "invalid api key") || strings.Contains(message, "no api key"):
			return ratings, "invalid_key"
		case resp.StatusCode == http.StatusOK && strings.Contains(message, "not found"):
			return ratings, "not_found"
		}
	}
	if decodeErr != nil || resp.StatusCode != http.StatusOK {
		return ratings, "omdb_error"
	}
	if !strings.EqualFold(result.Response, "true") || result.IMDbID != id || !strings.EqualFold(result.Type, titleType) {
		return ratings, "omdb_error"
	}
	for _, score := range result.Ratings {
		switch strings.ToLower(strings.TrimSpace(score.Source)) {
		case "internet movie database":
			if raw, ok := strings.CutSuffix(strings.TrimSpace(score.Value), "/10"); ok {
				ratings.IMDb = parseIMDbRating(raw)
			}
		case "rotten tomatoes":
			ratings.RottenTomatoes = parsePercentScore(score.Value, "%")
		case "metacritic":
			ratings.Metacritic = parsePercentScore(score.Value, "/100")
		}
	}
	if ratings.IMDb == nil {
		ratings.IMDb = parseIMDbRating(result.IMDbRating)
	}
	if ratings.Metacritic == nil {
		ratings.Metacritic = parsePercentScore(result.Metascore+"/100", "/100")
	}
	return ratings, ""
}

func parseIMDbRating(raw string) *float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err == nil && value >= 0 && value <= 10 {
		return &value
	}
	return nil
}

func parsePercentScore(raw, suffix string) *int {
	value, ok := strings.CutSuffix(strings.TrimSpace(raw), suffix)
	if !ok {
		return nil
	}
	if score, err := strconv.Atoi(value); err == nil && score >= 0 && score <= 100 {
		return &score
	}
	return nil
}
