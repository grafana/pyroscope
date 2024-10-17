package metastore

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/dns"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/hashicorp/raft"
	raftwal "github.com/hashicorp/raft-wal"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/dlq"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftleader"
)

const (
	snapshotsRetain       = 3
	walCacheEntries       = 512
	transportConnPoolSize = 10
	transportTimeout      = 10 * time.Second

	raftTrailingLogs      = 18 << 10
	raftSnapshotInterval  = 180 * time.Second
	raftSnapshotThreshold = 8 << 10
)

type Config struct {
	Address           string            `yaml:"address"`
	GRPCClientConfig  grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the metastore."`
	DataDir           string            `yaml:"data_dir"`
	Raft              RaftConfig        `yaml:"raft"`
	Compaction        CompactionConfig  `yaml:"compaction_config"`
	MinReadyDuration  time.Duration     `yaml:"min_ready_duration" category:"advanced"`
	DLQRecoveryPeriod time.Duration     `yaml:"dlq_recovery_period" category:"advanced"`
	Index             index.Config      `yaml:"index_config"`
}

type RaftConfig struct {
	Dir string `yaml:"dir"`

	BootstrapPeers       []string `yaml:"bootstrap_peers"`
	BootstrapExpectPeers int      `yaml:"bootstrap_expect_peers"`

	ServerID         string `yaml:"server_id"`
	BindAddress      string `yaml:"bind_address"`
	AdvertiseAddress string `yaml:"advertise_address"`

	ApplyTimeout time.Duration `yaml:"apply_timeout" doc:"hidden"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "metastore."
	f.StringVar(&cfg.Address, prefix+"address", "localhost:9095", "")
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix+"grpc-client-config", f)
	f.StringVar(&cfg.DataDir, prefix+"data-dir", "./data-metastore/data", "")
	f.DurationVar(&cfg.MinReadyDuration, prefix+"min-ready-duration", 15*time.Second, "Minimum duration to wait after the internal readiness checks have passed but before succeeding the readiness endpoint. This is used to slowdown deployment controllers (eg. Kubernetes) after an instance is ready and before they proceed with a rolling update, to give the rest of the cluster instances enough time to receive some (DNS?) updates.")
	f.DurationVar(&cfg.DLQRecoveryPeriod, prefix+"dlq-recovery-period", 15*time.Second, "Period for DLQ recovery loop.")
	cfg.Raft.RegisterFlagsWithPrefix(prefix+"raft.", f)
	cfg.Compaction.RegisterFlagsWithPrefix(prefix+"compaction.", f)
	cfg.Index.RegisterFlagsWithPrefix(prefix+"index.", f)
}

func (cfg *Config) Validate() error {
	if cfg.Address == "" {
		return fmt.Errorf("metastore.address is required")
	}
	if err := cfg.GRPCClientConfig.Validate(); err != nil {
		return err
	}
	return cfg.Raft.Validate()
}

func (cfg *RaftConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.Dir, prefix+"dir", "./data-metastore/raft", "")
	f.Var((*flagext.StringSlice)(&cfg.BootstrapPeers), prefix+"bootstrap-peers", "")
	f.IntVar(&cfg.BootstrapExpectPeers, prefix+"bootstrap-expect-peers", 1, "Expected number of peers including the local node.")
	f.StringVar(&cfg.BindAddress, prefix+"bind-address", "localhost:9099", "")
	f.StringVar(&cfg.ServerID, prefix+"server-id", "localhost:9099", "")
	f.StringVar(&cfg.AdvertiseAddress, prefix+"advertise-address", "localhost:9099", "")
	f.DurationVar(&cfg.ApplyTimeout, prefix+"apply-timeout", 5*time.Second, "")
}

func (cfg *RaftConfig) Validate() error {
	// TODO(kolesnikovae): Check the params.
	return nil
}

type Metastore struct {
	service services.Service
	metastorev1.MetastoreServiceServer
	metastorev1.OperatorServiceServer
	compactorv1.CompactionPlannerServer

	config Config
	logger log.Logger
	reg    prometheus.Registerer

	// In-memory state.
	state *metastoreState

	// Persistent state.
	db *boltdb

	// Raft module.
	wal          *raftwal.WAL
	snapshots    *raft.FileSnapshotStore
	transport    *raft.NetworkTransport
	raft         *raft.Raft
	leaderhealth *raftleader.LeaderObserver

	logStore      raft.LogStore
	stableStore   raft.StableStore
	snapshotStore raft.SnapshotStore

	walDir string

	metrics *metastoreMetrics
	client  *metastoreclient.Client

	readyOnce  sync.Once
	readySince time.Time

	placementMgr *adaptiveplacement.Manager
	dnsProvider  *dns.Provider
	dlq          *dlq.Recovery
}

func New(
	config Config,
	logger log.Logger,
	reg prometheus.Registerer,
	client *metastoreclient.Client,
	bucket objstore.Bucket,
	placementMgr *adaptiveplacement.Manager,
) (*Metastore, error) {
	metrics := newMetastoreMetrics(reg)
	m := &Metastore{
		config:       config,
		logger:       logger,
		reg:          reg,
		db:           newDB(config, logger, metrics),
		metrics:      metrics,
		client:       client,
		placementMgr: placementMgr,
	}
	m.leaderhealth = raftleader.NewRaftLeaderHealthObserver(logger, reg)
	m.state = newMetastoreState(logger, m.db, reg, &config.Compaction, &config.Index)
	m.dlq = dlq.NewRecovery(dlq.RecoveryConfig{
		Period: config.DLQRecoveryPeriod,
	}, logger, m, bucket)
	m.service = services.NewBasicService(m.starting, m.running, m.stopping)
	return m, nil
}

func (m *Metastore) Service() services.Service { return m.service }

func (m *Metastore) Shutdown() error {
	m.dlq.Stop()
	m.shutdownRaft()
	m.db.shutdown()
	return nil
}

func (m *Metastore) starting(context.Context) error {
	if err := m.db.open(false); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	if err := m.initRaft(); err != nil {
		return fmt.Errorf("failed to initialize raft: %w", err)
	}
	return nil
}

func (m *Metastore) stopping(_ error) error {
	return m.Shutdown()
}

func (m *Metastore) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (m *Metastore) initRaft() (err error) {
	defer func() {
		if err != nil {
			// If the initialization fails, initialized components
			// should be de-initialized gracefully.
			m.shutdownRaft()
		}
	}()

	hasState, err := m.openRaftStore()
	if err != nil {
		return err
	}

	addr, err := net.ResolveTCPAddr("tcp", m.config.Raft.AdvertiseAddress)
	if err != nil {
		return err
	}
	m.transport, err = raft.NewTCPTransport(m.config.Raft.BindAddress, addr, transportConnPoolSize, transportTimeout, os.Stderr)
	if err != nil {
		return err
	}

	config := raft.DefaultConfig()
	// TODO: Wrap gokit
	//	config.Logger
	config.LogLevel = "debug"
	config.TrailingLogs = raftTrailingLogs
	config.SnapshotThreshold = raftSnapshotThreshold
	config.SnapshotInterval = raftSnapshotInterval
	config.LocalID = raft.ServerID(m.config.Raft.ServerID)

	fsm := newFSM(m.logger, m.db, m.state)
	m.raft, err = raft.NewRaft(config, fsm, m.logStore, m.stableStore, m.snapshotStore, m.transport)
	if err != nil {
		return fmt.Errorf("starting raft node: %w", err)
	}

	if !hasState {
		_ = level.Warn(m.logger).Log("msg", "no existing state found, trying to bootstrap cluster")
		if err = m.bootstrap(); err != nil {
			return fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	} else {
		_ = level.Info(m.logger).Log("msg", "restoring existing state, not bootstraping")
	}

	m.leaderhealth.Register(m.raft, func(st raft.RaftState) {
		if st == raft.Leader {
			m.dlq.Start()
			m.placementMgr.Start()
		} else {
			m.dlq.Stop()
			m.placementMgr.Stop()
		}
	})
	return nil
}

func (m *Metastore) openRaftStore() (hasState bool, err error) {
	if err = m.createRaftDirs(); err != nil {
		return false, err
	}
	m.wal, err = raftwal.Open(m.walDir)
	if err != nil {
		return false, fmt.Errorf("failed to open WAL: %w", err)
	}
	m.snapshots, err = raft.NewFileSnapshotStore(m.config.Raft.Dir, snapshotsRetain, os.Stderr)
	if err != nil {
		return false, fmt.Errorf("failed to open shapshot store: %w", err)
	}
	m.logStore = m.wal
	m.logStore, _ = raft.NewLogCache(walCacheEntries, m.logStore)
	m.stableStore = m.wal
	m.snapshotStore = m.snapshots
	if hasState, err = raft.HasExistingState(m.logStore, m.stableStore, m.snapshotStore); err != nil {
		return hasState, fmt.Errorf("failed to check for existing state: %w", err)
	}
	return hasState, nil
}

func (m *Metastore) createRaftDirs() (err error) {
	m.walDir = filepath.Join(m.config.Raft.Dir, "wal")
	if err = os.MkdirAll(m.walDir, 0755); err != nil {
		return fmt.Errorf("WAL dir: %w", err)
	}
	if err = os.MkdirAll(m.config.Raft.Dir, 0755); err != nil {
		return fmt.Errorf("snapshot directory: %w", err)
	}
	return nil
}

func (m *Metastore) shutdownRaft() {
	if m.raft != nil {
		// If raft has been initialized, try to transfer leadership.
		// Only after this we remove the leader health observer and
		// shutdown the raft.
		// There is a chance that client will still be trying to connect
		// to this instance, therefore retrying is still required.
		if err := m.raft.LeadershipTransfer().Error(); err != nil {
			switch {
			case errors.Is(err, raft.ErrNotLeader):
				// Not a leader, nothing to do.
			case strings.Contains(err.Error(), "cannot find peer"):
				// It's likely that there's just one node in the cluster.
			default:
				_ = level.Error(m.logger).Log("msg", "failed to transfer leadership", "err", err)
			}
		}
		m.leaderhealth.Deregister()
		if err := m.raft.Shutdown().Error(); err != nil {
			_ = level.Error(m.logger).Log("msg", "failed to shutdown raft", "err", err)
		}
	}
	if m.transport != nil {
		if err := m.transport.Close(); err != nil {
			_ = level.Error(m.logger).Log("msg", "failed to close transport", "err", err)
		}
	}
	if m.wal != nil {
		if err := m.wal.Close(); err != nil {
			_ = level.Error(m.logger).Log("msg", "failed to close WAL", "err", err)
		}
	}
}
