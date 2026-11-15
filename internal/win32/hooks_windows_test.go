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
