package layout

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParsePositions(t *testing.T) {
	tests := []struct {
		name, fields string
		want         Rect
	}{
		{"坐标与尺寸", `"x":10,"width":30,"y":20,"height":40`, Rect{10, 20, 30, 40}},
		{"前缘与尺寸", `"left":10,"width":30,"top":20,"height":40`, Rect{10, 20, 30, 40}},
		{"后缘与尺寸", `"right":60,"width":30,"bottom":40,"height":40`, Rect{10, 20, 30, 40}},
		{"两侧边距", `"left":10,"right":60,"top":20,"bottom":40`, Rect{10, 20, 30, 40}},
		{"坐标与后缘", `"x":10,"right":60,"y":20,"bottom":40`, Rect{10, 20, 30, 40}},
		{"显式零值", `"x":0,"width":100,"y":0,"height":100`, Rect{0, 0, 100, 100}},
		{"小数", `"x":12.5,"width":37.5,"y":0,"height":100`, Rect{12.5, 0, 37.5, 100}},
		{"字段优先级", `"x":10,"left":80,"right":80,"width":30,"y":20,"top":90,"bottom":90,"height":40`, Rect{10, 20, 30, 40}},
		{"尺寸优先级", `"x":10,"left":20,"right":30,"y":20,"top":30,"bottom":40`, Rect{10, 20, 50, 30}},
		{"原始值限制", `"x":-10,"width":120,"y":-20,"height":200`, Rect{0, 0, 100, 100}},
		{"右下裁剪", `"x":80,"width":40,"y":90,"height":30`, Rect{80, 90, 20, 10}},
		{"左上裁剪", `"right":80,"width":40,"bottom":90,"height":30`, Rect{0, 0, 20, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := Parse([]byte(`{"positions":[{` + tt.fields + `}]}`))
			if err != nil {
				t.Fatal(err)
			}
			if l.Positions[0] != tt.want {
				t.Fatalf("得到 %+v，期望 %+v", l.Positions[0], tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidLayouts(t *testing.T) {
	tests := []struct {
		data string
		want error
	}{
		{`{`, ErrFormat},
		{`{"positions":[{"width":"50"}]}`, ErrFormat},
		{`null`, ErrPosition},
		{`{"positions":[]}`, ErrPosition},
		{`{"positions":[{"width":50,"height":100}]}`, ErrPosition},
		{`{"positions":[{"x":0,"width":0,"y":0,"height":100}]}`, ErrPosition},
		{`{"positions":[{"left":80,"right":30,"top":0,"height":100}]}`, ErrPosition},
		{`{"positions":[{"x":100,"width":50,"y":0,"height":100}]}`, ErrPosition},
		{`{"positions":[{"x":-5,"width":100,"y":0,"height":100},{"left":0,"right":0,"top":0,"bottom":0}]}`, ErrDuplicate},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.data))
		if !errors.Is(err, tt.want) {
			t.Errorf("%s：得到 %v，期望 %v", tt.data, err, tt.want)
		}
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); !errors.Is(err, ErrRead) {
		t.Fatal(err)
	}
}

func TestBasicLayout(t *testing.T) {
	_, err := Load(filepath.Join("..", "..", "layouts", "Basic.json"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestTarget(t *testing.T) {
	positions := []Rect{{0, 0, 20, 100}, {20, 0, 20, 100}, {40, 0, 60, 100}}
	tests := []struct {
		name       string
		current    Rect
		step, want int
		move       bool
	}{
		{"首次只到最近位置", Rect{21, 1, 22, 99}, 20, 1, true},
		{"首次反向仍到最近位置", Rect{21, 1, 22, 99}, -20, 1, true},
		{"已定位向前累计", positions[0], 2, 2, true},
		{"已定位向后累计", positions[2], -2, 0, true},
		{"上界停止", positions[2], math.MaxInt, 2, true},
		{"下界停止", positions[0], math.MinInt, 0, true},
		{"步数抵消", positions[0], 0, 0, false},
		{"阈值内继续前进", Rect{0.49, 0, 20, 100}, 1, 1, true},
		{"阈值本身不前进", Rect{0.5, 0, 20, 100}, 1, 0, true},
		{"同距取列表首项", Rect{10, 0, 20, 100}, 1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, move := Target(positions, tt.current, tt.step)
			if move != tt.move || move && got != positions[tt.want] {
				t.Fatalf("得到 %+v/%v，期望索引 %d/%v", got, move, tt.want, tt.move)
			}
		})
	}
}

func TestMonitorsAndPixels(t *testing.T) {
	monitors := []Monitor{
		{Number: 1, Screen: Rect{-1920, 0, 1920, 1080}, Work: Rect{-1920, 0, 1920, 1040}},
		{Number: 2, Primary: true, Screen: Rect{0, 0, 2560, 1440}, Work: Rect{0, 0, 2560, 1400}},
	}
	left, err := MonitorFor(Rect{-2000, 100, 300, 300}, monitors)
	if err != nil || left.Number != 1 {
		t.Fatalf("左侧负坐标显示器：%+v %v", left, err)
	}
	nearest, _ := MonitorFor(Rect{3000, 100, 200, 200}, monitors)
	if nearest.Number != 2 {
		t.Fatal(nearest)
	}
	tie := []Monitor{{Number: 2, Primary: true, Screen: Rect{20, 0, 10, 10}}, {Number: 1, Screen: Rect{0, 0, 10, 10}}}
	chosen, _ := MonitorFor(Rect{14, 4, 2, 2}, tie)
	if chosen.Number != 2 {
		t.Fatal("等距时未优先主显示器")
	}
	tie[0].Primary = false
	chosen, _ = MonitorFor(Rect{14, 4, 2, 2}, tie)
	if chosen.Number != 1 {
		t.Fatal("等距时未优先较小编号")
	}
	if _, err := MonitorFor(Rect{}, nil); err == nil {
		t.Fatal("无显示器应报错")
	}
	work := Rect{-1920, 20, 1920, 1040}
	percent := Rect{20, 0, 40, 50}
	pixel := Pixels(percent, work)
	if Difference(Percent(pixel, work), percent) > 0.01 {
		t.Fatal(pixel)
	}
	first := Pixels(Rect{0, 0, 33.3, 100}, Rect{0, 0, 1919, 1039})
	second := Pixels(Rect{33.3, 0, 66.7, 100}, Rect{0, 0, 1919, 1039})
	if first.X+first.Width != second.X || second.X+second.Width != 1919 {
		t.Fatal("相邻区域边缘不一致")
	}
}

func TestNamesAndCurrent(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"Z.json", "Basic.JSON", "A.json", "ignored.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	names, err := Names(directory)
	if err != nil || !reflect.DeepEqual(names, []string{"A", "Basic", "Z"}) {
		t.Fatalf("%v %v", names, err)
	}
	if Current(names, "missing") != "A" || Current(names, "Basic") != "Basic" || Current(nil, "Basic") != "" {
		t.Fatal("当前布局选择不符合规则")
	}
}
