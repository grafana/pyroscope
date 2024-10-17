package raftleader

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
)

type LeaderObserver struct {
	logger    log.Logger
	metrics   *metrics
	raft      *raft.Raft
	observer  *raft.Observer
	c         chan raft.Observation
	stop      chan struct{}
	done      chan struct{}
	listeners []LeaderListener
}

type LeaderListener interface {
	OnLeaderChange(state raft.RaftState)
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

func NewRaftLeaderHealthObserver(r *raft.Raft, logger log.Logger, reg prometheus.Registerer) *LeaderObserver {
	return &LeaderObserver{
		logger:    logger,
		metrics:   newMetrics(reg),
		c:         make(chan raft.Observation, 1),
		raft:      r,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		listeners: make([]LeaderListener, 0),
	}
}

func (o *LeaderObserver) Start() {
	go o.run()
	_ = level.Debug(o.logger).Log("msg", "registering raft observer")
	o.observer = raft.NewObserver(o.c, true, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	o.raft.RegisterObserver(o.observer)
}

func (o *LeaderObserver) Stop() {
	close(o.stop)
	<-o.done
	_ = level.Debug(o.logger).Log("msg", "deregistering raft observer")
	o.raft.DeregisterObserver(o.observer)
}

func (o *LeaderObserver) AddListener(listener LeaderListener) {
	o.listeners = append(o.listeners, listener)
	_ = level.Debug(o.logger).Log("msg", "added leader listener")
	o.updateStatus(listener)
}

func (o *LeaderObserver) run() {
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

func (o *LeaderObserver) updateStatus(listeners ...LeaderListener) {
	state := o.raft.State()
	if len(listeners) > 0 {
		for _, listener := range listeners {
			listener.OnLeaderChange(state)
		}
	} else {
		for _, listener := range o.listeners {
			listener.OnLeaderChange(state)
		}
		o.metrics.status.Set(float64(state))
		_ = level.Info(o.logger).Log("msg", "updated raft state", "state", state)
	}
}
