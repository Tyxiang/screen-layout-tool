//go:build windows

// Package app 协调托盘菜单、快捷键事件、设置和窗口操作。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"screen-layout-tool/internal/layout"
	"screen-layout-tool/internal/settings"
	"screen-layout-tool/internal/win32"
)

const (
	commandOpen = iota + 1
	commandAutoStart
	commandAbout
	commandHelp
	commandReload
	commandExit
	commandLayout = 100
)

const debounce = 200 * time.Millisecond

type pending struct {
	step   int
	window uintptr
	layout string
	token  uintptr
	timer  *time.Timer
}

func (p *pending) cancel() {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer, p.step, p.window, p.layout, p.token = nil, 0, 0, "", 0
}

type desktop interface {
	PostTimer(uintptr)
	Notify(string, string, bool)
	Quit()
}

type application struct {
	host          desktop
	store         *settings.Store
	values        settings.Values
	names         []string
	directory     string
	executable    string
	version       string
	logger        *slog.Logger
	sequence      uintptr
	windowPending pending
}

// Run 使用调用方的上下文控制退出，不创建第二个程序入口。
func Run(ctx context.Context, version string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return run(ctx, version, executable, win32.Startup, win32.SingleInstance)
}

func run(ctx context.Context, version, executable string, startup func(string, bool) error, instance func() (func(), bool, error)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	release, first, err := instance()
	if err != nil {
		return err
	}
	if !first {
		return nil
	}
	defer release()
	win32.EnableDPI()
	programDir := filepath.Dir(executable)
	store, locationErr := settings.Open(programDir, os.Getenv("APPDATA"))
	values, settingsErr := store.Load()
	logFile, logErr := openLog(programDir, os.Getenv("APPDATA"))
	var output io.Writer = os.Stderr
	if logErr == nil {
		output = logFile
		defer logFile.Close()
	}
	a := &application{store: store, values: values, executable: executable, version: version, directory: filepath.Join(programDir, "layouts"), logger: slog.New(slog.NewTextHandler(output, nil))}
	host, err := win32.NewHost(a, iconData)
	if err != nil {
		a.logger.Error("创建托盘或快捷键失败", "错误", err)
		return err
	}
	a.host = host
	defer host.Close()
	defer a.cancel()
	a.logger.Info("程序启动", "版本", version)
	for _, failure := range []error{locationErr, settingsErr, logErr} {
		if failure != nil {
			a.logger.Error("读取设置或初始化日志失败", "错误", failure)
		}
	}
	if locationErr != nil {
		host.Notify("Warning", "Settings cannot be saved. The application will use in-memory settings.", true)
	}
	if settingsErr != nil {
		host.Notify("Warning", "Settings could not be read. Default settings are being used.", true)
	}
	if logErr != nil {
		host.Notify("Warning", "The log file could not be opened.", true)
	}
	a.loadNames()
	if err := startup(executable, values.AutoStart); err != nil {
		a.startupFailure(err)
	}
	err = host.Loop(ctx)
	a.logger.Info("程序退出")
	return err
}

func (a *application) cancel() {
	a.windowPending.cancel()
}

func (a *application) arm(p *pending) {
	if p.timer != nil {
		p.timer.Stop()
	}
	a.sequence++
	if a.sequence == 0 {
		a.sequence++
	}
	p.token = a.sequence
	token := p.token
	p.timer = time.AfterFunc(debounce, func() { a.host.PostTimer(token) })
}

func (a *application) Input(action win32.Action, window uintptr) {
	step := -1
	if action == win32.WindowNext {
		step = 1
	}
	if action != win32.WindowPrevious && action != win32.WindowNext {
		return
	}
	if !win32.Manageable(window) {
		return
	}
	p := &a.windowPending
	if p.window != window || p.layout != a.values.Layout {
		p.cancel()
	}
	p.window, p.layout = window, a.values.Layout
	p.step += step
	a.arm(p)
}

func (a *application) Focus(window uintptr) {
	if a.windowPending.window != 0 && window != a.windowPending.window {
		a.windowPending.cancel()
	}
}

func (a *application) Timer(token uintptr) {
	if token == 0 {
		return
	}
	if token == a.windowPending.token {
		step, window, name := a.windowPending.step, a.windowPending.window, a.windowPending.layout
		a.windowPending.cancel()
		if step == 0 || name != a.values.Layout || window != win32.Foreground() || !win32.Manageable(window) {
			return
		}
		if err := a.move(window, step); err != nil {
			a.windowFailure(err)
		}
	}
}

func (a *application) move(window uintptr, step int) error {
	if a.values.Layout == "" {
		return layout.ErrRead
	}
	parsed, err := layout.Load(filepath.Join(a.directory, a.values.Layout+".json"))
	if err != nil {
		return err
	}
	if err := win32.Restore(window); err != nil {
		return err
	}
	current, err := win32.WindowRect(window)
	if err != nil {
		return err
	}
	monitors, err := win32.Monitors()
	if err != nil {
		return err
	}
	monitor, err := layout.MonitorFor(current, monitors)
	if err != nil {
		return err
	}
	if monitor.Work.Width <= 0 || monitor.Work.Height <= 0 {
		return errors.New("monitor has no work area")
	}
	target, move := layout.Target(parsed.Positions, layout.Percent(current, monitor.Work), step)
	if !move {
		return nil
	}
	return win32.Move(window, layout.Pixels(target, monitor.Work))
}

func (a *application) loadNames() {
	names, err := layout.Names(a.directory)
	if err != nil {
		a.Failure(err)
		return
	}
	a.names = names
	a.selectLayout(layout.Current(names, a.values.Layout))
	if len(names) == 0 {
		a.host.Notify("Warning", "No layouts are available. Add JSON files to the layout folder, then click Reload.", true)
	}
}

func (a *application) selectLayout(name string) {
	if name == a.values.Layout {
		return
	}
	a.windowPending.cancel()
	a.values.Layout = name
	a.save()
}

func (a *application) save() {
	if err := a.store.Save(a.values); err != nil {
		a.logger.Error("保存设置失败", "错误", err)
		a.host.Notify("Warning", "Settings could not be saved. Changes will apply to this session only.", true)
	}
}

func (a *application) Menu() []win32.MenuItem {
	children := make([]win32.MenuItem, 0, len(a.names))
	for i, name := range a.names {
		children = append(children, win32.MenuItem{ID: uint32(commandLayout + i), Text: escapeMenu(name), Checked: name == a.values.Layout})
	}
	if len(children) == 0 {
		children = append(children, win32.MenuItem{Text: "No Layouts Available", Disabled: true})
	}
	return []win32.MenuItem{
		{Text: "Layout", Submenu: true, Children: children},
		{ID: commandOpen, Text: "Open Layout Folder"}, {},
		{ID: commandAutoStart, Text: "Auto Start", Checked: a.values.AutoStart}, {},
		{ID: commandAbout, Text: "About"},
		{ID: commandHelp, Text: "Help"}, {},
		{ID: commandReload, Text: "Reload"},
		{ID: commandExit, Text: "Exit"},
	}
}

func (a *application) Command(id uint32) {
	if id >= commandLayout && int(id-commandLayout) < len(a.names) {
		a.selectLayout(a.names[id-commandLayout])
		return
	}
	switch id {
	case commandOpen:
		if err := os.MkdirAll(a.directory, 0755); err != nil {
			a.Failure(err)
			return
		}
		if err := win32.Open(a.directory); err != nil {
			a.Failure(err)
		}
	case commandAutoStart:
		enabled := !a.values.AutoStart
		if err := win32.Startup(a.executable, enabled); err != nil {
			a.startupFailure(err)
			return
		}
		a.values.AutoStart = enabled
		a.save()
	case commandAbout:
		win32.Message("About", fmt.Sprintf("Screen Layout Tool\nVersion %s", a.version), false)
	case commandHelp:
		if err := win32.Open("https://github.com/Tyxiang/screen-layout-tool#readme"); err != nil {
			a.Failure(err)
		}
	case commandReload:
		a.cancel()
		values, err := a.store.Load()
		if err != nil {
			a.logger.Error("重新读取设置失败", "错误", err)
			a.host.Notify("Warning", "Settings could not be reloaded.", true)
			return
		}
		a.values = values
		a.loadNames()
		if err := win32.Startup(a.executable, a.values.AutoStart); err != nil {
			a.startupFailure(err)
		}
	case commandExit:
		a.cancel()
		a.host.Quit()
	}
}

func (a *application) Failure(err error) {
	a.logger.Error("操作失败", "错误", err)
	a.host.Notify("Warning", "The operation failed. See the log file for details.", true)
}

func (a *application) startupFailure(err error) {
	a.logger.Error("更新自动启动快捷方式失败", "错误", err)
	a.host.Notify("Warning", "The startup shortcut could not be updated.", true)
}

func (a *application) windowFailure(err error) {
	a.logger.Error("切换窗口位置失败", "错误", err)
	text := "The window could not be adjusted."
	switch {
	case errors.Is(err, layout.ErrRead):
		text = "The layout file could not be read."
	case errors.Is(err, layout.ErrFormat):
		text = "The layout file contains invalid JSON or field types."
	case errors.Is(err, layout.ErrDuplicate):
		text = "The layout contains duplicate positions."
	case errors.Is(err, layout.ErrPosition):
		text = "The layout contains invalid positions."
	}
	a.host.Notify("Warning", text, true)
}
