package server

import "github.com/twmb/murmur3"

const seed = 6231912

type hashString string

func (hs hashString) Sum64() uint64 {
	return murmur3.SeedSum64(seed, []byte(hs))
}

func (ctrl *Controller) StatsInc(name string) {
	ctrl.statsMutex.Lock()
	defer ctrl.statsMutex.Unlock()

	ctrl.stats[name]++
}

func (ctrl *Controller) Stats() map[string]int {
	return ctrl.stats
}

func (ctrl *Controller) AppsCount() int {
	return int(ctrl.appStats.Count())
}
