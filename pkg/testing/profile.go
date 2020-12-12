package testing

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/pprof"
	"time"

	log "github.com/sirupsen/logrus"

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
			log.Debug("cmd", cmd)
			exec.Command("sh", "-c", cmd).Run()
		}()
	}
	cb()
	d := time.Now().Sub(t)
	return d
}

func Profile(name string, cb func()) time.Duration {
	t := time.Now()
	cb()
	d := time.Now().Sub(t)
	log.Debugf("%q took %s", name, d)
	return d
}

func PProfile(name string, cb func()) time.Duration {
	t := time.Now()
	f, err := os.Create("/tmp/profile-" + name + ".prof")
	if err = pprof.StartCPUProfile(f); err == nil {
		defer pprof.StopCPUProfile()
	}
	cb()
	d := time.Now().Sub(t)
	log.Debug(name, d)
	return d
}
