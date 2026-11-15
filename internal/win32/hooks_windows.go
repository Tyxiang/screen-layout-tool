//go:build windows

package win32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type keyboardData struct {
	Key, ScanCode, Flags, Time uint32
	Extra                      uintptr
}

type mouseData struct {
	Point             point
	Data, Flags, Time uint32
	Extra             uintptr
}

func held(key uintptr) bool {
	state, _, _ := getAsyncKeyState.Call(key)
	return state&0x8000 != 0
}

func modifiers() (win, shift, other bool) {
	return held(0x5B) || held(0x5C), held(0x10), held(0x11) || held(0x12)
}

func windowShortcut(win, shift, other bool) bool { return win && !shift && !other }

func (h *Host) installHooks() error {
	keyboard := windows.NewCallback(func(code int32, wparam uintptr, data *keyboardData) uintptr {
		if code >= 0 && data.Flags&0x10 == 0 {
			if wparam == 0x0101 || wparam == 0x0105 {
				if h.suppressed[data.Key] {
					delete(h.suppressed, data.Key)
					return 1
				}
			}
			if wparam == 0x0100 || wparam == 0x0104 {
				win, shift, other := modifiers()
				if windowShortcut(win, shift, other) && (data.Key == 0x25 || data.Key == 0x27) {
					action := WindowPrevious
					if data.Key == 0x27 {
						action = WindowNext
					}
					h.suppressed[data.Key] = true
					h.sendAction(action)
					return 1
				}
			}
		}
		result, _, _ := nextHook.Call(0, uintptr(code), wparam, uintptr(unsafe.Pointer(data)))
		return result
	})
	var remainder int32
	mouse := windows.NewCallback(func(code int32, wparam uintptr, data *mouseData) uintptr {
		if code >= 0 && wparam == 0x020A && data.Flags&1 == 0 {
			win, shift, other := modifiers()
			if windowShortcut(win, shift, other) {
				remainder += int32(int16(data.Data >> 16))
				steps := remainder / 120
				remainder %= 120
				action := WindowPrevious
				if steps < 0 {
					action = WindowNext
					steps = -steps
				}
				for range steps {
					h.sendAction(action)
				}
				return 1
			}
			remainder = 0
		}
		result, _, _ := nextHook.Call(0, uintptr(code), wparam, uintptr(unsafe.Pointer(data)))
		return result
	})
	focus := windows.NewCallback(func(hook uintptr, event uint32, window uintptr, object, child int32, thread, timestamp uint32) uintptr {
		h.post(wmFocus, 0, window)
		return 0
	})
	h.callbacks = append(h.callbacks, keyboard, mouse, focus)
	var err error
	h.keyboard, _, err = setHook.Call(13, keyboard, h.module, 0)
	if h.keyboard == 0 {
		return failed("SetWindowsHookExW (keyboard)", err)
	}
	h.mouse, _, err = setHook.Call(14, mouse, h.module, 0)
	if h.mouse == 0 {
		return failed("SetWindowsHookExW (mouse)", err)
	}
	h.foreground, _, err = setWinEventHook.Call(3, 3, 0, focus, 0, 0, 0)
	if h.foreground == 0 {
		return failed("SetWinEventHook", err)
	}
	return nil
}

func (h *Host) sendAction(action Action) {
	// 非功能键标记本次 Win 组合已被使用，避免松开 Win 时弹出开始菜单。
	keybdEvent.Call(0xE8, 0, 0, 0)
	keybdEvent.Call(0xE8, 0, 2, 0)
	h.post(wmAction, uintptr(action), Foreground())
}
