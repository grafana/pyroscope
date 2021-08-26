package testing

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/pprof"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/felixge/fgprof"
)

func FProfile(name string, cb func()) time.Duration {
	t := time.Now()
	path := "/tmp/profile-" + name + ".folded"
	pathSVG := "/tmp/profile-" + name + ".svg"
	f, err := os.Create(path)
	if err == nil {
		endProfile := fgprof.Start(f, fgprof.FormatFolded)
		defer func() {
			endProfile()
			cmd := fmt.Sprintf("cat '%s' | grep -v gopark | flamegraph.pl > '%s'", path, pathSVG)
			logrus.Debug("cmd", cmd)
			err := exec.Command("sh", "-c", cmd).Run()
			if err != nil {
				panic(err)
			}
		}()
	}
	cb()
	d := time.Since(t)
	return d
}

func Profile(name string, cb func()) time.Duration {
	t := time.Now()
	cb()
	d := time.Since(t)
	logrus.Debugf("%q took %s", name, d)
	return d
}

func PProfile(name string, cb func()) time.Duration {
	t := time.Now()
	f, err := os.Create("/tmp/profile-" + name + ".prof")
	if err = pprof.StartCPUProfile(f); err == nil {
		defer pprof.StopCPUProfile()
	}
	cb()
	d := time.Since(t)
	logrus.Debug(name, d)
	return d
}
