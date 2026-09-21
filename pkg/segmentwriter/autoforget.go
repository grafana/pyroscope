package segmentwriter

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
)

// autoForget periodically removes ring members whose heartbeat has been stale
// for longer than the forget period. The segment-writer lifecycler keeps its
// ring entry on shutdown (-segment-writer.unregister-on-shutdown defaults to
// false) so that restarts do not disturb shard placement, but instances that
// never come back, e.g. after a scale-down, would otherwise stay in the ring
// until forgotten manually. This is the classic-lifecycler equivalent of
// dskit's AutoForgetDelegate.
type autoForget struct {
	services.Service

	kv           kv.Client
	instanceID   string
	forgetPeriod time.Duration
	logger       log.Logger
}

func newAutoForget(kvClient kv.Client, instanceID string, forgetPeriod, interval time.Duration, logger log.Logger) *autoForget {
	f := &autoForget{
		kv:           kvClient,
		instanceID:   instanceID,
		forgetPeriod: forgetPeriod,
		logger:       logger,
	}
	f.Service = services.NewTimerService(interval, nil, f.iteration, nil)
	return f
}

func (f *autoForget) iteration(ctx context.Context) error {
	var forgotten []string
	err := f.kv.CAS(ctx, RingKey, func(in any) (out any, retry bool, err error) {
		forgotten = forgotten[:0]
		if in == nil {
			return nil, false, nil
		}
		desc, ok := in.(*ring.Desc)
		if !ok {
			level.Warn(f.logger).Log("msg", fmt.Sprintf("auto-forget saw a KV store value that was not `ring.Desc`, got `%T`", in))
			return nil, false, nil
		}
		now := time.Now()
		for id, instance := range desc.Ingesters {
			if now.Sub(time.Unix(instance.GetTimestamp(), 0)) <= f.forgetPeriod {
				continue
			}
			if id == f.instanceID {
				// If our own heartbeat looks stale in our view of the ring,
				// the problem is more likely on our side (e.g. a network
				// partition), so we must not judge the other members.
				level.Warn(f.logger).Log("msg", "auto-forget sees its own instance as unhealthy, the network may be partitioned, skipping this round", "instance", id)
				forgotten = forgotten[:0]
				return nil, false, nil
			}
			forgotten = append(forgotten, id)
		}
		if len(forgotten) == 0 {
			return nil, false, nil
		}
		for _, id := range forgotten {
			desc.RemoveIngester(id)
		}
		return desc, true, nil
	})
	if err != nil {
		level.Warn(f.logger).Log("msg", "auto-forget failed to update the ring", "err", err)
		return nil
	}
	for _, id := range forgotten {
		level.Info(f.logger).Log("msg", "auto-forget removed instance from the ring because its heartbeat was stale for too long", "instance", id, "forget_period", f.forgetPeriod)
	}
	return nil
}
