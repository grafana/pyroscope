package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
)

// made here http://patorjk.com/software/taag/#p=display&f=Doom&t=pyrobench
var banner = `
                       _                     _
                      | |                   | |
 _ __  _   _ _ __ ___ | |__   ___ _ __   ___| |__
| '_ \| | | | '__/ _ \| '_ \ / _ \ '_ \ / __| '_ \
| |_) | |_| | | | (_) | |_) |  __/ | | | (__| | | |
| .__/ \__, |_|  \___/|_.__/ \___|_| |_|\___|_| |_|
| |     __/ |
|_|    |___/
`

func init() {
	// removes extra new lines
	banner = banner[1 : len(banner)-2]
}

func gradientBanner() string {
	return cli.GradientBanner(banner)
}
