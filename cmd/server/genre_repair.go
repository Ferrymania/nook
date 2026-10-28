package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/database"
	"github.com/Ferrymania/nook/internal/service"
)

// This maintenance command opens only the existing database: no schema
// migrations, HTTP listener, scraper, probe or background jobs are started.
func runGenreRepair(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("repair-genres", flag.ContinueOnError)
	flags.SetOutput(out)
	libraryID := flags.String("library", "", "要检查的媒体库 ID")
	report := flags.String("report", "", "只读预览报告路径（含修改前后的类型）")
	apply := flags.String("apply", "", "应用已审阅的报告文件")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (*apply != "" && (*libraryID != "" || *report != "")) || (*apply == "" && (*libraryID == "" || *report == "")) {
		return errors.New("预览: repair-genres --library <ID> --report <文件>；应用: repair-genres --apply <报告文件>")
	}
	var plan service.GenreRepairPlan
	if *apply != "" {
		body, err := os.ReadFile(*apply) // #nosec G304 -- explicit administrator-supplied local report.
		if err != nil {
			return err
		}
		if err := json.Unmarshal(body, &plan); err != nil {
			return err
		}
	}
	cfg, err := config.LoadReadOnly()
	if err != nil {
		return err
	}
	db, err := database.Open(cfg, zap.NewNop())
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *apply != "" {
		if err := service.ApplyGenreRepairPlan(ctx, db, &plan); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "已更新 %d 条媒体的 genres；原值保留在报告中。请重启 Nook 清除内存缓存。\n", len(plan.Changes))
		return err
	}
	preview, err := service.BuildGenreRepairPlan(ctx, db, *libraryID)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- explicit local report; never overwrite an earlier backup.
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(preview)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	_, err = fmt.Fprintf(out, "只读预览：%s，待修复 %d 条，需人工核对 %d 条。报告: %s；数据库未修改。\n", preview.LibraryName, len(preview.Changes), len(preview.Skipped), *report)
	return err
}
