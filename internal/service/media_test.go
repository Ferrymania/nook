package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAccessibleLibraryPathMapsConfiguredHostMediaDir(t *testing.T) {
	root := t.TempDir()
	hostRoot := filepath.Join(root, "nas", "host", "media")
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_MEDIA_DIR", hostRoot)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(filepath.Join(hostRoot, "电视剧", "国产剧"))
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathMapsWindowsDriveBeforeLinuxAbs(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_MEDIA_DIR", `Q:\media`)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerRoot)

	for _, input := range []string{
		`Q:\media\电视剧\国产剧`,
		`Q:/media/电视剧/国产剧`,
		`/app/Q:\media\电视剧\国产剧`,
		`/app/Q:/media/电视剧/国产剧`,
	} {
		t.Run(input, func(t *testing.T) {
			got, err := resolveAccessibleLibraryPath(input)
			if err != nil {
				t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
			}
			if got != filepath.Clean(containerLibrary) {
				t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
			}
		})
	}
}

func TestResolveAccessibleLibraryPathRecoversDockerPollutedWindowsDrive(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_MEDIA_DIR", `F:\media`)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(`/app/F:\media\电视剧\国产剧`)
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathKeepsAccessibleContainerPath(t *testing.T) {
	containerLibrary := filepath.Join(t.TempDir(), "media", "电影")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveAccessibleLibraryPath(containerLibrary)
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathMapsRelativeDockerMediaMarker(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(filepath.Join("media", "电视剧", "国产剧"))
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

// 旧库把宿主机绝对路径整段写进来时（moviepilot 布局：/vol1/.../media/电视剧/国产剧），
// /media 出现在路径中段而非开头；解析必须能把最后一个 /media 段之后的尾巴映射到容器媒体
// 目录，否则新版容器只挂了 /media 就永远扫不出这类旧库。钉死该行为，防止再退化。
func TestResolveAccessibleLibraryPathMapsEmbeddedHostMediaMarker(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath("/vol1/1000/Docker/moviepilot-v2/media/电视剧/国产剧")
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

// 中段 marker 启发式绝不能污染目的地解析：resolveMappedDestinationPath 不校验存在性、
// 会返回首个候选，一旦启发式并进来，就会把形如 <tmp>/media/... 的合法整理目的地错误
// 重写到容器根 /media（曾导致 organizer 跨盘 hardlink 失败）。钉死该边界。
func TestResolveMappedDestinationPathIgnoresEmbeddedMediaMarker(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "001", "media", "电视剧", "国产剧", "Some Show")
	if got := resolveMappedDestinationPath(dst); got != filepath.Clean(dst) {
		t.Fatalf("resolveMappedDestinationPath() = %q, want %q (embedded /media must not remap destinations)", got, filepath.Clean(dst))
	}
}

func TestInferLibraryKindFromCategoryPathOverridesMovieDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "国产剧", path: `/media/电视剧/国产剧`, want: "tv"},
		{name: "日漫", path: `/media/电视剧/日漫`, want: "anime"},
		{name: "综艺", path: `/media/电视剧/综艺`, want: "variety"},
		{name: "成人", path: `/media/成人`, want: "adult"},
		{name: "动画电影", path: `/media/电影/动画电影`, want: "movie"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferLibraryKind(tc.name, tc.path, "movie"); got != tc.want {
				t.Fatalf("inferLibraryKind() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInferLibraryKindMatchesCategoryWords(t *testing.T) {
	for _, tc := range []struct {
		name      string
		path      string
		requested string
		want      string
	}{
		{name: "综合", path: "/var/folders/6tvttyb151/T/library", requested: "movie", want: "movie"},
		{name: "综合", path: "/var/folders/6tvttyb151/T/library", requested: "tv", want: "tv"},
		{name: "综合", path: "/srv/activity/library", requested: "movie", want: "movie"},
		{name: "综合", path: "/srv/filmstrip/library", requested: "tv", want: "tv"},
		{name: "综合", path: "/srv/adulthood/library", requested: "movie", want: "movie"},
		{name: "综合", path: "/srv/animecache/library", requested: "movie", want: "movie"},
		{name: "综合", path: "/media/TV Shows/collection", requested: "movie", want: "tv"},
		{name: "综合", path: `D:\media\TV\collection`, requested: "movie", want: "tv"},
		{name: "My Movies", path: "/media/collection", requested: "tv", want: "movie"},
		{name: "综合", path: "/media/Films/collection", requested: "tv", want: "movie"},
	} {
		t.Run(tc.path+"/"+tc.requested, func(t *testing.T) {
			if got := inferLibraryKind(tc.name, tc.path, tc.requested); got != tc.want {
				t.Fatalf("inferLibraryKind(%q, %q, %q) = %q, want %q", tc.name, tc.path, tc.requested, got, tc.want)
			}
		})
	}
}

func TestResolveMappedDestinationPathPrefersConfiguredContainerMapping(t *testing.T) {
	root := t.TempDir()
	containerMedia := filepath.Join(root, "container", "media")
	if err := os.MkdirAll(containerMedia, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_MEDIA_DIR", `Q:\media`)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerMedia)

	for _, input := range []string{`Q:\media`, `Q:/media`, `/app/Q:\media`} {
		t.Run(input, func(t *testing.T) {
			got := resolveMappedDestinationPath(input)
			if got != filepath.Clean(containerMedia) {
				t.Fatalf("resolveMappedDestinationPath() = %q, want %q", got, filepath.Clean(containerMedia))
			}
		})
	}
}

func TestResolveMappedDestinationPathPrefersContainerForRelativeMediaMarker(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	containerMedia := filepath.Join(root, "container", "media")
	if err := os.MkdirAll(filepath.Join(workDir, "media", "电视剧", "国产剧"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(containerMedia, "电视剧", "国产剧"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workDir)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerMedia)

	got := resolveMappedDestinationPath(filepath.Join("media", "电视剧", "国产剧"))
	want := filepath.Join(containerMedia, "电视剧", "国产剧")
	if got != filepath.Clean(want) {
		t.Fatalf("resolveMappedDestinationPath() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestMediaPathsIgnoreRetiredDownloadMapping(t *testing.T) {
	root := t.TempDir()
	hostRoot := filepath.Join(root, "host", "downloads")
	containerRoot := filepath.Join(root, "container", "downloads")
	containerSource := filepath.Join(containerRoot, "国产剧")
	if err := os.MkdirAll(containerSource, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOOK_DOWNLOAD_DIR", hostRoot)
	t.Setenv("NOOK_DOWNLOAD_CONTAINER_DIR", containerRoot)
	t.Chdir(root)
	for _, input := range []string{
		filepath.Join(hostRoot, "国产剧"),
		filepath.Join(root, "nas", "downloads", "国产剧"),
		`F:\downloads\国产剧`,
		filepath.Join("downloads", "国产剧"),
	} {
		t.Run(input, func(t *testing.T) {
			for _, candidate := range append(mappedPathCandidates(input), embeddedContainerMarkerCandidates(input)...) {
				if candidate == containerSource {
					t.Fatalf("retired download mapping still resolves %q to %q", input, candidate)
				}
			}
			if _, _, err := resolveAccessibleMappedPath(input); err == nil {
				t.Fatalf("retired download mapping unexpectedly resolved %q", input)
			}
			if _, err := resolveAccessibleLibraryPath(input); err == nil {
				t.Fatalf("retired library download mapping unexpectedly resolved %q", input)
			}
			if got := resolveMappedDestinationPath(input); got == containerSource {
				t.Fatalf("retired destination mapping unexpectedly resolved %q", input)
			}
		})
	}
	if isRelativeVolumeMarkerPath("downloads") || isRelativeVolumeMarkerPath("downloads/国产剧") {
		t.Fatal("downloads must no longer be a reserved container path marker")
	}
}

func TestMediaPathsKeepExplicitDownloadDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "downloads", "国产剧")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	got, info, err := resolveAccessibleMappedPath(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || got != directory {
		t.Fatalf("explicit directory = %q, want %q", got, directory)
	}
	if got, err := resolveAccessibleLibraryPath(directory); err != nil || got != directory {
		t.Fatalf("explicit library = %q, %v, want %q", got, err, directory)
	}
	if got := resolveMappedDestinationPath(directory); got != directory {
		t.Fatalf("explicit destination = %q, want %q", got, directory)
	}
}
