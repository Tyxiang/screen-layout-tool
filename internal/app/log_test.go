package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogLocationAndAppend(t *testing.T) {
	program, appData := t.TempDir(), t.TempDir()
	for _, text := range []string{"第一条\n", "第二条\n"} {
		file, err := openLog(program, appData)
		if err != nil {
			t.Fatal(err)
		}
		if file.Name() != filepath.Join(program, "log.txt") {
			t.Fatal(file.Name())
		}
		if _, err := file.WriteString(text); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(program, "log.txt"))
	if err != nil || string(data) != "第一条\n第二条\n" {
		t.Fatalf("日志内容：%q，错误：%v", data, err)
	}
}

func TestLogFallback(t *testing.T) {
	program, appData := t.TempDir(), t.TempDir()
	// 日志文件本身无法打开，即使程序目录可用，也必须尝试备用目录。
	if err := os.Mkdir(filepath.Join(program, "log.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := openLog(program, appData)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Name() != filepath.Join(appData, "screen-layout-tool", "log.txt") {
		t.Fatal(file.Name())
	}
}

func TestLogUnavailable(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if file, err := openLog(blocked, blocked); err == nil {
		file.Close()
		t.Fatal("两个位置均不可用时必须返回错误")
	}
}
