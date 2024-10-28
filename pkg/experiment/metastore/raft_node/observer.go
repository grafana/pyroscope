package raft_node

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
)

type Observer struct {
	logger   log.Logger
	metrics  *metrics
	raft     *raft.Raft
	observer *raft.Observer
	c        chan raft.Observation
	stop     chan struct{}
	done     chan struct{}
	cb       func(st raft.RaftState)
}

type metrics struct {
	status prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		status: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Name:      "metastore_raft_status",
		}),
	}
	if reg != nil {
		reg.MustRegister(m.status)
	}
	return m
}

func NewRaftLeaderObserver(logger log.Logger, reg prometheus.Registerer) *Observer {
	return &Observer{
		logger:  logger,
		metrics: newMetrics(reg),
		c:       make(chan raft.Observation, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (o *Observer) Register(r *raft.Raft, cb func(st raft.RaftState)) {
	if o.raft != nil {
		return
	}
	o.raft = r
	o.cb = cb
	_ = level.Debug(o.logger).Log("msg", "registering leader observer")
	o.updateStatus()
	go o.run()
	o.observer = raft.NewObserver(o.c, true, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	r.RegisterObserver(o.observer)
}

func (o *Observer) Deregister() {
	close(o.stop)
	<-o.done
	_ = level.Debug(o.logger).Log("msg", "deregistering raft observer")
	o.raft.DeregisterObserver(o.observer)
}

func (o *Observer) run() {
	defer func() {
		close(o.done)
	}()
	for {
		select {
		case <-o.c:
			o.updateStatus()
		case <-o.stop:
			return
		}
	}
}

func (o *Observer) updateStatus() {
	state := o.raft.State()
	if o.cb != nil {
		o.cb(state)
	}
	o.metrics.status.Set(float64(state))
	_ = level.Info(o.logger).Log("msg", "updated raft state", "state", state)
}
