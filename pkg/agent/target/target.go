package target

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrNotRunning = errors.New("not running")
)

const (
	defaultBackoffPeriod = time.Second * 10
)

// Manager tracks targets and attaches spies to running processes.
type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc

	logger agent.Logger
	agent  *config.Agent
	stop   chan struct{}
	wg     sync.WaitGroup

	resolve       func(config.Target) (target, bool)
	backoffPeriod time.Duration
}

func NewManager(logger agent.Logger, agentConfig *config.Agent) *Manager {
	mgr := Manager{
		logger:        logger,
		agent:         agentConfig,
		stop:          make(chan struct{}),
		backoffPeriod: defaultBackoffPeriod,
	}
	mgr.ctx, mgr.cancel = context.WithCancel(context.Background())
	mgr.resolve = mgr.resolveTarget
	return &mgr
}

func (mgr *Manager) Start() error {
	for _, t := range mgr.agent.Targets {
		tgt, ok := mgr.resolve(t)
		if !ok {
			return fmt.Errorf("unknown target type")
		}
		mgr.wg.Add(1)
		go mgr.runTarget(tgt)
	}
	return nil
}

func (mgr *Manager) Stop() {
	mgr.cancel()
	close(mgr.stop)
	mgr.wg.Wait()
}

type target interface {
	// attach blocks till the context cancellation or the target
	// process exit, whichever occurs first.
	attach(ctx context.Context)
}

func (mgr *Manager) resolveTarget(t config.Target) (target, bool) {
	var tgt target
	switch {
	case t.ServiceName != "":
		tgt = newServiceTarget(mgr.logger, mgr.agent, t)
	default:
		return nil, false
	}
	return tgt, true
}

func (mgr *Manager) runTarget(t target) {
	ticker := time.NewTicker(mgr.backoffPeriod)
	defer func() {
		ticker.Stop()
		mgr.wg.Done()
	}()
	for {
		select {
		default:
		case <-mgr.stop:
			return
		}
		t.attach(mgr.ctx)
		// Unless manager is stopped, run spy again after some backoff
		// period regardless of the exit reason.
		select {
		case <-ticker.C:
		case <-mgr.stop:
			return
		}
	}
}
