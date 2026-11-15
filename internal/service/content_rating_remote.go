package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

// Filter upstream metadata before paginating. Upstream age-rating algorithms
// vary by country, so requesting MaxOfficialRating alone is not sufficient.
func (r *EmbyRemoteService) doGet(ctx context.Context, acct *model.StrmAccount, cfg *EmbyRemoteConfig, path string, q url.Values, out any) error {
	policy := contentRatingPolicyFromContext(ctx)
	if strings.Contains(path, "/PlaybackInfo") {
		return r.doGetUnfiltered(ctx, acct, cfg, path, q, out)
	}
	if !policy.Restricted() || !(strings.Contains(path, "/Items") || strings.Contains(path, "/Shows/")) {
		return r.doGetUnfiltered(ctx, acct, cfg, path, q, out)
	}
	query := url.Values{}
	for key, values := range q {
		query[key] = append([]string(nil), values...)
	}
	query.Set("Fields", query.Get("Fields")+",OfficialRating,SeriesOfficialRating")
	list := strings.HasSuffix(path, "/Items") || strings.HasSuffix(path, "/Episodes") || strings.HasSuffix(path, "/Seasons")
	if !list {
		var raw json.RawMessage
		if err := r.doGetUnfiltered(ctx, acct, cfg, path, query, &raw); err != nil {
			return err
		}
		var items []map[string]any
		if json.Unmarshal(raw, &items) == nil {
			kept := make([]map[string]any, 0, len(items))
			for _, item := range items {
				if remoteRatingAllowed(ctx, item) {
					kept = append(kept, item)
				}
			}
			data, err := json.Marshal(kept)
			if err != nil {
				return err
			}
			return json.Unmarshal(data, out)
		}
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		if !remoteRatingAllowed(ctx, item) {
			return ErrEmbyRemoteNotFound
		}
		return json.Unmarshal(raw, out)
	}
	offset, _ := strconv.Atoi(query.Get("StartIndex"))
	limit, _ := strconv.Atoi(query.Get("Limit"))
	if offset < 0 {
		offset = 0
	}
	kept := make([]map[string]any, 0)
	cacheQuery := url.Values{}
	for key, values := range query {
		cacheQuery[key] = append([]string(nil), values...)
	}
	cacheQuery.Del("StartIndex")
	cacheQuery.Del("Limit")
	accountID := ""
	if acct != nil {
		accountID = acct.ID
	}
	cacheKey := r.remoteCacheKey(ctx, "rating-catalogue", accountID, cfg.BaseURL, path, cacheQuery.Encode())
	cached := r.cache != nil && r.cache.GetJSON(ctx, cacheKey, &kept)
	const batch = 200
	for start := 0; !cached; start += batch {
		if start >= 50000 {
			return errors.New("远程媒体库超过分级筛选的 50000 条处理上限")
		}
		query.Set("StartIndex", strconv.Itoa(start))
		query.Set("Limit", strconv.Itoa(batch))
		var page struct {
			Items            []map[string]any
			TotalRecordCount int
		}
		if err := r.doGetUnfiltered(ctx, acct, cfg, path, query, &page); err != nil {
			return err
		}
		for _, item := range page.Items {
			if remoteRatingAllowed(ctx, item) {
				kept = append(kept, item)
			}
		}
		if len(page.Items) < batch || start+len(page.Items) >= page.TotalRecordCount {
			break
		}
	}
	if !cached && r.cache != nil {
		r.cache.SetJSON(ctx, cacheKey, kept, r.remoteMediaCacheTTL())
	}
	total := len(kept)
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	data, err := json.Marshal(map[string]any{"Items": kept[offset:end], "TotalRecordCount": total, "StartIndex": offset})
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
