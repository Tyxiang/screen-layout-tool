//go:build windows

package win32

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
	"screen-layout-tool/internal/layout"
)

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) geometry() layout.Rect {
	return layout.Rect{X: float64(r.Left), Y: float64(r.Top), Width: float64(r.Right - r.Left), Height: float64(r.Bottom - r.Top)}
}

func Foreground() uintptr {
	handle, _, _ := getForeground.Call()
	return handle
}

func Manageable(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	if ok, _, _ := isWindow.Call(handle); ok == 0 {
		return false
	}
	for _, proc := range []*windows.LazyProc{getShellWindow, getDesktopWindow} {
		if h, _, _ := proc.Call(); handle == h {
			return false
		}
	}
	var name [256]uint16
	getClassName.Call(handle, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	switch windows.UTF16ToString(name[:]) {
	case "Shell_TrayWnd", "Shell_SecondaryTrayWnd", "Progman", "WorkerW":
		return false
	}
	var pid uint32
	getWindowThread.Call(handle, uintptr(unsafe.Pointer(&pid)))
	return pid != windows.GetCurrentProcessId()
}

func WindowRect(handle uintptr) (layout.Rect, error) {
	var r rect
	ok, _, err := getWindowRect.Call(handle, uintptr(unsafe.Pointer(&r)))
	if ok == 0 {
		return layout.Rect{}, failed("GetWindowRect", err)
	}
	return r.geometry(), nil
}

func Restore(handle uintptr) error {
	if zoomed, _, _ := isZoomed.Call(handle); zoomed != 0 {
		showWindow.Call(handle, 9)
		if zoomed, _, _ := isZoomed.Call(handle); zoomed != 0 {
			return errors.New("cannot restore maximized window")
		}
	}
	return nil
}

func Monitors() ([]layout.Monitor, error) {
	var monitors []layout.Monitor
	var callbackErr error
	// 回调只建立一次，重复枚举时不消耗新的运行时回调槽位。
	state := monitorEnumeration{monitors: &monitors, err: &callbackErr}
	ok, _, err := enumMonitors.Call(0, 0, monitorCallback, uintptr(unsafe.Pointer(&state)))
	if callbackErr != nil {
		return nil, callbackErr
	}
	if ok == 0 {
		return nil, failed("EnumDisplayMonitors", err)
	}
	return monitors, nil
}

type monitorEnumeration struct {
	monitors *[]layout.Monitor
	err      *error
}

var monitorCallback = windows.NewCallback(func(handle, dc uintptr, bounds *rect, state *monitorEnumeration) uintptr {
	var info struct {
		Size   uint32
		Screen rect
		Work   rect
		Flags  uint32
	}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, err := getMonitorInfo.Call(handle, uintptr(unsafe.Pointer(&info))); ok == 0 {
		*state.err = failed("GetMonitorInfoW", err)
		return 0
	}
	*state.monitors = append(*state.monitors, layout.Monitor{Number: len(*state.monitors) + 1, Primary: info.Flags&1 != 0, Screen: info.Screen.geometry(), Work: info.Work.geometry()})
	return 1
})

func Move(handle uintptr, target layout.Rect) error {
	if target.Width < 1 || target.Height < 1 {
		return errors.New("target window size is too small")
	}
	// 延续旧版经验证的两次设置，再读取窗口矩形校验结果。
	for range 2 {
		ok, _, err := setWindowPos.Call(handle, 0, uintptr(int32(target.X)), uintptr(int32(target.Y)), uintptr(int32(target.Width)), uintptr(int32(target.Height)), 0x0004|0x0010)
		if ok == 0 {
			return failed("SetWindowPos", err)
		}
	}
	actual, err := WindowRect(handle)
	if err != nil {
		return err
	}
	if math.Abs(actual.X-target.X) > 1 || math.Abs(actual.Y-target.Y) > 1 || math.Abs(actual.Width-target.Width) > 1 || math.Abs(actual.Height-target.Height) > 1 {
		return fmt.Errorf("window did not accept target position: wanted %+v, got %+v", target, actual)
	}
	return nil
}
