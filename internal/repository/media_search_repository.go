package repository

import (
	"context"
	"strings"
	"unicode"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

// Search runs a LIKE search against the title field. Empty query returns the
// most recently added items.
func (r *MediaRepository) Search(ctx context.Context, query string, limit int) ([]model.Media, error) {
	return r.SearchFiltered(ctx, query, limit, MediaQueryFilter{IncludeNSFW: true})
}

func (r *MediaRepository) SearchFiltered(ctx context.Context, query string, limit int, filter MediaQueryFilter) ([]model.Media, error) {
	items, _, err := r.SearchFilteredPage(ctx, query, 0, limit, filter)
	return items, err
}

func (r *MediaRepository) SearchFilteredPage(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 50
	}
	return r.searchFilteredLIKE(ctx, query, offset, limit, filter)
}

func (r *MediaRepository) searchFilteredLIKE(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	var items []model.Media
	var total int64
	q := r.db.WithContext(ctx).Model(&model.Media{})
	q = applyMediaQueryFilter(q, filter)
	// PostgreSQL ILIKE keeps title searches case insensitive.
	likeOp := likeOperator(r.db)
	terms := mediaSearchTerms(query)
	for _, term := range terms {
		like := "%" + escapeLike(term) + "%"
		q = q.Where(
			"(title "+likeOp+" ? ESCAPE '\\' OR original_name "+likeOp+" ? ESCAPE '\\' OR path "+likeOp+" ? ESCAPE '\\' OR genres "+likeOp+" ? ESCAPE '\\')",
			like, like, like, like,
		)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if query != "" {
		prefix := escapeLike(query) + "%"
		exact := query
		q = q.Order(gorm.Expr(
			"CASE WHEN title = ? THEN 0 WHEN original_name = ? THEN 1 WHEN title "+likeOp+" ? ESCAPE '\\' THEN 2 WHEN original_name "+likeOp+" ? ESCAPE '\\' THEN 3 ELSE 4 END, created_at desc",
			exact, exact, prefix, prefix,
		))
	} else {
		q = q.Order("created_at desc")
	}
	err := q.Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func mediaSearchTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		lower := strings.ToLower(field)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		out = append(out, field)
	}
	return out
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}
