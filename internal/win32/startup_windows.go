//go:build windows

package win32

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

var (
	shellLinkClass = ole.NewGUID("{00021401-0000-0000-C000-000000000046}")
	shellLinkID    = ole.NewGUID("{000214F9-0000-0000-C000-000000000046}")
	persistFileID  = ole.NewGUID("{0000010B-0000-0000-C000-000000000046}")
)

// Startup 仅管理本程序在当前用户启动目录中的快捷方式。
func Startup(executable string, enabled bool) error {
	directory, err := windows.KnownFolderPath(windows.FOLDERID_Startup, 0)
	if err != nil {
		return err
	}
	return Shortcut(filepath.Join(directory, "screen-layout-tool.lnk"), executable, enabled)
}

func Shortcut(path, executable string, enabled bool) error {
	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		// S_FALSE 表示当前线程此前已用相同方式初始化，仍须配对反初始化。
		if code, ok := err.(*ole.OleError); !ok || code.Code() != 1 {
			return err
		}
	}
	defer ole.CoUninitialize()
	link, err := ole.CreateInstance(shellLinkClass, shellLinkID)
	if err != nil {
		return err
	}
	defer link.Release()
	// IShellLinkW 的方法索引来自 Windows SDK 接口顺序。
	if err := comText(link, 20, executable); err != nil {
		return err
	}
	if err := comText(link, 9, filepath.Dir(executable)); err != nil {
		return err
	}
	if err := comText(link, 7, "Screen Layout Tool"); err != nil {
		return err
	}
	var persist *ole.IUnknown
	if err := link.PutQueryInterface(persistFileID, &persist); err != nil {
		return err
	}
	defer persist.Release()
	vtable := (*[9]uintptr)(unsafe.Pointer(persist.RawVTable))
	result, _, _ := syscall.SyscallN(vtable[6], uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(wide(path))), 1)
	return comError(result)
}

func comText(object *ole.IUnknown, index int, text string) error {
	vtable := (*[21]uintptr)(unsafe.Pointer(object.RawVTable))
	result, _, _ := syscall.SyscallN(vtable[index], uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(wide(text))))
	return comError(result)
}

func comError(result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("COM operation failed: HRESULT 0x%08X", uint32(result))
	}
	return nil
}
