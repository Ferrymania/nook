package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func (p *openListProvider) Mkdir(ctx context.Context, parentDir, name string) (*FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	cleanName, err := cleanCloudEntryName(name)
	if err != nil {
		return nil, err
	}
	parent := normalizeCloudDAVPath(parentDir)
	target := joinOpenListAPIPath(parent, cleanName)
	if err := p.openListAPIMkdir(ctx, target); err != nil {
		return nil, err
	}
	return &FileEntry{ID: target, Name: cleanName, IsDir: true}, nil
}

func (p *openListProvider) Rename(ctx context.Context, ref, name string) (*FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	cleanName, err := cleanCloudEntryName(name)
	if err != nil {
		return nil, err
	}
	source := normalizeCloudDAVPath(ref)
	if source == "/" {
		return nil, fmt.Errorf("%s: cannot rename root directory", p.name)
	}
	target := joinOpenListAPIPath(path.Dir(source), cleanName)
	if err := p.openListAPIRename(ctx, source, cleanName); err != nil {
		return nil, err
	}
	return &FileEntry{ID: target, Name: cleanName, IsDir: true}, nil
}

func (p *openListProvider) Move(ctx context.Context, ref, targetDir, name string) (*FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	source := normalizeCloudDAVPath(ref)
	if source == "/" {
		return nil, fmt.Errorf("%s: cannot move root directory", p.name)
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = path.Base(source)
	}
	var err error
	cleanName, err = cleanCloudEntryName(cleanName)
	if err != nil {
		return nil, err
	}
	targetDir = normalizeCloudDAVPath(targetDir)
	target := joinOpenListAPIPath(targetDir, cleanName)
	if sameCloudDAVPath(source, target) {
		return &FileEntry{ID: target, Name: cleanName}, nil
	}
	if err := p.openListAPIMove(ctx, source, targetDir, cleanName); err != nil {
		return nil, err
	}
	return &FileEntry{ID: target, Name: cleanName}, nil
}

func cleanCloudEntryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("entry name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("entry name cannot contain path separators")
	}
	return name, nil
}

func (p *openListProvider) openListAPIMkdir(ctx context.Context, target string) error {
	return p.openListAPIPost(ctx, "/api/fs/mkdir", map[string]string{"path": normalizeCloudDAVPath(target)}, "mkdir")
}

func (p *openListProvider) openListAPIRename(ctx context.Context, source, name string) error {
	return p.openListAPIPost(ctx, "/api/fs/rename", map[string]string{
		"path": normalizeCloudDAVPath(source),
		"name": name,
	}, "rename")
}

func (p *openListProvider) openListAPIMove(ctx context.Context, source, targetDir, targetName string) error {
	targetDir = normalizeCloudDAVPath(targetDir)
	sourceName := path.Base(normalizeCloudDAVPath(source))
	if sameCloudDAVPath(path.Dir(source), targetDir) {
		if sourceName == targetName {
			return nil
		}
		return p.openListAPIRename(ctx, source, targetName)
	}
	if err := p.openListAPIPost(ctx, "/api/fs/move", map[string]any{
		"src_dir": normalizeCloudDAVPath(path.Dir(source)),
		"dst_dir": targetDir,
		"names":   []string{sourceName},
	}, "move"); err != nil {
		return err
	}
	if sourceName != targetName {
		moved := joinOpenListAPIPath(targetDir, sourceName)
		return p.openListAPIRename(ctx, moved, targetName)
	}
	return nil
}

func (p *openListProvider) openListAPIPost(ctx context.Context, apiPath string, payload any, action string) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL(apiPath), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.ua)
	req.Header.Set("Authorization", p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return decorateDAVTransportError(p.name, p.openListAPIURL(apiPath), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return p.openListAPIStatusError(action, "", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: api %s returned http %d", p.name, action, resp.StatusCode)
	}
	var decoded struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return fmt.Errorf("%s: decode api %s: %w", p.name, action, err)
	}
	if decoded.Code != 0 && decoded.Code != 200 {
		msg := strings.TrimSpace(decoded.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", decoded.Code)
		}
		return fmt.Errorf("%s: api %s failed: %s", p.name, action, msg)
	}
	return nil
}

// PutFile 把本地文件内容上传（覆盖）到远端 remotePath。
// 使用 OpenList /api/fs/form 表单接口和账号 Token。
func (p *openListProvider) PutFile(ctx context.Context, remotePath string, r io.Reader) error {
	if err := p.validate(); err != nil {
		return err
	}
	target := normalizeCloudDAVPath(remotePath)
	if target == "/" {
		return fmt.Errorf("%s: cannot upload to root directory", p.name)
	}
	return p.openListAPIPutFile(ctx, target, r)
}

// openListAPIPutFile 通过 OpenList /api/fs/form 上传（QMediaSync 同款契约：
// PUT + multipart + File-Path 头）。使用 io.Pipe + multipart.Writer 边写边发，
// 避免把整个文件读进内存。
func (p *openListProvider) openListAPIPutFile(ctx context.Context, remotePath string, r io.Reader) error {
	encodedPath := openListPathEscape(remotePath)

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		var writeErr error
		defer func() {
			// 读源失败必须传给 pipe 写端，让 HTTP 请求以失败收场而不是静默截断
			if writeErr != nil {
				_ = pw.CloseWithError(writeErr)
				return
			}
			_ = pw.Close()
		}()
		formFile, err := writer.CreateFormFile("file", path.Base(remotePath))
		if err != nil {
			writeErr = err
			return
		}
		if _, err := io.Copy(formFile, r); err != nil {
			writeErr = err
			return
		}
		writeErr = writer.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.openListAPIURL("/api/fs/form"), pr)
	if err != nil {
		// 关闭读端以释放仍在等待写入的后台 goroutine（其 Write 会立即失败返回）
		_ = pr.Close()
		return err
	}
	req.Header.Set("Authorization", p.token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("File-Path", encodedPath)
	req.Header.Set("As-Task", "true")
	req.Header.Set("Overwrite", "true")
	resp, err := p.client.Do(req)
	if err != nil {
		// 传输层失败（含提前断开）时 net/http 会关闭请求 body，解除后台 goroutine 阻塞
		return decorateDAVTransportError(p.name, p.openListAPIURL("/api/fs/form"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return p.openListAPIStatusError("upload", remotePath, resp.StatusCode)
	}
	// OpenList 返回 code=200 即任务受理成功（小文件同步完成，大文件异步排队）。
	return nil
}

// openListPathEscape 保留斜杠地 URL 编码远端路径（OpenList File-Path 需要）。
func openListPathEscape(p string) string {
	parts := strings.Split(strings.TrimPrefix(normalizeCloudDAVPath(p), "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
