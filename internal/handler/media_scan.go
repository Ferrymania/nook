package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/service"
)

func scanLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		lib, err := svc.Repo.Library.FindByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}
		if _, ok := service.ParseCloudLibraryMount(lib.Path); ok {
			// 云盘扫描已随网盘后端移除；此处保留空分支以兼容既有客户端。
			c.JSON(http.StatusBadRequest, gin.H{"error": "cloud 网盘后端已移除，无法扫描云盘媒体库"})
			return
		}
		finishScan, ok := svc.Scan.TryBeginLocalScan(id)
		if !ok {
			c.JSON(http.StatusAccepted, gin.H{
				"library_id":       id,
				"queued":           true,
				"already_running":  true,
				"task_id":          currentScanTaskID(svc, id),
				"message":          "该媒体库正在后台扫描，请在任务面板查看进度",
				"estimate_message": "页面关闭不会中断扫描",
			})
			return
		}
		task := launchLibraryScan(svc, id, "", lib.Name, lib.Path, finishScan)
		c.JSON(http.StatusAccepted, gin.H{
			"library_id":       id,
			"queued":           true,
			"task_id":          task.ID(),
			"message":          "本地媒体库扫描已在后台运行，页面关闭不会中断",
			"estimate_message": "可在右上角任务面板查看扫描进度",
		})
	}
}

func scanLibraryRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		rootID := c.Param("root_id")
		finishScan, ok := svc.Scan.TryBeginLocalScan(id + ":" + rootID)
		if !ok {
			c.JSON(http.StatusAccepted, gin.H{
				"library_id":       id,
				"queued":           true,
				"already_running":  true,
				"task_id":          currentScanTaskID(svc, id),
				"message":          "该路径正在后台扫描，请在任务面板查看进度",
				"estimate_message": "页面关闭不会中断扫描",
			})
			return
		}
		task := launchLibraryScan(svc, id, rootID, id, rootID, finishScan)
		c.JSON(http.StatusAccepted, gin.H{
			"library_id":       id,
			"queued":           true,
			"task_id":          task.ID(),
			"message":          "媒体库路径扫描已在后台运行，页面关闭不会中断",
			"estimate_message": "可在右上角任务面板查看扫描进度",
		})
	}
}

// queueLibraryRootScan starts the same background scan used by the manual
// scan endpoints. Library creation/root addition used to refresh only the
// watcher, which left a newly-created library empty until the user clicked
// "扫描" manually. Keeping this helper in the scan handler makes all mutation
// paths use one scan lifecycle and preserves the local-scan de-duplication.
func queueLibraryRootScan(svc *service.Container, libraryID, rootID string) {
	if svc == nil || svc.Scan == nil || strings.TrimSpace(libraryID) == "" {
		return
	}
	key := libraryID
	if strings.TrimSpace(rootID) != "" {
		key += ":" + rootID
	}
	finish, ok := svc.Scan.TryBeginLocalScan(key)
	if !ok {
		return
	}
	launchLibraryScan(svc, libraryID, rootID, libraryID, rootID, finish)
}

func currentScanTaskID(svc *service.Container, libraryID string) string {
	if task := svc.Tasks.LibraryScan(libraryID); task != nil {
		return task.ID
	}
	return ""
}

func launchLibraryScan(svc *service.Container, libraryID, rootID, name, path string, finish func()) *service.TaskHandle {
	var task *service.TaskHandle
	if svc.Tasks != nil {
		task = svc.Tasks.Start(service.TaskKindScan, "扫描媒体库："+name, service.TaskUpdate{
			LibraryID: libraryID, Stage: "scan", SourcePath: path, Message: "正在扫描并入库",
		})
	}
	go func() {
		defer finish()
		var result *service.ScanResult
		err := helper.Recover(svc.Log, "scan.library", func() error {
			var err error
			ctx := service.WithScanTask(context.Background(), task)
			if rootID == "" {
				result, err = svc.Scan.ScanLibrary(ctx, libraryID)
			} else {
				result, err = svc.Scan.ScanLibraryRoot(ctx, libraryID, rootID)
			}
			return err
		})
		if err != nil {
			finishHTTPTask(task, err, "scan", "媒体库扫描失败", result.TaskMetrics(), scanTaskDetails(result, 20))
		} else {
			finishHTTPTask(task, nil, "completed", "媒体库扫描完成", result.TaskMetrics(), scanTaskDetails(result, 20))
		}
	}()
	return task
}

func libraryScanStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		lib, err := svc.Repo.Library.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}
		task := svc.Tasks.LibraryScan(lib.ID)
		c.JSON(http.StatusOK, gin.H{"task": task})
	}
}

func scanTaskDetails(res *service.ScanResult, limit int) []string {
	if res == nil || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, line := range res.Errors {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "错误: "+line)
		if len(out) >= limit {
			return out
		}
	}
	return out
}
