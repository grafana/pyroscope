package raftnode

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/hashicorp/raft"
	raftwal "github.com/hashicorp/raft-wal"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

type Config struct {
	Dir string `yaml:"dir"`

	BootstrapPeers       []string `yaml:"bootstrap_peers"`
	BootstrapExpectPeers int      `yaml:"bootstrap_expect_peers"`

	ServerID         string `yaml:"server_id"`
	BindAddress      string `yaml:"bind_address"`
	AdvertiseAddress string `yaml:"advertise_address"`

	ApplyTimeout          time.Duration `yaml:"apply_timeout" doc:"hidden"`
	LogIndexCheckInterval time.Duration `yaml:"log_index_check_interval" doc:"hidden"`
	ReadIndexMaxDistance  uint64        `yaml:"read_index_max_distance" doc:"hidden"`

	WALCacheEntries       uint64        `yaml:"wal_cache_entries" doc:"hidden"`
	TrailingLogs          uint64        `yaml:"trailing_logs" doc:"hidden"`
	SnapshotsRetain       uint64        `yaml:"snapshots_retain" doc:"hidden"`
	SnapshotInterval      time.Duration `yaml:"snapshot_interval" doc:"hidden"`
	SnapshotThreshold     uint64        `yaml:"snapshot_threshold" doc:"hidden"`
	TransportConnPoolSize uint64        `yaml:"transport_conn_pool_size" doc:"hidden"`
	TransportTimeout      time.Duration `yaml:"transport_timeout" doc:"hidden"`
}

const (
	defaultWALCacheEntries       = 512
	defaultTrailingLogs          = 18 << 10
	defaultSnapshotsRetain       = 3
	defaultSnapshotInterval      = 180 * time.Second
	defaultSnapshotThreshold     = 8 << 10
	defaultTransportConnPoolSize = 10
	defaultTransportTimeout      = 10 * time.Second
)

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.Dir, prefix+"dir", "./data-metastore/raft", "")

	f.Var((*flagext.StringSlice)(&cfg.BootstrapPeers), prefix+"bootstrap-peers", "")
	f.IntVar(&cfg.BootstrapExpectPeers, prefix+"bootstrap-expect-peers", 1, "Expected number of peers including the local node.")

	f.StringVar(&cfg.ServerID, prefix+"server-id", "localhost:9099", "")
	f.StringVar(&cfg.BindAddress, prefix+"bind-address", "localhost:9099", "")
	f.StringVar(&cfg.AdvertiseAddress, prefix+"advertise-address", "localhost:9099", "")

	f.DurationVar(&cfg.ApplyTimeout, prefix+"apply-timeout", 5*time.Second, "")
	f.DurationVar(&cfg.LogIndexCheckInterval, prefix+"log-index-check-interval", 14*time.Millisecond, "")
	f.Uint64Var(&cfg.ReadIndexMaxDistance, prefix+"read-index-max-distance", 10<<10, "")

	f.Uint64Var(&cfg.WALCacheEntries, prefix+"wal-cache-entries", defaultWALCacheEntries, "")
	f.Uint64Var(&cfg.TrailingLogs, prefix+"trailing-logs", defaultTrailingLogs, "")
	f.Uint64Var(&cfg.SnapshotsRetain, prefix+"snapshots-retain", defaultSnapshotsRetain, "")
	f.DurationVar(&cfg.SnapshotInterval, prefix+"snapshot-interval", defaultSnapshotInterval, "")
	f.Uint64Var(&cfg.SnapshotThreshold, prefix+"snapshot-threshold", defaultSnapshotThreshold, "")
	f.Uint64Var(&cfg.TransportConnPoolSize, prefix+"transport-conn-pool-size", defaultTransportConnPoolSize, "")
	f.DurationVar(&cfg.TransportTimeout, prefix+"transport-timeout", defaultTransportTimeout, "")
}

func (cfg *Config) Validate() error {
	// TODO(kolesnikovae): Check the params.
	return nil
}

type Node struct {
	logger  log.Logger
	config  Config
	metrics *metrics
	reg     prometheus.Registerer
	fsm     raft.FSM

	walDir        string
	wal           *raftwal.WAL
	snapshots     *raft.FileSnapshotStore
	transport     *raft.NetworkTransport
	raft          *raft.Raft
	logStore      raft.LogStore
	stableStore   raft.StableStore
	snapshotStore raft.SnapshotStore

	observer *Observer
	service  *RaftNodeService
}

func NewNode(
	logger log.Logger,
	config Config,
	reg prometheus.Registerer,
	fsm raft.FSM,
) (_ *Node, err error) {
	n := Node{
		logger:  logger,
		config:  config,
		metrics: newMetrics(reg),
		reg:     reg,
		fsm:     fsm,
	}

	defer func() {
		if err != nil {
			// If the initialization fails, initialized components
			// should be de-initialized gracefully.
			n.Shutdown()
		}
	}()

	addr, err := net.ResolveTCPAddr("tcp", config.AdvertiseAddress)
	if err != nil {
		return nil, err
	}
	n.transport, err = raft.NewTCPTransport(
		config.BindAddress, addr,
		int(config.TransportConnPoolSize),
		config.TransportTimeout,
		os.Stderr)
	if err != nil {
		return nil, err
	}

	if err = n.openStore(); err != nil {
		return nil, err
	}

	return &n, nil
}

func (n *Node) Init() (err error) {
	raftConfig := raft.DefaultConfig()
	// TODO: Wrap gokit
	//	config.Logger
	raftConfig.LogLevel = "debug"

	raftConfig.TrailingLogs = n.config.TrailingLogs
	raftConfig.SnapshotThreshold = n.config.SnapshotThreshold
	raftConfig.SnapshotInterval = n.config.SnapshotInterval
	raftConfig.LocalID = raft.ServerID(n.config.ServerID)

	n.raft, err = raft.NewRaft(raftConfig, n.fsm, n.logStore, n.stableStore, n.snapshotStore, n.transport)
	if err != nil {
		return fmt.Errorf("starting raft node: %w", err)
	}
	n.observer = NewRaftStateObserver(n.logger, n.raft, n.metrics.state)
	n.service = NewRaftNodeService(n)

	hasState, err := raft.HasExistingState(n.logStore, n.stableStore, n.snapshotStore)
	if err != nil {
		return fmt.Errorf("failed to check for existing state: %w", err)
	}
	if !hasState {
		level.Warn(n.logger).Log("msg", "no existing state found, trying to bootstrap cluster")
		if err = n.bootstrap(); err != nil {
			return fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	} else {
		level.Debug(n.logger).Log("msg", "restoring existing state, not bootstrapping")
	}

	return nil
}

func (n *Node) openStore() (err error) {
	if err = n.createDirs(); err != nil {
		return err
	}
	n.wal, err = raftwal.Open(n.walDir)
	if err != nil {
		return fmt.Errorf("failed to open WAL: %w", err)
	}
	n.snapshots, err = raft.NewFileSnapshotStore(n.config.Dir, int(n.config.SnapshotsRetain), os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to open shapshot store: %w", err)
	}
	n.logStore = n.wal
	n.logStore, _ = raft.NewLogCache(int(n.config.WALCacheEntries), n.logStore)
	n.stableStore = n.wal
	n.snapshotStore = n.snapshots
	return nil
}

func (n *Node) createDirs() (err error) {
	n.walDir = filepath.Join(n.config.Dir, "wal")
	if err = os.MkdirAll(n.walDir, 0755); err != nil {
		return fmt.Errorf("WAL dir: %w", err)
	}
	if err = os.MkdirAll(n.config.Dir, 0755); err != nil {
		return fmt.Errorf("snapshot directory: %w", err)
	}
	return nil
}

func (n *Node) Shutdown() {
	if n.raft != nil {
		if err := n.raft.Shutdown().Error(); err != nil {
			level.Error(n.logger).Log("msg", "failed to shutdown raft", "err", err)
		}
		n.observer.Deregister()
	}
	if n.transport != nil {
		if err := n.transport.Close(); err != nil {
			level.Error(n.logger).Log("msg", "failed to close transport", "err", err)
		}
	}
	if n.wal != nil {
		if err := n.wal.Close(); err != nil {
			level.Error(n.logger).Log("msg", "failed to close WAL", "err", err)
		}
	}
}

func (n *Node) ListSnapshots() ([]*raft.SnapshotMeta, error) {
	return n.snapshots.List()
}

func (n *Node) Register(server *grpc.Server) {
	raftnodepb.RegisterRaftNodeServiceServer(server, n.service)
}

// LeaderActivity is started when the node becomes a leader and stopped
// when it stops being a leader. The implementation MUST be idempotent.
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

func (n *Node) RunOnLeader(a LeaderActivity) {
	n.observer.RegisterHandler(&leaderStateHandler{activity: a})
}

func (n *Node) TransferLeadership() (err error) {
	switch err = n.raft.LeadershipTransfer().Error(); {
	case err == nil:
	case errors.Is(err, raft.ErrNotLeader):
		// Not a leader, nothing to do.
	case strings.Contains(err.Error(), "cannot find peer"):
		// No peers, nothing to do.
	default:
		level.Error(n.logger).Log("msg", "failed to transfer leadership", "err", err)
	}
	return err
}

// Propose makes an attempt to apply the given command to the FSM.
// The function returns an error if node is not the leader.
func (n *Node) Propose(t fsm.RaftLogEntryType, m proto.Message) (resp proto.Message, err error) {
	raw, err := fsm.MarshalEntry(t, m)
	if err != nil {
		return nil, err
	}
	timer := prometheus.NewTimer(n.metrics.apply)
	defer timer.ObserveDuration()
	future := n.raft.Apply(raw, n.config.ApplyTimeout)
	if err = future.Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	r := future.Response().(fsm.Response)
	if r.Data != nil {
		resp = r.Data
	}
	return resp, r.Err
}
