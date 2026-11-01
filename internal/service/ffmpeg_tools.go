package service

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/Ferrymania/nook/internal/config"
)

// FFmpegToolsService checks tools without downloading or changing configuration.
type FFmpegToolsService struct {
	cfg *config.Config
}

func NewFFmpegToolsService(cfg *config.Config) *FFmpegToolsService {
	return &FFmpegToolsService{cfg: cfg}
}

type ffToolInfo struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

func inspectFFTool(ctx context.Context, path, name string) ffToolInfo {
	if path == "" {
		return ffToolInfo{}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-version").Output() // #nosec G204 -- locally resolved executable; no shell is used.
	if err != nil {
		return ffToolInfo{Path: path}
	}
	version := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
	return ffToolInfo{Installed: strings.HasPrefix(strings.ToLower(version), name+" version "), Path: path, Version: version}
}

// Detect checks configured executables and PATH without applying any changes.
func (s *FFmpegToolsService) Detect(ctx context.Context) map[string]ffToolInfo {
	config.RuntimeMu.RLock()
	configured := map[string]string{"ffmpeg": s.cfg.App.FFmpegPath, "ffprobe": s.cfg.App.FFprobePath}
	config.RuntimeMu.RUnlock()
	found := map[string]ffToolInfo{"ffmpeg": {}, "ffprobe": {}}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		for _, path := range executableCandidates(configured[name], name) {
			if ctx.Err() != nil {
				return found
			}
			if info := inspectFFTool(ctx, path, name); info.Installed {
				found[name] = info
				break
			}
		}
	}
	return found
}

// Status reports only the active tools. Discovery candidates are applied only
// after the administrator explicitly saves their paths in settings.
func (s *FFmpegToolsService) Status(ctx context.Context) map[string]ffToolInfo {
	config.RuntimeMu.RLock()
	ffmpegPath, _ := resolveLocalExecutable(s.cfg.App.FFmpegPath, "ffmpeg")
	ffprobePath, _ := resolveLocalExecutable(s.cfg.App.FFprobePath, "ffprobe")
	config.RuntimeMu.RUnlock()
	return map[string]ffToolInfo{
		"ffmpeg":  inspectFFTool(ctx, ffmpegPath, "ffmpeg"),
		"ffprobe": inspectFFTool(ctx, ffprobePath, "ffprobe"),
	}
}
