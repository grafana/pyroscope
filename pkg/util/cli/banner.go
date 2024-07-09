package cli

import (
	"io"
	"strings"

	"github.com/aybabtme/rgbterm"
	"github.com/fatih/color"
)

const (
	startColor = 0xffd651
	endColor   = 0xf64d3d
)

func GradientBanner(banner string, w io.Writer) error {
	if color.NoColor {
		_, err := w.Write([]byte(banner + "\n"))
		return err
	}

	arr := strings.Split(banner, "\n")
	l := len(arr)
	for i, line := range arr {
		if len(line) == 0 {
			continue
		}
		progress := float64(i) / float64(l-1)
		r := gradient(startColor, endColor, 16, progress)
		g := gradient(startColor, endColor, 8, progress)
		b := gradient(startColor, endColor, 0, progress)
		_, err := w.Write([]byte(rgbterm.FgString(line, r, g, b) + "\n"))
		if err != nil {
			return nil
		}
	}
	return nil
}

func gradient(start, end, offset int, progress float64) uint8 {
	start = (start >> offset) & 0xff
	end = (end >> offset) & 0xff
	return uint8(start + int(float64(end-start)*progress))
}
