//go:build windows

package win32

import (
	"encoding/binary"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

type notifyIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Timeout             uint32
	Title               [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

func iconFromICO(data []byte) (uintptr, error) {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return 0, errors.New("invalid application icon")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || len(data) < 6+16*count {
		return 0, errors.New("invalid application icon directory")
	}
	chosen := data[6:22]
	for i := range count {
		entry := data[6+i*16 : 22+i*16]
		if entry[0] == 32 && entry[1] == 32 {
			chosen = entry
			break
		}
	}
	size, offset := uint64(binary.LittleEndian.Uint32(chosen[8:12])), uint64(binary.LittleEndian.Uint32(chosen[12:16]))
	if size == 0 || offset+size > uint64(len(data)) {
		return 0, errors.New("invalid application icon image")
	}
	proc := user32.NewProc("CreateIconFromResourceEx")
	icon, _, err := proc.Call(uintptr(unsafe.Pointer(&data[offset])), uintptr(size), 1, 0x00030000, 32, 32, 0)
	if icon == 0 {
		return 0, failed("CreateIconFromResourceEx", err)
	}
	return icon, nil
}

func copyText(destination []uint16, text string) {
	encoded := windows.StringToUTF16(text)
	if len(encoded) > len(destination) {
		encoded = encoded[:len(destination)-1]
	}
	copy(destination, encoded)
	destination[len(destination)-1] = 0
}

func (h *Host) iconData() notifyIconData {
	data := notifyIconData{Window: h.window.Load(), ID: 1, Icon: h.icon, Flags: 1 | 2 | 4, Callback: wmTray}
	data.Size = uint32(unsafe.Sizeof(data))
	copyText(data.Tip[:], "Screen Layout Tool")
	return data
}

func (h *Host) addIcon() error {
	data := h.iconData()
	if ok, _, err := shellNotifyIcon.Call(0, uintptr(unsafe.Pointer(&data))); ok == 0 {
		return failed("Shell_NotifyIconW", err)
	}
	return nil
}

func (h *Host) removeIcon(window uintptr) {
	data := h.iconData()
	data.Window = window
	shellNotifyIcon.Call(2, uintptr(unsafe.Pointer(&data)))
}

func (h *Host) Notify(title, text string, warning bool) {
	data := h.iconData()
	data.Flags = 0x10
	data.InfoFlags = 1
	if warning {
		data.InfoFlags = 2
	}
	copyText(data.Title[:], title)
	copyText(data.Info[:], text)
	shellNotifyIcon.Call(1, uintptr(unsafe.Pointer(&data)))
}
