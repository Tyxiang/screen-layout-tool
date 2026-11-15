//go:build windows

package main

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"strings"

	"screen-layout-tool/internal/app"
	"screen-layout-tool/internal/win32"
)

//go:embed VERSION
var sourceVersion string

var version string

func main() {
	if version == "" {
		version = strings.TrimSpace(sourceVersion) + "-dev"
	}
	if err := app.Run(context.Background(), version); err != nil {
		slog.Error("程序启动失败", "错误", err)
		win32.Message("Screen Layout Tool", "Failed to start Screen Layout Tool. See the log file for details.", true)
		os.Exit(1)
	}
}
