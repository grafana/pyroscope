package godeltaprof

import (
	"github.com/pyroscope-io/godeltaprof/internal/pprof"
	"io"
	"runtime"
	"sort"
)

type BlockProfiler struct {
	impl           pprof.DeltaMutexProfiler
	runtimeProfile func([]runtime.BlockProfileRecord) (int, bool)
	scaleProfile   func(int64, float64) (int64, float64)
}

func NewMutexProfiler() *BlockProfiler {
	return &BlockProfiler{runtimeProfile: runtime.MutexProfile, scaleProfile: scaleMutexProfile}
}
func NewBlockProfiler() *BlockProfiler {
	return &BlockProfiler{runtimeProfile: runtime.BlockProfile, scaleProfile: scaleBlockProfile}
}

func (d *BlockProfiler) Profile(w io.Writer) error {
	var p []runtime.BlockProfileRecord
	n, ok := d.runtimeProfile(nil)
	for {
		p = make([]runtime.BlockProfileRecord, n+50)
		n, ok = d.runtimeProfile(p)
		if ok {
			p = p[:n]
			break
		}
	}

	sort.Slice(p, func(i, j int) bool { return p[i].Cycles > p[j].Cycles })

	return d.impl.PrintCountCycleProfile(w, "contentions", "delay", d.scaleProfile, p)
}

func scaleMutexProfile(cnt int64, ns float64) (int64, float64) {
	period := runtime.SetMutexProfileFraction(-1)
	return cnt * int64(period), ns * float64(period)
}

func scaleBlockProfile(cnt int64, ns float64) (int64, float64) {
	// Do nothing.
	// The current way of block profile sampling makes it
	// hard to compute the unsampled number. The legacy block
	// profile parse doesn't attempt to scale or unsample.
	return cnt, ns
}
