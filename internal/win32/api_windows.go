//go:build windows

// Package win32 封装程序使用的 Windows 原生接口。
package win32

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	registerClass    = user32.NewProc("RegisterClassExW")
	unregisterClass  = user32.NewProc("UnregisterClassW")
	createWindow     = user32.NewProc("CreateWindowExW")
	destroyWindow    = user32.NewProc("DestroyWindow")
	defWindowProc    = user32.NewProc("DefWindowProcW")
	getMessage       = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessage  = user32.NewProc("DispatchMessageW")
	postMessage      = user32.NewProc("PostMessageW")
	postQuitMessage  = user32.NewProc("PostQuitMessage")
	loadImage        = user32.NewProc("LoadImageW")
	loadIcon         = user32.NewProc("LoadIconW")
	destroyIcon      = user32.NewProc("DestroyIcon")
	createPopupMenu  = user32.NewProc("CreatePopupMenu")
	appendMenu       = user32.NewProc("AppendMenuW")
	destroyMenu      = user32.NewProc("DestroyMenu")
	trackPopupMenu   = user32.NewProc("TrackPopupMenu")
	getCursorPos     = user32.NewProc("GetCursorPos")
	setForeground    = user32.NewProc("SetForegroundWindow")
	getForeground    = user32.NewProc("GetForegroundWindow")
	getWindowRect    = user32.NewProc("GetWindowRect")
	setWindowPos     = user32.NewProc("SetWindowPos")
	showWindow       = user32.NewProc("ShowWindow")
	isZoomed         = user32.NewProc("IsZoomed")
	isWindow         = user32.NewProc("IsWindow")
	getWindowThread  = user32.NewProc("GetWindowThreadProcessId")
	getShellWindow   = user32.NewProc("GetShellWindow")
	getDesktopWindow = user32.NewProc("GetDesktopWindow")
	getClassName     = user32.NewProc("GetClassNameW")
	enumMonitors     = user32.NewProc("EnumDisplayMonitors")
	getMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	setDPIContext    = user32.NewProc("SetProcessDpiAwarenessContext")
	setDPIAware      = user32.NewProc("SetProcessDPIAware")
	setHook          = user32.NewProc("SetWindowsHookExW")
	unhook           = user32.NewProc("UnhookWindowsHookEx")
	nextHook         = user32.NewProc("CallNextHookEx")
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	keybdEvent       = user32.NewProc("keybd_event")
	setWinEventHook  = user32.NewProc("SetWinEventHook")
	unhookWinEvent   = user32.NewProc("UnhookWinEvent")
	registerMessage  = user32.NewProc("RegisterWindowMessageW")
	shellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	shellExecute     = shell32.NewProc("ShellExecuteW")
	messageBox       = user32.NewProc("MessageBoxW")
	getModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

func wide(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func failed(api string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s failed", api)
	}
	return fmt.Errorf("%s: %w", api, err)
}

func Message(title, body string, warning bool) {
	flags := uintptr(0x40)
	if warning {
		flags = 0x10
	}
	messageBox.Call(0, uintptr(unsafe.Pointer(wide(body))), uintptr(unsafe.Pointer(wide(title))), flags)
}

func EnableDPI() {
	if setDPIContext.Find() == nil {
		// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2。
		if ret, _, _ := setDPIContext.Call(^uintptr(3)); ret != 0 {
			return
		}
	}
	setDPIAware.Call()
}

func Open(path string) error {
	result, _, err := shellExecute.Call(0, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(path))), 0, 0, 1)
	if result <= 32 {
		return failed("ShellExecuteW", err)
	}
	return nil
}
