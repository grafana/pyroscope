package adaptive_placement

import (
	"math/rand"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

func buildLBFunc(lb adaptive_placementpb.LoadBalancing) func(k placement.Key) func(int) int {
	switch lb {
	default:
		return pickFingerprintMod
	case adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN:
		return pickRoundRobin
	}
}

func pickRoundRobin(_ placement.Key) func(int) int {
	return func(n int) int {
		return rand.Intn(n)
	}
}

func pickFingerprintMod(k placement.Key) func(int) int {
	return func(n int) int {
		return int(k.Fingerprint) % n
	}
}

func loadBalancingFuncForDataset(*adaptive_placementpb.DatasetStats) adaptive_placementpb.LoadBalancing {
	// TODO(kolesnikovae): Adaptive load balancing.
	return adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT_MOD
}
