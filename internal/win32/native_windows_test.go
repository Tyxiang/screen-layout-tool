//go:build windows

package win32

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"screen-layout-tool/internal/layout"
)

type testHandler struct{}

func (*testHandler) Input(Action, uintptr) {}
func (*testHandler) Timer(uintptr)         {}
func (*testHandler) Focus(uintptr)         {}
func (*testHandler) Command(uint32)        {}
func (*testHandler) Menu() []MenuItem      { return nil }
func (*testHandler) Failure(error)         {}

func TestNativeHostAndWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	EnableDPI()
	data, err := os.ReadFile(filepath.Join("..", "..", "raw", "icon", "16-256.ico"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(&testHandler{}, data)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	monitors, err := Monitors()
	if err != nil || len(monitors) == 0 {
		t.Fatalf("显示器枚举失败：%v", err)
	}
	work := monitors[0].Work
	target := layout.Pixels(layout.Rect{X: 20, Y: 20, Width: 40, Height: 40}, work)
	if err := Move(host.window.Load(), target); err != nil {
		t.Fatal(err)
	}
	actual, err := WindowRect(host.window.Load())
	if err != nil || actual != target {
		t.Fatalf("窗口矩形不一致：%+v %v", actual, err)
	}
	menu, err := createMenu([]MenuItem{{Text: "Layout", Submenu: true, Children: []MenuItem{{ID: 100, Text: "Basic", Checked: true}}}, {}, {ID: 1, Text: "Exit"}})
	if err != nil {
		t.Fatal(err)
	}
	destroyMenu.Call(menu)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := host.Loop(ctx); err != nil {
		t.Fatal(err)
	}
	if host.window.Load() != 0 {
		t.Fatal("退出时未销毁窗口")
	}
}

func TestShortcutRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lnk")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := Shortcut(path, executable, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 76 || binary.LittleEndian.Uint32(data[:4]) != 76 {
		t.Fatalf("无效快捷方式：%v", err)
	}
	if err := Shortcut(path, executable, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("未删除快捷方式")
	}
	if err := Shortcut(path, executable, false); err != nil {
		t.Fatal(err)
	}
}
