package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// openLog 独立选择日志位置，不受设置文件位置影响。
func openLog(programDir, appData string) (*os.File, error) {
	paths := []string{programDir}
	if appData != "" {
		paths = append(paths, filepath.Join(appData, "screen-layout-tool"))
	}
	var failures []error
	for i, dir := range paths {
		if i > 0 {
			if err := os.MkdirAll(dir, 0700); err != nil {
				failures = append(failures, err)
				continue
			}
		}
		file, err := os.OpenFile(filepath.Join(dir, "log.txt"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
		if err == nil {
			return file, nil
		}
		failures = append(failures, err)
	}
	return nil, fmt.Errorf("日志文件无法打开: %w", errors.Join(failures...))
}
