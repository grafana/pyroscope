package raftleader

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
)

type HealthObserver struct {
	logger     log.Logger
	mu         sync.Mutex
	registered map[serviceKey]*raftService
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

func NewRaftLeaderHealthObserver(logger log.Logger, m *Metrics) *HealthObserver {
	return &HealthObserver{
		logger:     logger,
		metrics:    m,
		registered: make(map[serviceKey]*raftService),
	}
}

func (hs *HealthObserver) Register(r *raft.Raft, cb func(st raft.RaftState)) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	k := serviceKey{raft: r}
	if _, ok := hs.registered[k]; ok {
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
	hs.registered[k] = svc
}

func (hs *HealthObserver) Deregister(r *raft.Raft) {
	hs.mu.Lock()
	k := serviceKey{raft: r}
	svc, ok := hs.registered[k]
	delete(hs.registered, k)
	hs.mu.Unlock()
	if ok {
		close(svc.stop)
		<-svc.done
	}
}

type serviceKey struct {
	raft *raft.Raft
}

type raftService struct {
	hs       *HealthObserver
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
