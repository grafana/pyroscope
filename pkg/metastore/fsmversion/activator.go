package fsmversion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
)

const nodeInfoTimeout = 10 * time.Second

type Raft interface {
	NodeInfo() (*raftnodepb.NodeInfo, error)
	Propose(context.Context, fsm.RaftLogEntryType, proto.Message) (proto.Message, error)
}

type Nodes interface {
	NodeInfoAll(context.Context) ([]*raftnodepb.NodeInfo, error)
}

type Activator struct {
	logger log.Logger
	config Config
	state  *State
	raft   Raft
	nodes  Nodes
	now    func() time.Time

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

type pendingActivation struct {
	version Version
	since   time.Time
}

func NewActivator(logger log.Logger, config Config, state *State, raft Raft, nodes Nodes) *Activator {
	return &Activator{
		logger: logger,
		config: config,
		state:  state,
		raft:   raft,
		nodes:  nodes,
		now:    time.Now,
	}
}

func (a *Activator) Start() {
	if a.config.CheckInterval <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.started = true
	go a.loop(ctx)
	level.Info(a.logger).Log("msg", "FSM version activator started", "supported_version", a.state.Supported(), "active_version", a.state.Active())
}

func (a *Activator) Stop() {
	if a.config.CheckInterval <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.started {
		return
	}
	a.cancel()
	a.started = false
	level.Info(a.logger).Log("msg", "FSM version activator stopped")
}

func (a *Activator) loop(ctx context.Context) {
	ticker := time.NewTicker(a.config.CheckInterval)
	defer ticker.Stop()
	var pending pendingActivation
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.activate(ctx, &pending); err != nil && !errors.Is(err, context.Canceled) {
				level.Warn(a.logger).Log("msg", "FSM version activation check failed", "err", err)
			}
		}
	}
}

func (a *Activator) activate(ctx context.Context, pending *pendingActivation) error {
	local, err := a.raft.NodeInfo()
	if err != nil {
		return fmt.Errorf("failed to get local node info: %w", err)
	}
	if local.State != raft.Leader.String() {
		return nil
	}

	active := a.state.Active()
	target := a.state.Supported()
	if limit := Version(a.config.MaxVersion); limit > 0 && limit < target {
		target = limit
	}
	if target <= active {
		*pending = pendingActivation{}
		return nil
	}

	if target, err = a.minSupportedVersion(ctx, local, target); err != nil {
		return err
	}
	if target <= active {
		*pending = pendingActivation{}
		return nil
	}

	if a.config.ActivationDelay > 0 {
		now := a.now()
		if pending.version != target {
			*pending = pendingActivation{version: target, since: now}
			level.Info(a.logger).Log(
				"msg", "all replicas support a newer FSM version; activation scheduled",
				"version", target,
				"active_version", active,
				"delay", a.config.ActivationDelay,
			)
			return nil
		}
		if now.Sub(pending.since) < a.config.ActivationDelay {
			return nil
		}
	}

	latest, err := a.raft.NodeInfo()
	if err != nil {
		return fmt.Errorf("failed to get local node info: %w", err)
	}
	if latest.ConfigurationIndex != local.ConfigurationIndex || latest.CurrentTerm != local.CurrentTerm {
		level.Info(a.logger).Log("msg", "raft configuration or term changed during the FSM version check; retrying later")
		return nil
	}

	cmd := fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_SET_FSM_VERSION)
	resp, err := a.raft.Propose(ctx, cmd, &raft_log.SetFSMVersionRequest{
		Term:    local.CurrentTerm,
		Version: uint32(target),
	})
	if err != nil {
		return fmt.Errorf("failed to propose FSM version %d: %w", target, err)
	}
	r, _ := resp.(*raft_log.SetFSMVersionResponse)
	if Version(r.GetVersion()) != target {
		level.Warn(a.logger).Log("msg", "FSM version change was not applied", "version", target, "active_version", r.GetVersion())
		return nil
	}
	*pending = pendingActivation{}
	return nil
}

func (a *Activator) minSupportedVersion(ctx context.Context, local *raftnodepb.NodeInfo, target Version) (Version, error) {
	ctx, cancel := context.WithTimeout(ctx, nodeInfoTimeout)
	defer cancel()
	infos, infoErr := a.nodes.NodeInfoAll(ctx)

	reported := make(map[string]Version, len(infos))
	for _, info := range infos {
		v := Version(info.SupportedFsmVersion)
		if prev, ok := reported[info.ServerId]; !ok || v < prev {
			reported[info.ServerId] = v
		}
	}
	reported[local.ServerId] = a.state.Supported()

	for _, peer := range local.Peers {
		v, ok := reported[peer.ServerId]
		if !ok {
			if infoErr != nil {
				return 0, fmt.Errorf("replica %s did not report its supported FSM version: %w", peer.ServerId, infoErr)
			}
			return 0, fmt.Errorf("replica %s did not report its supported FSM version", peer.ServerId)
		}
		if v < target {
			level.Debug(a.logger).Log("msg", "replica does not support the FSM version", "server_id", peer.ServerId, "supported_version", v, "version", target)
			target = v
		}
	}
	return target, nil
}
