package raftleader

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
)

type LeaderObserver struct {
	logger     log.Logger
	mu         sync.Mutex
	registered *raftService
	metrics    *Metrics
}
type Metrics struct {
	status prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
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

func NewRaftLeaderHealthObserver(logger log.Logger, m *Metrics) *LeaderObserver {
	return &LeaderObserver{
		logger:     logger,
		metrics:    m,
		registered: nil,
	}
}

func (hs *LeaderObserver) Register(r *raft.Raft, cb func(st raft.RaftState)) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.registered != nil {
		return
	}
	svc := &raftService{
		hs:     hs,
		logger: hs.logger,
		raft:   r,
		cb:     cb,
		c:      make(chan raft.Observation, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	_ = level.Debug(svc.logger).Log("msg", "registering health check")
	svc.updateStatus()
	go svc.run()
	svc.observer = raft.NewObserver(svc.c, true, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	r.RegisterObserver(svc.observer)
	hs.registered = svc
}

func (hs *LeaderObserver) Deregister(r *raft.Raft) {
	hs.mu.Lock()
	svc := hs.registered
	ok := svc != nil
	hs.registered = nil
	hs.mu.Unlock()
	if ok {
		close(svc.stop)
		<-svc.done
	}
}

type raftService struct {
	hs       *LeaderObserver
	logger   log.Logger
	raft     *raft.Raft
	observer *raft.Observer
	c        chan raft.Observation
	stop     chan struct{}
	done     chan struct{}
	cb       func(st raft.RaftState)
}

func (svc *raftService) run() {
	defer func() {
		close(svc.done)
	}()
	for {
		select {
		case <-svc.c:
			svc.updateStatus()
		case <-svc.stop:
			_ = level.Debug(svc.logger).Log("msg", "deregistering health check")
			svc.raft.DeregisterObserver(svc.observer)
			return
		}
	}
}

func (svc *raftService) updateStatus() {
	state := svc.raft.State()
	svc.cb(state)
	svc.hs.metrics.status.Set(float64(state))
	_ = level.Info(svc.logger).Log("msg", "updated raft state", "state", state)
}
