package raftnode

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

func NewRaftStateObserver(logger log.Logger, r *raft.Raft, state *prometheus.GaugeVec) *Observer {
	o := &Observer{
		logger: logger,
		raft:   r,
		c:      make(chan raft.Observation, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		state:  state,
	}
	level.Debug(o.logger).Log("msg", "registering raft state observer")
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

func (o *Observer) Deregister() {
	close(o.stop)
	<-o.done
	level.Debug(o.logger).Log("msg", "deregistering raft observer")
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
	level.Debug(o.logger).Log("msg", "raft state changed", "raft_state", state)
	if o.state != nil {
		o.state.Reset()
		o.state.WithLabelValues(state.String()).Set(1)
	}
	for _, h := range o.handlers {
		h.Observe(state)
	}
}
