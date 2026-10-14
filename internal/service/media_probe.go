package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// 探测结果的缓存与预算策略。
const (
	// 刮削中的单次探测预算包含换链；排队不占用这段预算。
	mediaProbeTimeout       = 30 * time.Second
	mediaProbeRecordTimeout = 3 * time.Second
	// mediaProbeFailureRetry 是探测失败后允许重新探测的间隔。失败的结果也会落库，
	// 避免重复刮削时不断读取同一个坏源。
	mediaProbeFailureRetry = 6 * time.Hour
	// 错误脱敏后的最大字符数，保持在 last_error 列宽内。
	mediaProbeErrorLimit = 200
)

// mediaProber 是 MediaProbeService 需要的探测能力。抽成接口是为了在测试里注入
// 桩，避免依赖真实 ffprobe 二进制。
type mediaProber interface {
	ProbeFull(ctx context.Context, input ProbeInput) (*FullProbeResult, error)
}

// MediaProbeService 用 ffprobe 提取媒体的基础信息（容器、每路轨道、内嵌章节）
// 并落库缓存。
//
// 刮削阶段补齐信息，播放与详情接口只读取已保存的结果。
//
// 它**不参与片头/片尾判定**：章节标题绝大多数没有语义（生产库实测抽样 64 个
// 文件，命中 0 个），拿它去猜跳过点只会给出错误的位置，时间轴数据仍然只信
// TheIntroDB。
type MediaProbeService struct {
	log         *zap.Logger
	repo        *repository.Container
	probe       mediaProber
	cache       *RuntimeCacheService
	remoteSlots chan struct{}
	// resolve 把 strm 播放目标解析成最终直链（含绑定 UA 的请求头）。与转码、
	// 内嵌字幕发现走同一条换链路径，否则会踩到网盘 CDN 的防盗链 403。
	resolve func(ctx context.Context, raw, userAgent string) (*StrmPlayResult, error)

	mu       sync.Mutex
	inFlight map[string]struct{}
}

// NewMediaProbeService is the constructor.
func NewMediaProbeService(log *zap.Logger, repo *repository.Container, probe *FFprobeService) *MediaProbeService {
	svc := &MediaProbeService{
		log:         log,
		repo:        repo,
		inFlight:    make(map[string]struct{}),
		remoteSlots: make(chan struct{}, 1),
	}
	if probe != nil {
		svc.probe = probe
	}
	return svc
}

// SetPlayTargetResolver injects the strm → direct-link resolver.
func (s *MediaProbeService) SetPlayTargetResolver(resolve func(ctx context.Context, raw, userAgent string) (*StrmPlayResult, error)) *MediaProbeService {
	if s != nil {
		s.resolve = resolve
	}
	return s
}

func (s *MediaProbeService) SetRuntimeCache(cache *RuntimeCacheService) *MediaProbeService {
	if s != nil {
		s.cache = cache
	}
	return s
}

// Ensure 在现有刮削工作协程中补齐缺失信息，不另建后台队列。
func (s *MediaProbeService) Ensure(ctx context.Context, m *model.Media) error {
	return s.probeMedia(ctx, m, false)
}

// Probe 是管理员显式重试，绕过成功缓存及失败冷却。
func (s *MediaProbeService) Probe(ctx context.Context, m *model.Media) error {
	return s.probeMedia(ctx, m, true)
}

func (s *MediaProbeService) probeMedia(ctx context.Context, m *model.Media, force bool) error {
	if s == nil || s.probe == nil || s.repo == nil || s.repo.MediaProbe == nil {
		return errors.New("media probe unavailable")
	}
	if m == nil || strings.TrimSpace(m.ID) == "" {
		return ErrMediaNotFound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.reserve(m.ID) {
		if force {
			return errors.New("media probe already running")
		}
		return nil
	}
	defer s.release(m.ID)
	if !force {
		row, err := s.repo.MediaProbe.Get(ctx, m.ID)
		if err != nil {
			return err
		}
		if mediaProbeSettled(row, m) {
			return nil
		}
	}
	if isStrmMediaRow(m) {
		select {
		case s.remoteSlots <- struct{}{}:
			defer func() { <-s.remoteSlots }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.run(ctx, m)
}

func mediaProbeSettled(row *model.MediaProbe, m *model.Media) bool {
	if row == nil || (row.Signature != "" && row.Signature != mediaProbeSignature(m)) {
		return false
	}
	var payload mediaProbePayload
	if json.Unmarshal([]byte(row.Payload), &payload) == nil && validProbeStreams(payload.Streams) {
		return true
	}
	return row.LastError != "" && time.Since(row.ProbedAt) < mediaProbeFailureRetry
}

func validProbeStreams(streams []ProbeStream) bool {
	for _, stream := range streams {
		if (stream.Type == "video" || stream.Type == "audio") && stream.Codec != "" && stream.Codec != "unknown" {
			return true
		}
	}
	return false
}

func (s *MediaProbeService) reserve(mediaID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight == nil {
		s.inFlight = make(map[string]struct{})
	}
	if _, running := s.inFlight[mediaID]; running {
		return false
	}
	s.inFlight[mediaID] = struct{}{}
	return true
}

func (s *MediaProbeService) release(mediaID string) {
	s.mu.Lock()
	delete(s.inFlight, mediaID)
	s.mu.Unlock()
}

// run 返回的错误与影片匹配状态独立，超时后仍留出短暂时间保存失败原因。
func (s *MediaProbeService) run(ctx context.Context, m *model.Media) (err error) {
	ctx, cancel := context.WithTimeout(ctx, mediaProbeTimeout)
	defer cancel()
	defer func() {
		if err != nil {
			s.markFailure(ctx, m, err)
			err = mediaProbeError{err}
		}
	}()

	input, err := s.probeInput(ctx, m)
	if err != nil {
		return err
	}
	result, err := s.probe.ProbeFull(ctx, input)
	if err != nil {
		return err
	}
	if result == nil || !validProbeStreams(result.Streams) {
		return errors.New("no audio or video stream metadata parsed")
	}
	return s.persistProbe(ctx, m, result)
}

// probeInput 把媒体行解析成 ffprobe 能直接打开的输入。
//
// 本地文件给路径；STRM / 云盘先解析成最终直链并带上绑定的请求头——直链与 UA
// 必须配套，用错会被 CDN 拒绝。
func (s *MediaProbeService) probeInput(ctx context.Context, m *model.Media) (ProbeInput, error) {
	return resolveMediaProbeInput(ctx, m, s.resolve)
}

func resolveMediaProbeInput(ctx context.Context, m *model.Media, resolve func(context.Context, string, string) (*StrmPlayResult, error)) (ProbeInput, error) {
	if m == nil {
		return ProbeInput{}, ErrMediaNotFound
	}
	if !isStrmMediaRow(m) {
		if _, err := os.Stat(m.Path); err != nil {
			return ProbeInput{}, ErrMediaNotFound
		}
		return ProbeInput{Source: m.Path}, nil
	}
	raw := strings.TrimSpace(m.STRMURL)
	if raw == "" && strings.HasSuffix(strings.ToLower(strings.TrimSpace(m.Path)), ".strm") {
		parsed, err := readLocalSTRMTarget(m.Path)
		if err == nil {
			raw = strings.TrimSpace(parsed)
		}
	}
	if raw == "" {
		return ProbeInput{}, errors.New("strm play target missing")
	}
	if resolve != nil {
		resolved, err := resolve(ctx, raw, "")
		if err != nil {
			return ProbeInput{}, err
		}
		in, err := transcodeInputFromPlayResult(resolved)
		if err != nil {
			return ProbeInput{}, err
		}
		return ProbeInput{Source: in.Source, Headers: in.Headers}, nil
	}
	if isHTTPPlaybackTarget(raw) {
		return ProbeInput{Source: raw}, nil
	}
	return ProbeInput{}, errors.New("strm probe source unavailable")
}

// persistProbe 把一次成功的探测落库。
func (s *MediaProbeService) persistProbe(ctx context.Context, m *model.Media, result *FullProbeResult) error {
	if s.repo == nil || s.repo.MediaProbe == nil {
		return errors.New("media probe repository not wired")
	}
	row, err := mediaProbeRow(m, result)
	if err != nil {
		return err
	}
	if err := s.repo.MediaProbe.Upsert(ctx, row); err != nil {
		return err
	}
	updates := map[string]any{
		"width": row.Width, "height": row.Height,
		"video_codec": row.VideoCodec, "audio_codec": row.AudioCodec,
	}
	if result.DurationSec > 0 {
		updates["duration_sec"] = result.DurationSec
	}
	if result.Container != "" {
		updates["container"] = result.Container
	}
	if result.SizeBytes > 0 {
		updates["size_bytes"] = result.SizeBytes
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", m.ID).Updates(updates).Error; err != nil {
		return err
	}
	if s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
	return nil
}

func mediaProbeRow(m *model.Media, result *FullProbeResult) (*model.MediaProbe, error) {
	payload, err := result.PayloadJSON()
	if err != nil {
		return nil, err
	}
	return &model.MediaProbe{
		MediaID:         m.ID,
		Signature:       mediaProbeSignature(m),
		Source:          mediaProbeInputKind(m),
		Container:       result.Container,
		DurationSec:     result.DurationSec,
		BitRate:         result.BitRate,
		Width:           firstStreamDimension(result, "video", true),
		Height:          firstStreamDimension(result, "video", false),
		VideoCodec:      firstStreamCodec(result, "video"),
		AudioCodec:      firstStreamCodec(result, "audio"),
		VideoStreams:    len(result.StreamsOfType("video")),
		AudioStreams:    len(result.StreamsOfType("audio")),
		SubtitleStreams: len(result.StreamsOfType("subtitle")),
		ChapterCount:    len(result.Chapters),
		Payload:         payload,
		ProbedAt:        time.Now(),
	}, nil
}

func (s *MediaProbeService) markFailure(ctx context.Context, m *model.Media, probeErr error) {
	if s == nil || s.repo == nil || probeErr == nil {
		return
	}
	message := truncateProbeError(probeErr)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaProbeRecordTimeout)
	defer cancel()
	row, err := s.repo.MediaProbe.Get(ctx, m.ID)
	if err == nil {
		if row == nil || row.Signature != mediaProbeSignature(m) {
			// 文件变更后旧信息已失效；不能把旧 Payload 记成新文件的结果。
			err = s.repo.MediaProbe.Upsert(ctx, &model.MediaProbe{
				MediaID: m.ID, Signature: mediaProbeSignature(m), Source: mediaProbeInputKind(m),
				ProbedAt: time.Now(), LastError: message,
			})
		} else {
			err = s.repo.MediaProbe.MarkFailure(ctx, m.ID, message, time.Now())
		}
	}
	if err != nil && s.log != nil {
		s.log.Warn("record media probe failure failed", zap.String("media_id", m.ID), zap.Error(redactSensitiveError(err)))
	}
	if s.log != nil {
		s.log.Warn("media probe failed", zap.String("media_id", m.ID), zap.String("error", message))
	}
}

var mediaProbeURLPattern = regexp.MustCompile(`(?i)\b(?:https?|cloud)://[^\s"'<>]+`)

type mediaProbeError struct{ cause error }

func (e mediaProbeError) Error() string { return truncateProbeError(e.cause) }
func (e mediaProbeError) Unwrap() error { return e.cause }

// 错误在日志、数据库和 API 响应中都不保留可播放的签名链接。
func truncateProbeError(err error) string {
	message := mediaProbeURLPattern.ReplaceAllString(redactSensitiveText(strings.TrimSpace(err.Error())), "[redacted URL]")
	runes := []rune(message)
	if len(runes) > mediaProbeErrorLimit {
		return string(runes[:mediaProbeErrorLimit])
	}
	return message
}

// mediaProbeInputKind 记录输入形态，供详情页判断「这个时长是本地读的还是远端读的」。
func mediaProbeInputKind(m *model.Media) string {
	if m == nil {
		return ""
	}
	if isStrmMediaRow(m) {
		return "strm"
	}
	return "local"
}

// mediaProbeSignature 是「探的是哪个文件」的指纹。
//
// 本地文件用路径 + 大小 + 修改时间；STRM / 云盘没有本地文件，用固化的播放目标，
// 且绝不能用解析后的直链（每次签名都不同，缓存会永远失效）。整体做哈希：
// 既固定长度，也不把路径或 pickcode 再抄一份进数据库。
func mediaProbeSignature(m *model.Media) string {
	if m == nil {
		return ""
	}
	var base string
	if isStrmMediaRow(m) {
		target := MediaSTRMTarget(m)
		if target == "" {
			target = strings.TrimSpace(m.Path)
		}
		base = "strm|" + target
	} else if info, err := os.Stat(m.Path); err == nil {
		base = fmt.Sprintf("local|%s|%d|%d", m.Path, info.Size(), info.ModTime().UnixNano())
	} else {
		base = "local|" + m.Path
	}
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:16])
}

func firstStreamCodec(result *FullProbeResult, kind string) string {
	if result == nil {
		return ""
	}
	for _, stream := range result.Streams {
		if stream.Type == kind && stream.Codec != "" {
			return stream.Codec
		}
	}
	return ""
}

// firstStreamDimension 取第一路指定类型轨道的宽（width=true）或高。
func firstStreamDimension(result *FullProbeResult, kind string, width bool) int {
	if result == nil {
		return 0
	}
	for _, stream := range result.Streams {
		if stream.Type != kind {
			continue
		}
		if width {
			return stream.Width
		}
		return stream.Height
	}
	return 0
}
