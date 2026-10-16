package service

import (
	"context"
	"fmt"
	"net/url"
)

// GetTVSeasonYear 用于核对后续季文件中的年份，不使用整剧首播年来代替。
func (t *TMDbProvider) GetTVSeasonYear(ctx context.Context, tmdbID, season int) (int, error) {
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" || tmdbID <= 0 || season < 0 {
		return 0, fmt.Errorf("tmdb: season identity unavailable")
	}
	q := url.Values{"api_key": {apiKey}, "language": {tmdbMetadataLanguage(ctx)}}
	u := fmt.Sprintf("%s/tv/%d/season/%d?%s", t.resolveBaseURL(ctx), tmdbID, season, q.Encode())
	var r struct {
		AirDate string `json:"air_date"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return 0, err
	}
	if len(r.AirDate) < 4 {
		return 0, nil
	}
	var year int
	_, err := fmt.Sscanf(r.AirDate[:4], "%d", &year)
	return year, err
}

func (t *TMDbProvider) GetTVEpisodeDetails(ctx context.Context, tmdbID, season, episode int) (*TMDbEpisodeDetails, error) {
	if tmdbID <= 0 || episode <= 0 {
		return nil, nil
	}
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := t.resolveBaseURL(ctx)
	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", tmdbMetadataLanguage(ctx))
	u := base + "/tv/" + fmt.Sprint(tmdbID) + "/season/" + fmt.Sprint(season) + "/episode/" + fmt.Sprint(episode) + "?" + q.Encode()
	var r struct {
		Name        string  `json:"name"`
		Overview    string  `json:"overview"`
		StillPath   string  `json:"still_path"`
		AirDate     string  `json:"air_date"`
		VoteAverage float32 `json:"vote_average"`
		Runtime     int     `json:"runtime"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	details := &TMDbEpisodeDetails{
		Name:     r.Name,
		Overview: r.Overview,
		Rating:   r.VoteAverage,
		Runtime:  r.Runtime,
	}
	if r.StillPath != "" {
		details.StillURL = t.imgCDN + "/w500" + r.StillPath
	}
	if len(r.AirDate) >= 4 {
		_, _ = fmt.Sscanf(r.AirDate[:4], "%d", &details.AirYear)
	}
	return details, nil
}

func (t *TMDbProvider) GetTVEpisodeCount(ctx context.Context, tmdbID int) (int, error) {
	if tmdbID <= 0 {
		return 0, nil
	}
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return 0, nil
	}
	base := t.resolveBaseURL(ctx)
	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", tmdbMetadataLanguage(ctx))
	u := base + "/tv/" + fmt.Sprint(tmdbID) + "?" + q.Encode()
	var r struct {
		NumberOfEpisodes int `json:"number_of_episodes"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return 0, err
	}
	return r.NumberOfEpisodes, nil
}
