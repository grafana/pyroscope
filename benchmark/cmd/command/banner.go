// Copied from github.com/pyroscope-io/pyroscope/cmd/command/banner.go
package command

import (
	"strings"

	"github.com/aybabtme/rgbterm"
	"github.com/fatih/color"
)

// made here http://patorjk.com/software/taag/#p=display&f=Doom&t=Pyrobench
var banner = `
______                _                     _
| ___ \              | |                   | |
| |_/ /   _ _ __ ___ | |__   ___ _ __   ___| |__
|  __/ | | | '__/ _ \| '_ \ / _ \ '_ \ / __| '_ \
| |  | |_| | | | (_) | |_) |  __/ | | | (__| | | |
\_|   \__, |_|  \___/|_.__/ \___|_| |_|\___|_| |_|
       __/ |
      |___/
`

func init() {
	// removes extra new lines
	banner = banner[1 : len(banner)-2]
}

const (
	startColor = 0xffd651
	endColor   = 0xf64d3d
)

func gradient(start, end, offset int, progress float64) uint8 {
	start = (start >> offset) & 0xff
	end = (end >> offset) & 0xff
	return uint8(start + int(float64(end-start)*progress))
}

func gradientBanner() string {
	if color.NoColor {
		return banner + "\n"
	}

	str := ""
	arr := strings.Split(banner, "\n")
	l := len(arr)
	for i, line := range arr {
		if line == "" {
			break
		}
		progress := float64(i) / float64(l-1)
		r := gradient(startColor, endColor, 16, progress)
		g := gradient(startColor, endColor, 8, progress)
		b := gradient(startColor, endColor, 0, progress)
		str += rgbterm.FgString(line, r, g, b) + "\n"
	}
	return str
}
