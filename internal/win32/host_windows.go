//go:build windows

package win32

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmCommand = 0x0111
	wmTray    = 0x8001
	wmAction  = 0x8002
	wmTimer   = 0x8003
	wmFocus   = 0x8004
	wmWake    = 0x8005
)

type Action uint32

const (
	WindowPrevious Action = iota + 1
	WindowNext
)

type Handler interface {
	Input(Action, uintptr)
	Timer(uintptr)
	Focus(uintptr)
	Command(uint32)
	Menu() []MenuItem
	Failure(error)
}

type MenuItem struct {
	ID       uint32
	Text     string
	Checked  bool
	Disabled bool
	Children []MenuItem
	Submenu  bool
}

type Host struct {
	window         atomic.Uintptr
	handler        Handler
	keyboard       uintptr
	mouse          uintptr
	foreground     uintptr
	icon           uintptr
	class          *uint16
	module         uintptr
	taskbarCreated uint32
	callbacks      []uintptr
	suppressed     map[uint32]bool
	maskedWin      uint8
	closed         bool
}

type point struct{ X, Y int32 }
type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}

type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}

func NewHost(handler Handler, iconData []byte) (*Host, error) {
	h := &Host{handler: handler, suppressed: make(map[uint32]bool)}
	h.module, _, _ = getModuleHandle.Call(0)
	h.class = wide(fmt.Sprintf("ScreenLayoutTool.%d", windows.GetCurrentProcessId()))
	windowCallback := windows.NewCallback(h.procedure)
	h.callbacks = append(h.callbacks, windowCallback)
	class := windowClass{Proc: windowCallback, Instance: h.module, ClassName: h.class}
	class.Size = uint32(unsafe.Sizeof(class))
	if result, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return nil, failed("RegisterClassExW", err)
	}
	window, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(h.class)), uintptr(unsafe.Pointer(wide("Screen Layout Tool"))), 0, 0, 0, 0, 0, 0, 0, h.module, 0)
	if window == 0 {
		unregisterClass.Call(uintptr(unsafe.Pointer(h.class)), h.module)
		return nil, failed("CreateWindowExW", err)
	}
	h.window.Store(window)
	defer func() {
		if err != nil {
			h.Close()
		}
	}()
	h.icon, err = iconFromICO(iconData)
	if err != nil {
		return nil, err
	}
	h.taskbarCreated = uint32(register("TaskbarCreated"))
	if err = h.addIcon(); err != nil {
		return nil, err
	}
	if err = h.installHooks(); err != nil {
		return nil, err
	}
	return h, nil
}

func register(name string) uintptr {
	id, _, _ := registerMessage.Call(uintptr(unsafe.Pointer(wide(name))))
	return id
}

func (h *Host) PostTimer(token uintptr) { h.post(wmTimer, token, 0) }
func (h *Host) Quit()                   { h.post(wmClose, 0, 0) }
func (h *Host) post(message uint32, wparam, lparam uintptr) {
	if handle := h.window.Load(); handle != 0 {
		postMessage.Call(handle, uintptr(message), wparam, lparam)
	}
}

// Loop 和 Close 必须在创建窗口的系统线程上调用。
func (h *Host) Loop(ctx context.Context) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			h.Quit()
		case <-done:
		}
	}()
	var msg message
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return failed("GetMessageW", err)
		}
		if result == 0 {
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (h *Host) procedure(window uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmTray:
		if lparam == 0x0205 || lparam == 0x007B {
			h.openMenu()
		}
		return 0
	case wmAction:
		h.handler.Input(Action(wparam), lparam)
		return 0
	case wmTimer:
		h.handler.Timer(wparam)
		return 0
	case wmFocus:
		h.handler.Focus(lparam)
		return 0
	case wmCommand:
		h.handler.Command(uint32(wparam & 0xffff))
		return 0
	case wmClose:
		h.Close()
		return 0
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	}
	if msg == h.taskbarCreated && msg != 0 {
		if err := h.addIcon(); err != nil {
			h.handler.Failure(err)
		}
		return 0
	}
	result, _, _ := defWindowProc.Call(window, uintptr(msg), wparam, lparam)
	return result
}

func (h *Host) Close() {
	if h.closed {
		return
	}
	h.closed = true
	for _, hook := range []uintptr{h.keyboard, h.mouse} {
		if hook != 0 {
			unhook.Call(hook)
		}
	}
	if h.foreground != 0 {
		unhookWinEvent.Call(h.foreground)
	}
	window := h.window.Swap(0)
	if window != 0 {
		h.removeIcon(window)
		destroyWindow.Call(window)
	}
	if h.icon != 0 {
		destroyIcon.Call(h.icon)
	}
	unregisterClass.Call(uintptr(unsafe.Pointer(h.class)), h.module)
	runtime.KeepAlive(h.callbacks)
}

func createMenu(items []MenuItem) (uintptr, error) {
	menu, _, err := createPopupMenu.Call()
	if menu == 0 {
		return 0, failed("CreatePopupMenu", err)
	}
	for _, item := range items {
		flags, id := uintptr(0), uintptr(item.ID)
		if item.Text == "" {
			flags = 0x800
		}
		if item.Checked {
			flags |= 0x8
		}
		if item.Disabled {
			flags |= 0x3
		}
		if item.Submenu {
			id, err = createMenu(item.Children)
			if err != nil {
				destroyMenu.Call(menu)
				return 0, err
			}
			flags |= 0x10
		}
		if ok, _, callErr := appendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(wide(item.Text)))); ok == 0 {
			if item.Submenu {
				destroyMenu.Call(id)
			}
			destroyMenu.Call(menu)
			return 0, failed("AppendMenuW", callErr)
		}
	}
	return menu, nil
}

func (h *Host) openMenu() {
	menu, err := createMenu(h.handler.Menu())
	if err != nil {
		h.handler.Failure(err)
		return
	}
	defer destroyMenu.Call(menu)
	var p point
	if ok, _, err := getCursorPos.Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		h.handler.Failure(failed("GetCursorPos", err))
		return
	}
	window := h.window.Load()
	setForeground.Call(window)
	command, _, _ := trackPopupMenu.Call(menu, 0x0100|0x0002, uintptr(p.X), uintptr(p.Y), 0, window, 0)
	h.post(0, 0, 0)
	if command != 0 {
		h.handler.Command(uint32(command))
	}
}

func SingleInstance() (func(), bool, error) {
	handle, err := windows.CreateMutex(nil, false, wide("Local\\ScreenLayoutTool.Go"))
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { windows.CloseHandle(handle) }, true, nil
}
