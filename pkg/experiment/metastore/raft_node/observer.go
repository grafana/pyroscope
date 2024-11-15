package raft_node

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
)

// StateHandler is called every time the
// Raft state change is observed.
type StateHandler interface {
	Observe(raft.RaftState)
}

// LeaderActivity is started when the node becomes a
// leader and stopped when it stops being a leader.
// The implementation must be idempotent.
type LeaderActivity interface {
	Start()
	Stop()
}

type leaderStateHandler struct{ activity LeaderActivity }

func (h *leaderStateHandler) Observe(state raft.RaftState) {
	if state == raft.Leader {
		h.activity.Start()
	} else {
		h.activity.Stop()
	}
}

type Observer struct {
	logger   log.Logger
	raft     *raft.Raft
	observer *raft.Observer
	state    *prometheus.GaugeVec
	handlers []StateHandler
	c        chan raft.Observation
	stop     chan struct{}
	done     chan struct{}
}

func NewRaftStateObserver(logger log.Logger, r *raft.Raft, reg prometheus.Registerer) *Observer {
	o := &Observer{
		logger: logger,
		raft:   r,
		c:      make(chan raft.Observation, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	o.state = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Subsystem: "metastore",
			Name:      "raft_state",
			Help:      "Current Raft state",
		},
		[]string{"state"},
	)
	if reg != nil {
		reg.MustRegister(o.state)
	}
	_ = level.Debug(o.logger).Log("msg", "registering raft state observer")
	o.observer = raft.NewObserver(o.c, true, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.RaftState)
		return ok
	})
	r.RegisterObserver(o.observer)
	o.updateRaftState()
	go o.run()
	return o
}

func (o *Observer) RegisterHandler(h StateHandler) {
	o.handlers = append(o.handlers, h)
	o.updateRaftState()
}

func (o *Observer) OnLeader(a LeaderActivity) {
	o.RegisterHandler(&leaderStateHandler{activity: a})
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
			o.updateRaftState()
		case <-o.stop:
			return
		}
	}
}

func (o *Observer) updateRaftState() {
	state := o.raft.State()
	o.state.Reset()
	o.state.WithLabelValues(state.String()).Set(1)
	_ = level.Debug(o.logger).Log("msg", "raft state changed", "raft_state", state)
	for _, h := range o.handlers {
		h.Observe(state)
	}
}
