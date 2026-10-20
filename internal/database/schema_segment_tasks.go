package database

import "gorm.io/gorm"

// Recover the latest known successful/missing lookup on upgrade, without making
// provider requests or inventing a trigger, duration, or historical failures.
// Check even soft-deleted tasks so clearing history stays effective on restart.
func backfillSegmentTasks(db *gorm.DB) error {
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_segment_tasks_active_time
ON media_segment_tasks (created_at DESC, id DESC) WHERE deleted_at IS NULL`).Error; err != nil {
		return err
	}
	return db.Exec(`INSERT INTO media_segment_tasks
  (id, created_at, updated_at, media_id, library_id, library_name, media_title,
   media_path, season_num, episode_num, provider, trigger, status, error, segments, finished_at)
SELECT f.id, f.fetched_at, f.fetched_at, m.id, m.library_id, COALESCE(l.name, ''),
       m.title, m.path, m.season_num, m.episode_num, f.source, 'history',
       CASE WHEN f.found THEN 'found' ELSE 'missing' END, '',
       COALESCE((SELECT json_agg(json_build_object('kind', s.kind, 'start_ms', s.start_ms, 'end_ms', s.end_ms)
                                ORDER BY s.start_ms)::text
                 FROM media_segments s WHERE s.media_id = m.id AND s.source = f.source AND s.deleted_at IS NULL), '[]'),
       f.fetched_at
FROM media_segment_fetches f
JOIN media m ON m.id = f.media_id AND m.deleted_at IS NULL
LEFT JOIN libraries l ON l.id = m.library_id AND l.deleted_at IS NULL
WHERE f.deleted_at IS NULL AND f.source = 'theintrodb'
  AND NOT EXISTS (SELECT 1 FROM media_segment_tasks t WHERE t.media_id = m.id AND t.provider = f.source)
ON CONFLICT (id) DO NOTHING`).Error
}
