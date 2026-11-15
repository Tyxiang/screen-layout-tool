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

type keyboardInput struct {
	Key, Scan   uint16
	Flags, Time uint32
	Extra       uintptr
}

// INPUT 的联合体按 MOUSEINPUT 的大小和对齐方式分配。
type nativeInput struct {
	Kind uint32
	Data struct {
		X, Y                   int32
		MouseData, Flags, Time uint32
		Extra                  uintptr
	}
}

const (
	leftWinMask  = 1
	rightWinMask = 2
)

func (h *Host) markShortcut(left, right bool) {
	if left {
		h.maskedWin |= leftWinMask
	}
	if right {
		h.maskedWin |= rightWinMask
	}
}

func (h *Host) consumeWinMask(key uint32) bool {
	var bit uint8
	switch key {
	case 0x5B:
		bit = leftWinMask
	case 0x5C:
		bit = rightWinMask
	default:
		return false
	}
	marked := h.maskedWin&bit != 0
	h.maskedWin &^= bit
	return marked
}

func maskWinRelease(data *keyboardData) bool {
	var inputs [3]nativeInput
	set := func(index int, key, scan uint16, flags uint32) {
		inputs[index].Kind = 1
		*(*keyboardInput)(unsafe.Pointer(&inputs[index].Data)) = keyboardInput{Key: key, Scan: scan, Flags: flags}
	}
	// 先按下并释放左 Ctrl，再释放对应 Win 键；全部事件一次性按序插入。
	set(0, 0x11, 0x1D, 0)
	set(1, 0x11, 0x1D, 2)
	set(2, uint16(data.Key), uint16(data.ScanCode), 2|data.Flags&1)
	sent, _, _ := sendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if sent == 1 {
		// 部分插入时释放合成的 Ctrl，原始 Win 释放仍继续传递。
		keybdEvent.Call(0x11, 0x1D, 2, 0)
	}
	return sent == uintptr(len(inputs))
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
				if h.consumeWinMask(data.Key) {
					_, shift, other := modifiers()
					if !shift && !other && maskWinRelease(data) {
						return 1
					}
				}
				if h.suppressed[data.Key] {
					delete(h.suppressed, data.Key)
					return 1
				}
			}
			if wparam == 0x0100 || wparam == 0x0104 {
				win, shift, other := modifiers()
				if windowShortcut(win, shift, other) && (data.Key == 0x25 || data.Key == 0x27) {
					h.markShortcut(held(0x5B), held(0x5C))
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
				h.markShortcut(held(0x5B), held(0x5C))
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
	h.post(wmAction, uintptr(action), Foreground())
}
