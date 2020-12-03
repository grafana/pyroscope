package timing

import (
	"encoding/json"
	"fmt"
	"time"
)

type prettyJsonDuration time.Duration

func (pjd prettyJsonDuration) MarshalJSON() ([]byte, error) {
	d := time.Duration(pjd)
	return json.Marshal(fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000))
}

type Timer struct {
	parent    *Timer
	startTime time.Time
	results   map[string]prettyJsonDuration
}

func new(parent *Timer) *Timer {
	t := &Timer{
		parent:    parent,
		startTime: time.Now(),
		results:   make(map[string]prettyJsonDuration),
	}
	return t
}

func New() *Timer {
	return new(nil)
}

func (t *Timer) Start() *Timer {
	return new(t)
}

func (t *Timer) End(name string) time.Duration {
	dur := time.Now().Sub(t.startTime)
	for ; t.parent != nil; t = t.parent {
	}
	t.results[name] = prettyJsonDuration(dur)
	return dur
}

func (t *Timer) Measure(name string, cb func()) time.Duration {
	t2 := t.Start()
	cb()
	return t2.End(name)
}

// func (t *Timer) MeasureAndProfile(name string, cb func()) time.Duration {
// 	f, err := os.Create("/tmp/profile-" + name + ".folded")
// 	if err != nil {
// 		panic(err)
// 	}
// 	endProfile := fgprof.Start(f, fgprof.FormatFolded)
// 	defer func(){
// 		endProfile()
// 		exec.Command("sh", "-c" "flamegraph.pl")
// 	}
// 	return t.Measure(name, cb)
// }

func (t *Timer) Marshal() []byte {
	b, _ := json.MarshalIndent(t.results, "", "  ")
	return b
}

// func measure(name string, cb func()) time.Duration {
// 	t := time.Now()
// 	f, err := os.Create("/tmp/profile-" + name + ".folded")
// 	// if name == "render" {
// 	if err == nil {
// 		endProfile := fgprof.Start(f, fgprof.FormatFolded)
// 		defer endProfile()
// 	}
// 	// }
// 	// f, err := os.Create("/tmp/profile-" + name + ".prof")
// 	// if err = pprof.StartCPUProfile(f); err == nil {
// 	// 	defer pprof.StopCPUProfile()
// 	// }
// 	cb()
// 	d := time.Now().Sub(t)
// 	log.Debug(name, d)
// 	return d
// }
