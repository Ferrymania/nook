package service

import "strings"

// Per-run diagnostics carry safe, actionable reasons to the persisted queue.
// Never include provider request URLs, which may contain API credentials.
type scrapeLookupFailure struct {
	titleMismatch bool
	yearMismatch  bool
	typeMismatch  bool
	requestFailed bool
	ambiguous     bool
}

func (f *scrapeLookupFailure) merge(other scrapeLookupFailure) {
	if f == nil {
		return
	}
	f.titleMismatch = f.titleMismatch || other.titleMismatch
	f.yearMismatch = f.yearMismatch || other.yearMismatch
	f.typeMismatch = f.typeMismatch || other.typeMismatch
	f.requestFailed = f.requestFailed || other.requestFailed
	f.ambiguous = f.ambiguous || other.ambiguous
}

func (f *scrapeLookupFailure) reject(query string, year int, match *Match) {
	if f == nil || match == nil {
		return
	}
	if !automaticMetadataTitleTrusted(query, match) || unsafeAutomaticEpisodeQuery(query) {
		f.titleMismatch = true
	} else if !organizeMetadataMatchTrusted(query, year, match) {
		f.yearMismatch = true
	}
}

func (f scrapeLookupFailure) message() string {
	var reasons []string
	if f.ambiguous {
		return "多个候选符合名称和年份条件，请手动匹配"
	}
	if f.yearMismatch {
		reasons = append(reasons, "搜索到候选，但上映年份不符")
	}
	if f.titleMismatch {
		reasons = append(reasons, "搜索到候选，但标题或续集编号不符")
	}
	if f.typeMismatch {
		reasons = append(reasons, "搜索到候选，但电影/剧集类型不符")
	}
	if f.requestFailed {
		reasons = append(reasons, "元数据接口请求失败，请检查网络和 API 配置")
	}
	if len(reasons) == 0 {
		return "未搜索到匹配的元数据（搜索结果为空）"
	}
	return strings.Join(reasons, "；")
}
