package svg

import (
	"fmt"
	"io"
	"math"
	"text/template"

	htmlTemplate "html/template"

	"github.com/spaolacci/murmur3"
)

type Header struct {
	Width  int
	Height int
	LabelY int
	TitleX int
}

type Box struct {
	X       float64
	Y       float64
	X2      float64
	Y2      float64
	Width   float64
	Height  float64
	Samples uint64
	Percent float64
	Label   string
	Color   string
}

var BoxTmplt *htmlTemplate.Template
var HeaderTmplt *template.Template

func init() {
	BoxTmplt, _ = htmlTemplate.New("box").Parse(`
	<g class="func_g" onmouseover="s(this)" onmouseout="c()" onclick="zoom(this)">
	<title>{{ .Label }} ({{ .Samples }} samples, {{ .Percent }}%)</title><rect x="{{ .X }}" y="{{ .Y }}" width="{{ .Width }}" height="{{ .Height }}" fill="{{ .Color }}" rx="2" ry="2" />
	<text text-anchor="" x="{{ .X2 }}" y="{{ .Y2 }}" font-size="12" font-family="system-ui, -apple-system, 'Segoe UI', 'Roboto', 'Ubuntu', 'Cantarell', 'Noto Sans', sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji', 'Segoe UI Symbol', 'Noto Color Emoji'" fill="#111"  ></text>
	</g>
	`)
	HeaderTmplt, _ = template.New("header").Parse(HeaderStr)
}

var Hd = float64(53 - 37)
var tm = float64(37)
var textx = float64(3.0)
var texty = float64(10.5)

func RenderBlock(w io.Writer, label []byte, level, samples uint64, width, x, perc, self float64, childrenCount int) {
	// templates are too slow
	color := colorRand(label, self, childrenCount)
	safeLabel := string(label)
	BoxTmplt.Execute(w, Box{
		X:       x,
		Y:       tm + float64(level)*Hd,
		X2:      x + textx,
		Y2:      tm + float64(level)*Hd + texty,
		Width:   width,
		Height:  15.0,
		Samples: samples,
		Percent: perc,
		Label:   safeLabel,
		Color:   string(color),
	})
}

var Margin = float64(10.0)

func colorRand(label []byte, self float64, childrenCount int) string {
	// TODO: could be faster?
	u1, u2 := murmur3.Sum128WithSeed(label, 6231912)

	r := 205 + u2%50
	g := 100 + u1%70
	b := 0 + u2%55
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}

func colorSelf(label []byte, self float64, childrenCount int) string {
	r := 255
	self = 1.0 - self
	v := math.Pow(self, 6) * 255.0
	g := int(v)
	b := int(v)
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}

func colorChildren(label []byte, self float64, childrenCount int) string {
	r := 255
	childrenCount--
	self = 1.0 - float64(childrenCount)/10
	if self < 0 {
		self = 0.0
	}
	if self > 1 {
		self = 1.0
	}
	v := math.Pow(self, 6) * 255.0
	g := int(v)
	b := int(v)
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}
