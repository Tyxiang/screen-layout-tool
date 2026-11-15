package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableSettingsRoundTrip(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory, t.TempDir())
	if err != nil || s.Path != filepath.Join(directory, "config.ini") {
		t.Fatalf("%v %v", s, err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("首次读取前不应创建设置文件")
	}
	v, err := s.Load()
	if err != nil || v != Default() {
		t.Fatalf("%+v %v", v, err)
	}
	for _, value := range []Values{{Layout: "中文 & Layout", AutoStart: true}, {Layout: "Basic", AutoStart: false}} {
		if err := s.Save(value); err != nil {
			t.Fatal(err)
		}
		loaded, err := s.Load()
		if err != nil || loaded != value {
			t.Fatalf("%+v %v", loaded, err)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(directory, ".config-*"))
	if len(leftovers) != 0 {
		t.Fatal(leftovers)
	}
}

func TestFallbackAndInMemorySettings(t *testing.T) {
	directory := t.TempDir()
	blocked := filepath.Join(directory, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	appData := t.TempDir()
	s, err := Open(blocked, appData)
	if err != nil || s.Path != filepath.Join(appData, "screen-layout-tool", "config.ini") {
		t.Fatalf("%+v %v", s, err)
	}
	s, err = Open(blocked, blocked)
	if err == nil || s.Path != "" {
		t.Fatal("两处目录失败时应使用内存设置")
	}
	if v, err := s.Load(); err != nil || v != Default() {
		t.Fatal(v, err)
	}
	if err := s.Save(Default()); err == nil {
		t.Fatal("内存设置不可报告保存成功")
	}
}

func TestSaveFailureDoesNotSwitchDirectory(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := s.Path
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Default()); err == nil {
		t.Fatal("目标路径是目录时保存应失败")
	}
	if s.Path != path {
		t.Fatal("保存失败后切换了路径")
	}
}

func TestInvalidSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	if err := os.WriteFile(path, []byte("[Setting]\nAutoStart=wrong\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Store{Path: path}).Load(); err == nil {
		t.Fatal("错误布尔值应报告错误")
	}
}
