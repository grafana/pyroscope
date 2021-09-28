package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
)

// made here http://patorjk.com/software/taag/#p=display&f=Doom&t=Pyroscope
var banner = `
 _ __  _   _ _ __ ___  ___  ___ ___  _ __   ___
| '_ \| | | | '__/ _ \/ __|/ __/ _ \| '_ \ / _ \
| |_) | |_| | | | (_) \__ \ (_| (_) | |_) |  __/
| .__/ \__, |_|  \___/|___/\___\___/| .__/ \___|
| |     __/ |                       | |
|_|    |___/                        |_|

`

func init() {
	// removes extra new lines
	banner = banner[1 : len(banner)-2]
}

func gradientBanner() string {
	return cli.GradientBanner(banner)
}
