package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// movieLibraryHasEpisodicContent 报告电影类型库里是否混入了「剧集结构」内容
// (有季集号且路径形如剧集,例如 .../国产剧/某剧/Season 01/某剧 - S01E01.mkv)。
// 用于决定是否需要走 movieLibraryItems 把这些内容聚成 Series 卡片。普通电影库
// 没有这类行时返回 false,继续走常规 mediaItems。
func (e *EmbyService) movieLibraryHasEpisodicContent(ctx context.Context, libraryID string) (bool, error) {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		return false, nil
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", []string{libraryID}).
		Where("(season_num > 0 OR episode_num > 0) AND ("+clause+")", args...)
	var count int64
	if err := q.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// movieLibraryItems 处理电影类型库的常规浏览,返回「真正的电影(Movie)」与
// 「库内剧集结构内容聚成的 Series 卡片」的合并列表(按 DateCreated 倒序分页)。
// 与 mediaItems 的区别: 后者会把剧集结构行当散装 Episode 漏出;这里改为聚合成
// Series,从根本上消除「电影库里整部剧被拆成单集」的现象。
func (e *EmbyService) movieLibraryItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	cacheKey := e.embyItemsCacheKey("movie-library-items-v1", p)
	var cached embyItemsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		e.rememberArtworkRefs(cached.Artwork)
		return map[string]any{
			"Items":            cached.Items,
			"TotalRecordCount": int(cached.TotalRecordCount),
			"StartIndex":       cached.StartIndex,
		}, nil
	}

	libIDs := []string{p.ParentID}
	apply := func(q *gorm.DB) *gorm.DB {
		q = e.applyUserMediaVisibility(ctx, q, p.UserID)
		if p.ParentID != "" {
			q = q.Where("library_id IN ?", libIDs)
		}
		if p.SearchTerm != "" {
			// 方言相关的 ILIKE/LIKE：PostgreSQL 的 LIKE 区分大小写，
			// 直接用 LIKE 会让客户端搜索的大小写与入库标题不一致时搜不到。
			likeOp := repository.LikeOperator(e.repo.DB)
			term := "%" + p.SearchTerm + "%"
			q = q.Where("title "+likeOp+" ? OR original_name "+likeOp+" ?", term, term)
		}
		if containsEmbyFilter(p.Filters, "IsFavorite") {
			if strings.TrimSpace(p.UserID) == "" {
				return nil
			}
			q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
		}
		return q
	}

	// 剧集结构内容 -> Series 卡片。
	clause, args := embyLikelyEpisodicPathSQL()
	var episodicRows []model.Media
	if clause != "" {
		epQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
		if epQ == nil {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		episodicClause := clause
		if ids := e.episodicLibraryIDs(ctx); len(ids) > 0 {
			episodicClause += " OR library_id IN ?"
			args = append(args, ids)
		}
		epQ = epQ.Where("(season_num > 0 OR episode_num > 0) AND ("+episodicClause+")", args...).
			Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit)
		if err := epQ.Find(&episodicRows).Error; err != nil {
			return nil, err
		}
	}
	seriesGroups := e.seriesGroupsFromMedia(ctx, episodicRows)

	// 真正的电影 -> Movie 项(剔除剧集结构行)。
	movieQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
	if movieQ == nil {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
	}
	movieQ = e.filterMovieItems(ctx, movieQ).
		Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit)
	var movieRows []model.Media
	if err := movieQ.Find(&movieRows).Error; err != nil {
		return nil, err
	}
	// 先按版本去重，再参与排序。这里不立即构建 payload：大电影库可能有
	// 数万行，而客户端一页通常只要几十条，提前构建会触发大量 NFO / 数据
	// 查询并把响应时间浪费在用户根本看不到的条目上。
	movieRows = e.collapseMediaVersionRows(ctx, movieRows)

	// Merge before sorting/paging, so a mixed-type page includes both kinds.
	type entry struct {
		sortAt time.Time
		name   string
		rating float32
		media  *model.Media
		group  *embySeriesGroup
	}
	entries := make([]entry, 0, len(seriesGroups)+len(movieRows))
	for i := range seriesGroups {
		group := &seriesGroups[i]
		at := embySeriesReleaseSortTime(*group)
		switch primarySupportedEmbySort(p.SortBy, false) {
		case "datecreated":
			at = group.CreatedAt
		case "datelastcontentadded", "datelastmediaadded":
			at = group.DateLastMediaAdded
		}
		entries = append(entries, entry{sortAt: at, name: group.Name, rating: group.Rating, group: group})
	}
	for i := range movieRows {
		media := &movieRows[i]
		at := embyMediaReleaseSortTime(*media)
		switch primarySupportedEmbySort(p.SortBy, false) {
		case "datecreated", "datelastcontentadded", "datelastmediaadded":
			at = media.CreatedAt
		}
		entries = append(entries, entry{sortAt: at, name: media.Title, rating: media.Rating, media: media})
	}
	sortKey := primarySupportedEmbySort(p.SortBy, false)
	ascending := strings.EqualFold(firstCSVValue(p.SortOrder), "Ascending") ||
		(strings.TrimSpace(p.SortOrder) == "" && (sortKey == "sortname" || sortKey == "name"))
	sort.SliceStable(entries, func(i, j int) bool {
		if ascending {
			i, j = j, i
		}
		switch sortKey {
		case "sortname", "name":
			return strings.ToLower(entries[i].name) > strings.ToLower(entries[j].name)
		case "communityrating":
			return entries[i].rating > entries[j].rating
		}
		return entries[i].sortAt.After(entries[j].sortAt)
	})
	total := len(entries)
	paged := pageSlice(entries, p.StartIndex, p.Limit)

	pageMovies := make([]model.Media, 0, len(paged))
	for _, en := range paged {
		if en.media != nil {
			pageMovies = append(pageMovies, *en.media)
		}
	}
	moviePayloads, err := e.payloadsForMedia(ctx, pageMovies, p.UserID)
	if err != nil {
		return nil, err
	}
	payloadByID := make(map[string]map[string]any, len(moviePayloads))
	for _, item := range moviePayloads {
		if id, ok := item["Id"].(string); ok {
			payloadByID[id] = item
		}
	}

	items := make([]map[string]any, 0, len(paged))
	pageGroups := make([]embySeriesGroup, 0, len(paged))
	for _, en := range paged {
		switch {
		case en.group != nil:
			pageGroups = append(pageGroups, *en.group)
			items = append(items, e.seriesPayload(*en.group))
		case en.media != nil:
			if item := payloadByID[en.media.ID]; item != nil {
				items = append(items, item)
			}
		}
	}
	out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{
			Items:            items,
			TotalRecordCount: int64(total),
			StartIndex:       p.StartIndex,
			Artwork:          e.artworkRefsForSeriesGroups(pageGroups),
		}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
	}
	return out, nil
}

// embyPayloadCreatedAt 从 item payload 里取 DateCreated(time.Time),用于合并排序。
func embyPayloadCreatedAt(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	if v, ok := item["DateCreated"].(time.Time); ok {
		return v
	}
	return time.Time{}
}

func embyPayloadReleaseSortTime(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	if v, ok := item["PremiereDate"].(time.Time); ok {
		return v
	}
	if year, ok := item["ProductionYear"].(int); ok && year > 0 {
		return time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	}
	return embyPayloadCreatedAt(item)
}

func (e *EmbyService) libraryIsEpisodic(ctx context.Context, libraryID string) (bool, error) {
	if strings.TrimSpace(libraryID) == "" {
		return false, nil
	}
	// 走请求级缓存（若有），避免同一请求内对同一库重复查表。
	if typ, ok, err := e.payloadLibraryType(ctx, libraryID); err != nil {
		return false, err
	} else if ok {
		return embyLibraryTypeIsEpisodic(typ), nil
	}
	var count int64
	err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND (season_num > 0 OR episode_num > 0)", []string{libraryID}).
		Count(&count).Error
	return count > 0, err
}

func (e *EmbyService) mediaBelongsToEpisodicLibrary(ctx context.Context, m *model.Media) bool {
	if e == nil || m == nil || strings.TrimSpace(m.LibraryID) == "" {
		return false
	}
	typ, ok, err := e.payloadLibraryType(ctx, m.LibraryID)
	if err != nil || !ok {
		return false
	}
	return embyLibraryTypeIsEpisodic(typ)
}

func (e *EmbyService) mediaShouldBeEpisode(ctx context.Context, m *model.Media) bool {
	if m == nil || (m.SeasonNum <= 0 && m.EpisodeNum <= 0) {
		return false
	}
	if e.mediaBelongsToEpisodicLibrary(ctx, m) {
		return true
	}
	return embyMediaPathLooksEpisodic(m.Path)
}

func embyLibraryTypeIsEpisodic(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}

func (e *EmbyService) filterMovieItems(ctx context.Context, q *gorm.DB) *gorm.DB {
	episodicIDs := e.episodicLibraryIDs(ctx)
	if len(episodicIDs) == 0 {
		return filterLikelyEpisodicPathsFromMovieQuery(q)
	}
	q = q.Where("(media.season_num = 0 AND media.episode_num = 0) OR media.library_id NOT IN ?", episodicIDs)
	return filterLikelyEpisodicPathsFromMovieQuery(q)
}

func (e *EmbyService) filterEpisodeItems(ctx context.Context, q *gorm.DB) *gorm.DB {
	episodicIDs := e.episodicLibraryIDs(ctx)
	if len(episodicIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("media.library_id IN ? AND (media.season_num > 0 OR media.episode_num > 0)", episodicIDs)
}

func (e *EmbyService) episodicLibraryIDs(ctx context.Context) []string {
	if e == nil || e.repo == nil || e.repo.DB == nil {
		return nil
	}
	var ids []string
	if err := e.repo.DB.WithContext(ctx).Model(&model.Library{}).
		Where("LOWER(type) IN ?", []string{"tv", "anime", "variety"}).
		Pluck("id", &ids).Error; err != nil {
		return nil
	}
	return ids
}

func filterLikelyEpisodicPathsFromMovieQuery(q *gorm.DB) *gorm.DB {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		return q
	}
	return q.Where("NOT ((media.season_num > 0 OR media.episode_num > 0) AND ("+clause+"))", args...)
}

// embyEpisodicPathExpr normalizes path separators before matching episode folders.
const embyEpisodicPathExpr = `LOWER(REPLACE(media.path, '\', '/'))`

func embyLikelyEpisodicPathSQL() (string, []any) {
	patterns := []string{
		"%/season %/%", "%/season.%/%", "%/season-%/%", "%/season_%/%",
		"%/s0%/%", "%/s1%/%", "%/s2%/%", "%/s3%/%", "%/s4%/%", "%/s5%/%", "%/s6%/%", "%/s7%/%", "%/s8%/%", "%/s9%/%",
		"%/special/%", "%/specials/%", "%/sp/%", "%/ova/%", "%/ovas/%", "%/oad/%", "%/oads/%", "%/ovd/%", "%/ovds/%", "%/ona/%", "%/onas/%", "%/extra/%", "%/extras/%", "%/bonus/%", "%/bonuses/%", "%/omake/%", "%/picture drama/%", "%/ncop/%", "%/nced/%",
		"%/电视剧/%", "%/剧集/%", "%/连续剧/%", "%/短剧/%", "%/国产剧/%", "%/国剧/%", "%/大陆剧/%", "%/华语剧/%", "%/国产电视剧/%", "%/大陆电视剧/%", "%/华语电视剧/%", "%/欧美剧/%", "%/欧美电视剧/%", "%/美剧/%", "%/英剧/%", "%/日韩剧/%", "%/日韩电视剧/%", "%/日剧/%", "%/韩剧/%", "%/港剧/%", "%/台剧/%", "%/港台剧/%", "%/泰剧/%",
		"%/日番/%", "%/国漫/%", "%/番剧/%", "%/动漫/%", "%/特别篇/%", "%/特別篇/%", "%/番外/%", "%/特典/%",
	}
	clauses := make([]string, 0, len(patterns))
	args := make([]any, 0, len(patterns))
	for _, pattern := range patterns {
		clauses = append(clauses, embyEpisodicPathExpr+" LIKE ?")
		args = append(args, pattern)
	}
	return strings.Join(clauses, " OR "), args
}

func embyMediaPathLooksEpisodic(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/"))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"/season ", "/season.", "/season-", "/season_", "/special/", "/specials/", "/sp/", "/ova/", "/ovas/", "/oad/", "/oads/", "/ovd/", "/ovds/", "/ona/", "/onas/", "/extra/", "/extras/", "/bonus/", "/bonuses/", "/omake/", "/picture drama/", "/ncop/", "/nced/",
		"/电视剧/", "/剧集/", "/连续剧/", "/短剧/", "/国产剧/", "/国剧/", "/大陆剧/", "/华语剧/", "/国产电视剧/", "/大陆电视剧/", "/华语电视剧/", "/欧美剧/", "/欧美电视剧/", "/美剧/", "/英剧/", "/日韩剧/", "/日韩电视剧/", "/日剧/", "/韩剧/", "/港剧/", "/台剧/", "/港台剧/", "/泰剧/",
		"/日番/", "/国漫/", "/番剧/", "/动漫/", "/特别篇/", "/特別篇/", "/番外/", "/特典/",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	for _, marker := range []string{"/s0", "/s1", "/s2", "/s3", "/s4", "/s5", "/s6", "/s7", "/s8", "/s9"} {
		if idx := strings.Index(normalized, marker); idx >= 0 {
			after := idx + len(marker)
			if after < len(normalized) && normalized[after] >= '0' && normalized[after] <= '9' {
				slash := after + 1
				if slash < len(normalized) && normalized[slash] == '/' {
					return true
				}
			}
		}
	}
	return false
}
