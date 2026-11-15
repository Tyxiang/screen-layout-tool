package app

import "strings"

// 原生菜单把 & 当作快捷键标记，布局文件名中的 & 必须转义。
func escapeMenu(name string) string { return strings.ReplaceAll(name, "&", "&&") }
