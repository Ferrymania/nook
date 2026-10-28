package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenreRepairCommandRequiresPreviewOrExplicitReport(t *testing.T) {
	for _, args := range [][]string{nil, {"--library", "id"}, {"--apply", "file", "--library", "id"}, {"--library", "id", "--report", "file", "extra"}} {
		if err := runGenreRepair(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "预览:") {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
	if err := runGenreRepair([]string{"--help"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
