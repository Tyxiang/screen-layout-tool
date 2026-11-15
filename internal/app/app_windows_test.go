//go:build windows

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"screen-layout-tool/internal/layout"
	"screen-layout-tool/internal/settings"
)

func TestReleaseBinarySmoke(t *testing.T) {
	executable := os.Getenv("SLT_TEST_RELEASE_EXECUTABLE")
	if executable == "" {
		t.Skip("发布构建完成后运行")
	}
	command := exec.Command(executable)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("发布程序提前退出：%v\n%s", err, output.String())
	case <-time.After(500 * time.Millisecond):
		if err := command.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		<-done
	}
}

func TestNativeApplicationStartupAndShutdown(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "layouts"), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"positions":[{"x":0,"y":0,"width":100,"height":100}]}`)
	if err := os.WriteFile(filepath.Join(directory, "layouts", "Basic.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	startupCalled := false
	err := run(ctx, "test", filepath.Join(directory, "screen-layout-tool.exe"), func(path string, enabled bool) error {
		startupCalled = true
		if enabled {
			t.Error("默认不应启用自动启动")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !startupCalled {
		t.Fatal("启动流程没有同步自动启动设置")
	}
}

type fakeDesktop struct {
	warning string
	quit    bool
}

func (*fakeDesktop) PostTimer(uintptr)                         {}
func (d *fakeDesktop) Notify(title, text string, warning bool) { d.warning = text }
func (d *fakeDesktop) Quit()                                   { d.quit = true }

func testApplication(t *testing.T) (*application, *fakeDesktop) {
	t.Helper()
	store, err := settings.Open(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	desktop := &fakeDesktop{}
	application := &application{host: desktop, store: store, names: []string{"A", "Basic", "Z"}, values: settings.Default(), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	t.Cleanup(application.cancel)
	return application, desktop
}

func TestMenuSelectsAndSavesLayout(t *testing.T) {
	a, _ := testApplication(t)
	a.Command(commandLayout + 2)
	loaded, err := a.store.Load()
	if err != nil || loaded.Layout != "Z" || a.values.Layout != "Z" {
		t.Fatal("菜单选择布局未保存", loaded, err)
	}
}

func TestWindowBatchCancelledByFocusAndLayout(t *testing.T) {
	a, _ := testApplication(t)
	a.windowPending = pending{step: 3, window: 123, layout: "Basic"}
	a.arm(&a.windowPending)
	token := a.windowPending.token
	a.Focus(123)
	if a.windowPending.token != token {
		t.Fatal("相同窗口不应取消操作")
	}
	a.Focus(456)
	if a.windowPending.token != 0 {
		t.Fatal("窗口改变后未取消操作")
	}
	a.windowPending = pending{step: 2, window: 456, layout: "Basic"}
	a.arm(&a.windowPending)
	a.selectLayout("A")
	if a.windowPending.token != 0 {
		t.Fatal("布局改变后未取消窗口操作")
	}
	// 已取消批次的计时回调到达时不得执行窗口操作。
	a.Timer(token)
}

func TestMenuAndExit(t *testing.T) {
	a, d := testApplication(t)
	a.names = []string{"A&B", "Basic"}
	menu := a.Menu()
	if menu[0].Children[0].Text != "A&&B" || !menu[0].Children[1].Checked {
		t.Fatal(menu)
	}
	a.names = nil
	if children := a.Menu()[0].Children; len(children) != 1 || !children[0].Disabled {
		t.Fatal(children)
	}
	a.windowPending = pending{step: 1, window: 123, layout: "Basic"}
	a.arm(&a.windowPending)
	a.Command(commandExit)
	if !d.quit || a.windowPending.token != 0 {
		t.Fatal("退出未取消待执行操作")
	}
}

func TestLayoutErrorsHaveSpecificEnglishMessages(t *testing.T) {
	a, d := testApplication(t)
	for _, failure := range []struct {
		err  error
		text string
	}{
		{layout.ErrRead, "could not be read"},
		{layout.ErrFormat, "invalid JSON"},
		{layout.ErrPosition, "invalid positions"},
		{layout.ErrDuplicate, "duplicate positions"},
		{errors.New("native error"), "could not be adjusted"},
	} {
		a.windowFailure(failure.err)
		if !strings.Contains(d.warning, failure.text) {
			t.Fatal(d.warning)
		}
	}
}
