package adaptive_placement

import (
	"errors"
	"fmt"
	"math"
	"math/rand"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

type LoadBalancing string

const (
	FingerprintLoadBalancing LoadBalancing = "fingerprint"
	RoundRobinLoadBalancing  LoadBalancing = "round-robin"
	DynamicLoadBalancing     LoadBalancing = "dynamic"
)

var ErrLoadBalancing = errors.New("invalid load balancing option")

var loadBalancingOptions = []LoadBalancing{
	FingerprintLoadBalancing,
	RoundRobinLoadBalancing,
	DynamicLoadBalancing,
}

const validOptionsString = "valid options: fingerprint, round-robin, dynamic"

func (lb *LoadBalancing) Set(text string) error {
	x := LoadBalancing(text)
	for _, name := range loadBalancingOptions {
		if x == name {
			*lb = x
			return nil
		}
	}
	return fmt.Errorf("%w: %s; %s", ErrLoadBalancing, x, validOptionsString)
}

func (lb *LoadBalancing) String() string { return string(*lb) }

func (lb LoadBalancing) proto() adaptive_placementpb.LoadBalancing {
	switch lb {
	default:
		return adaptive_placementpb.LoadBalancing_LOAD_BALANCING_UNSPECIFIED
	case DynamicLoadBalancing:
		return adaptive_placementpb.LoadBalancing_LOAD_BALANCING_UNSPECIFIED
	case RoundRobinLoadBalancing:
		return adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN
	case FingerprintLoadBalancing:
		return adaptive_placementpb.LoadBalancing_LOAD_BALANCING_FINGERPRINT
	}
}

func loadBalancingFromProto(lb adaptive_placementpb.LoadBalancing) LoadBalancing {
	switch lb {
	default:
		return FingerprintLoadBalancing
	case adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN:
		return RoundRobinLoadBalancing
	}
}

func (lb LoadBalancing) pick(k placement.Key) func(int) int {
	switch lb {
	default:
		return pickFingerprintMod(k)
	case RoundRobinLoadBalancing:
		return pickRoundRobin()
	}
}

func pickFingerprintMod(k placement.Key) func(int) int {
	return func(n int) int {
		return int(k.Fingerprint % uint64(n))
	}
}

func pickRoundRobin() func(int) int { return roundRobin }
func roundRobin(n int) int          { return rand.Intn(n) }

// needsDynamicBalancing returns true if the load balancing strategy
// should be chosen dynamically based on the dataset stats.
// x is the currently set load balancing strategy.
func (lb LoadBalancing) needsDynamicBalancing(x adaptive_placementpb.LoadBalancing) bool {
	// If the configured load balancing is "dynamic", we should
	// try to find the best strategy based on the dataset stats,
	// except if the x is already set to round-robin, which should
	// ensure the best distribution (from the available options).
	return lb == DynamicLoadBalancing && x != adaptive_placementpb.LoadBalancing_LOAD_BALANCING_ROUND_ROBIN
}

// loadBalancingStrategy chooses the load balancing strategy.
//
// By default, we adhere to the standard fingerprint-based distribution,
// since it provides slightly better locality in case if the dataset has
// enough keys to distribute. However, oftentimes this is not the case.
//
// If at least one shard is significantly overheated, and relative standard
// deviation withing the aggregation window is very high, which indicates
// that the distribution is uneven, we resort to round-robin load balancing.
func loadBalancingStrategy(stats *adaptive_placementpb.DatasetStats, unit uint64, target int) LoadBalancing {
	lb := FingerprintLoadBalancing
	if len(stats.Shards) < 2 {
		return lb
	}
	if p := float64(len(stats.Shards)) / float64(target); p > 2 || p < 0.5 {
		// It is possible that the dataset is being moved
		// to a different node, or different shards, or is
		// being scaled in/out, and therefore nonuniform
		// distribution might be expected within some period
		// of time. Moreover, there might be a sudden surge
		// in usage; together with high dispersion, this can
		// lead to false positives.
		return lb
	}
	t := 2 * unit
	var overheated bool
	for _, v := range stats.Usage {
		if v >= t {
			overheated = true
			break
		}
	}
	if !overheated {
		return lb
	}
	if float64(stats.StdDev)/float64(mean(stats.Usage)) < 0.5 {
		return lb
	}
	// Thresholds (2 x unit size, shards/target ratio, 0.5 RSD) are arbitrary
	// and can be adjusted. The current values are conservative and were chosen
	// to use RR as a last resort.
	return RoundRobinLoadBalancing
}

func stdDev(d []uint64) uint64 {
	if len(d) == 0 {
		return 0
	}
	m := mean(d)
	var variance uint64
	for _, v := range d {
		dev := v - m
		variance += dev * dev
	}
	variance /= uint64(len(d))
	return uint64(math.Sqrt(float64(variance)))
}

func mean(d []uint64) (m uint64) {
	if len(d) == 0 {
		return m
	}
	for _, v := range d {
		m += v
	}
	return m / uint64(len(d))
}

func sum(d []uint64) (s uint64) {
	for _, v := range d {
		s += v
	}
	return s
}
