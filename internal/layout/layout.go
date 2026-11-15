// Package layout 处理布局解析和与操作系统无关的位置计算。
package layout

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var (
	ErrRead      = errors.New("cannot read layout file")
	ErrFormat    = errors.New("invalid layout format")
	ErrPosition  = errors.New("invalid layout position")
	ErrDuplicate = errors.New("duplicate layout positions")
)

// Rect 在布局中表示百分比，在显示器计算中表示物理像素。
type Rect struct {
	X, Y, Width, Height float64
}

type Layout struct {
	Author      string `json:"author"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Positions   []Rect `json:"-"`
}

// 可选字段使用指针，区分缺失和显式填写的零。
type position struct {
	X      *float64 `json:"x"`
	Y      *float64 `json:"y"`
	Left   *float64 `json:"left"`
	Right  *float64 `json:"right"`
	Top    *float64 `json:"top"`
	Bottom *float64 `json:"bottom"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

func Load(path string) (Layout, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Layout{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	return Parse(data)
}

func Parse(data []byte) (Layout, error) {
	var raw struct {
		Author      string     `json:"author"`
		Version     string     `json:"version"`
		Description string     `json:"description"`
		Positions   []position `json:"positions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Layout{}, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	if len(raw.Positions) == 0 {
		return Layout{}, fmt.Errorf("%w: positions must not be empty", ErrPosition)
	}
	result := Layout{Author: raw.Author, Version: raw.Version, Description: raw.Description}
	seen := make(map[Rect]bool)
	for i, p := range raw.Positions {
		x, width, err := axis(p.X, p.Left, p.Right, p.Width)
		if err != nil {
			return Layout{}, fmt.Errorf("%w: position %d horizontal: %v", ErrPosition, i+1, err)
		}
		y, height, err := axis(p.Y, p.Top, p.Bottom, p.Height)
		if err != nil {
			return Layout{}, fmt.Errorf("%w: position %d vertical: %v", ErrPosition, i+1, err)
		}
		r := Rect{x, y, width, height}
		if seen[r] {
			return Layout{}, fmt.Errorf("%w: position %d", ErrDuplicate, i+1)
		}
		seen[r] = true
		result.Positions = append(result.Positions, r)
	}
	return result, nil
}

func axis(coordinate, leading, trailing, size *float64) (float64, float64, error) {
	value := func(p *float64) float64 { return max(0, min(100, *p)) }
	var start, length float64
	switch {
	case coordinate != nil:
		start = value(coordinate)
	case trailing != nil && size != nil:
		start = 100 - value(trailing) - value(size)
	case leading != nil:
		start = value(leading)
	default:
		return 0, 0, errors.New("position cannot be determined")
	}
	switch {
	case size != nil:
		length = value(size)
	case leading != nil && trailing != nil:
		length = 100 - value(leading) - value(trailing)
	case coordinate != nil && trailing != nil:
		length = 100 - value(coordinate) - value(trailing)
	default:
		return 0, 0, errors.New("size cannot be determined")
	}
	end := min(100, start+length)
	start = max(0, start)
	length = end - start
	if length <= 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return 0, 0, errors.New("position has no area")
	}
	return start, length, nil
}

func Names(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			names = append(names, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		}
	}
	slices.Sort(names)
	return names, nil
}

func Current(names []string, name string) string {
	if slices.Contains(names, name) {
		return name
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

// 避免连续累计的大步数导致整数加法溢出。
func advance(index, step, count int) int {
	if step > 0 {
		return index + min(step, count-1-index)
	}
	return index + max(step, -index)
}

func Difference(a, b Rect) float64 {
	return math.Abs(a.X-b.X) + math.Abs(a.Y-b.Y) + math.Abs(a.Width-b.Width) + math.Abs(a.Height-b.Height)
}

// Target 的 current 和 positions 使用同一工作区域的百分比坐标。
func Target(positions []Rect, current Rect, step int) (Rect, bool) {
	if len(positions) == 0 || step == 0 {
		return Rect{}, false
	}
	closest, distance := 0, math.Inf(1)
	for i, p := range positions {
		if d := Difference(p, current); d < distance {
			closest, distance = i, d
		}
	}
	if distance < 0.5 {
		closest = advance(closest, step, len(positions))
	}
	return positions[closest], true
}

type Monitor struct {
	Number  int
	Primary bool
	Screen  Rect
	Work    Rect
}

func MonitorFor(window Rect, monitors []Monitor) (Monitor, error) {
	if len(monitors) == 0 {
		return Monitor{}, errors.New("no monitors available")
	}
	x, y := window.X+window.Width/2, window.Y+window.Height/2
	for _, m := range monitors {
		if x >= m.Screen.X && x < m.Screen.X+m.Screen.Width && y >= m.Screen.Y && y < m.Screen.Y+m.Screen.Height {
			return m, nil
		}
	}
	best, distance := monitors[0], math.Inf(1)
	for _, m := range monitors {
		dx := max(m.Screen.X-x, 0, x-m.Screen.X-m.Screen.Width)
		dy := max(m.Screen.Y-y, 0, y-m.Screen.Y-m.Screen.Height)
		d := dx*dx + dy*dy
		if d < distance || d == distance && (m.Primary && !best.Primary || m.Primary == best.Primary && m.Number < best.Number) {
			best, distance = m, d
		}
	}
	return best, nil
}

func Percent(window, work Rect) Rect {
	return Rect{100 * (window.X - work.X) / work.Width, 100 * (window.Y - work.Y) / work.Height, 100 * window.Width / work.Width, 100 * window.Height / work.Height}
}

func Pixels(position, work Rect) Rect {
	// 分别取整两条边，确保相邻区域的公共边保持一致。
	x := math.Round(work.X + work.Width*position.X/100)
	y := math.Round(work.Y + work.Height*position.Y/100)
	right := math.Round(work.X + work.Width*(position.X+position.Width)/100)
	bottom := math.Round(work.Y + work.Height*(position.Y+position.Height)/100)
	return Rect{x, y, right - x, bottom - y}
}
