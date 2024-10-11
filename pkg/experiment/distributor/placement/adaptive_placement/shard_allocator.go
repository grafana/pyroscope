package adaptive_placement

import (
	"math"
)

// shardAllocator dynamically adjusts the number of shards allocated for a
// dataset based on observed data rates. The system is designed to scale out
// rapidly in response to increased load while scaling in more conservatively
// to prevent unnecessary shard churn.
//
// The system calculates the total data rate from incoming dataset statistics
// and determines the required number of shards based on a fixed unit size
// i.e., the portion of the rate that a single shard can handle. Note that
// it is expected that the rate values are aggregated over a time window and
// are not varying overly frequently.
//
// When the observed data rate increases, the system aggressively increases the
// number of shards. This is achieved using an exponential growth factor that
// doubles the shard allocation request on consecutive scale-out events. This
// allows preventing "laddering" (slow, step-wise shard increases) when load
// is growing steadily.
//
// To avoid the risk of premature shrinking that could cause oscillations, the
// system decreases the number of shards more cautiously. It enforces a minimum
// shard count over a configurable time window: the system doesn't allocate
// fewer shards than were allocated during the last window.
type shardAllocator struct {
	// Unit size denotes the portion of rate that needs
	// to be allocated to a single shard.
	unitSize uint64
	// Minimum  and maximum number of shards allowed.
	min, max int
	// Burst window specifies the time interval during which
	// the shard allocation delta multiplier grows on scale outs.
	burstWindow int64
	// Decay window specifies the minimal time interval
	// before the target number of shards can be decreased.
	decayWindow int64

	target      int   // Target number of shards.
	burstOffset int64 // Timestamp of the burst window start.
	multiplier  float64
	decayOffset int64 // Timestamp of the decay window start.
	previousMin int   // Minimum number of shards in the previous decay window.
	currentMin  int   // Minimum number of shards in the current decay window.
}

func newShardAllocator(limits PlacementLimits) *shardAllocator {
	a := new(shardAllocator)
	a.setLimits(limits)
	return a
}

func (a *shardAllocator) setLimits(limits PlacementLimits) {
	a.unitSize = limits.UnitSizeBytes
	a.min = int(limits.MinDatasetShards)
	a.max = int(limits.MaxDatasetShards)
	a.burstWindow = limits.BurstWindow.Nanoseconds()
	a.decayWindow = limits.DecayWindow.Nanoseconds()
}

func (a *shardAllocator) observe(usage uint64, now int64) int {
	target := int(usage/a.unitSize) + 1
	delta := target - a.target
	if delta > 0 {
		// Scale out.
		if a.burstOffset == 0 || now-a.burstOffset >= a.burstWindow {
			// Reset multiplier if burst window has passed.
			a.multiplier = 1
		} else {
			// Increase multiplier on consecutive
			// scale-outs within burst window.
			a.multiplier *= 2
			scaled := target + int(math.Ceil(float64(delta)*a.multiplier))
			target = min(2*target, scaled)
		}
		// Start/prolong burst window.
		a.burstOffset = now
	}
	if a.decayOffset == 0 || now-a.decayOffset >= a.decayWindow {
		a.previousMin, a.currentMin = a.currentMin, target
		a.decayOffset = now
	}
	a.currentMin = max(a.currentMin, target)
	a.target = min(a.max, max(a.min, a.previousMin, a.currentMin))
	return a.target
}
