//go:build windows

package win32

import "testing"

func TestWindowShortcutModifiers(t *testing.T) {
	for _, test := range []struct{ win, shift, other, want bool }{
		{true, false, false, true},
		{true, true, false, false},
		{true, false, true, false},
		{false, false, false, false},
	} {
		if got := windowShortcut(test.win, test.shift, test.other); got != test.want {
			t.Fatalf("修饰键 %+v：得到 %v", test, got)
		}
	}
}

func TestMaskOnlyWinKeysUsedForShortcut(t *testing.T) {
	h := &Host{}
	if h.consumeWinMask(0x5B) {
		t.Fatal("单独按 Win 不应遮罩")
	}
	h.markShortcut(true, false)
	if h.consumeWinMask(0x5C) {
		t.Fatal("未使用的右 Win 不应遮罩")
	}
	if !h.consumeWinMask(0x5B) || h.consumeWinMask(0x5B) {
		t.Fatal("左 Win 遮罩应只消费一次")
	}
	h.markShortcut(true, true)
	if !h.consumeWinMask(0x5B) || !h.consumeWinMask(0x5C) {
		t.Fatal("同时按下的两个 Win 应分别遮罩")
	}
}
