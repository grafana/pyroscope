package raftleader

import (
	"sync"

	"github.com/hashicorp/raft"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/pyroscope/pkg/util/health"
)

type HealthObserver struct {
	server health.Server

	mu         sync.Mutex
	registered map[serviceKey]*raftService
}

func NewRaftLeaderHealthObserver(hs health.Server) *HealthObserver {
	return &HealthObserver{
		server:     hs,
		registered: make(map[serviceKey]*raftService),
	}
}

func (hs *HealthObserver) Register(r *raft.Raft, service string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	k := serviceKey{raft: r, service: service}
	svc, ok := hs.registered[k]
	if ok {
		return
	}
	svc = &raftService{
		server:  hs.server,
		service: service,
		raft:    r,
		c:       make(chan raft.Observation, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	svc.updateStatus()
	go svc.run()
	svc.observer = raft.NewObserver(svc.c, true, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.LeaderObservation)
		return ok
	})
	r.RegisterObserver(svc.observer)
	hs.registered[k] = svc
}

func (hs *HealthObserver) Deregister(r *raft.Raft, service string) {
	hs.mu.Lock()
	k := serviceKey{raft: r, service: service}
	svc, ok := hs.registered[k]
	delete(hs.registered, k)
	hs.mu.Unlock()
	if ok {
		close(svc.stop)
		<-svc.done
	}
}

type serviceKey struct {
	raft    *raft.Raft
	service string
}

type raftService struct {
	server   health.Server
	service  string
	raft     *raft.Raft
	observer *raft.Observer
	c        chan raft.Observation
	stop     chan struct{}
	done     chan struct{}
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
			// We explicitly remove the service from serving when we stop observing it.
			svc.server.SetServingStatus(svc.service, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
			svc.raft.DeregisterObserver(svc.observer)
			return
		}
	}
}

func (svc *raftService) updateStatus() {
	status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if svc.raft.State() == raft.Leader {
		status = grpc_health_v1.HealthCheckResponse_SERVING
	}
	svc.server.SetServingStatus(svc.service, status)
}
