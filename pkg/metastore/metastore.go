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

	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/services"
	"github.com/hashicorp/raft"
	raftwal "github.com/hashicorp/raft-wal"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/raftleader"
	"github.com/grafana/pyroscope/pkg/util/health"
)

const (
	snapshotsRetain       = 3
	walCacheEntries       = 512
	transportConnPoolSize = 10
	transportTimeout      = 10 * time.Second

	raftTrailingLogs      = 18 << 10
	raftSnapshotInterval  = 180 * time.Second
	raftSnapshotThreshold = 8 << 10

	metastoreRaftLeaderHealthServiceName = "metastore.v1.MetastoreService.RaftLeader"
)

type Config struct {
	DataDir string     `yaml:"data_dir"`
	Raft    RaftConfig `yaml:"raft"`
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
	f.StringVar(&cfg.DataDir, prefix+"data-dir", "./data-metastore/data", "")
	cfg.Raft.RegisterFlags(f)
}

func (cfg *RaftConfig) RegisterFlags(f *flag.FlagSet) {
	const prefix = "metastore.raft."
	f.StringVar(&cfg.Dir, prefix+"dir", "./data-metastore/raft", "")
	f.Var((*flagext.StringSlice)(&cfg.BootstrapPeers), prefix+"bootstrap-peers", "")
	f.IntVar(&cfg.BootstrapExpectPeers, prefix+"bootstrap-expect-peers", 1, "Expected number of peers including the local node.")
	f.StringVar(&cfg.BindAddress, prefix+"bind-address", "localhost:9099", "")
	f.StringVar(&cfg.ServerID, prefix+"server-id", "localhost:9099", "")
	f.StringVar(&cfg.AdvertiseAddress, prefix+"advertise-address", "localhost:9099", "")
	f.DurationVar(&cfg.ApplyTimeout, prefix+"apply-timeout", 5*time.Second, "")
}

type Metastore struct {
	service services.Service
	metastorev1.MetastoreServiceServer
	compactorv1.CompactionPlannerServer

	config Config
	logger log.Logger
	reg    prometheus.Registerer
	limits Limits

	// In-memory state.
	state *metastoreState

	// Persistent state.
	db *boltdb

	// Compaction
	compactionPlanner *Planner

	// Raft module.
	wal          *raftwal.WAL
	snapshots    *raft.FileSnapshotStore
	transport    *raft.NetworkTransport
	raft         *raft.Raft
	leaderhealth *raftleader.HealthObserver

	logStore      raft.LogStore
	stableStore   raft.StableStore
	snapshotStore raft.SnapshotStore

	walDir string

	done    chan struct{}
	wg      sync.WaitGroup
	metrics *metastoreMetrics
	client  *metastoreclient.Client
}

type Limits interface{}

func New(config Config, limits Limits, logger log.Logger, reg prometheus.Registerer, hs health.Service, client *metastoreclient.Client) (*Metastore, error) {
	metrics := newMetastoreMetrics(reg)
	m := &Metastore{
		config:  config,
		logger:  logger,
		reg:     reg,
		limits:  limits,
		db:      newDB(config, logger, metrics),
		done:    make(chan struct{}),
		metrics: metrics,
		client:  client,
	}
	m.leaderhealth = raftleader.NewRaftLeaderHealthObserver(hs, logger, raftleader.NewMetrics(reg))
	m.state = newMetastoreState(logger, m.db, m.reg)
	m.service = services.NewBasicService(m.starting, m.running, m.stopping)
	return m, nil
}

func (m *Metastore) Service() services.Service { return m.service }

func (m *Metastore) Shutdown() error {
	m.shutdownRaft()
	m.db.shutdown()
	return nil
}

func (m *Metastore) starting(ctx context.Context) error {
	if err := m.db.open(false); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	if err := m.initRaft(); err != nil {
		return fmt.Errorf("failed to initialize raft: %w", err)
	}
	m.compactionPlanner = NewPlanner(m.state, m.logger)
	m.wg.Add(1)
	go m.cleanupLoop()
	return nil
}

func (m *Metastore) stopping(_ error) error {
	close(m.done)
	m.wg.Wait()
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
	}

	m.leaderhealth.Register(m.raft, metastoreRaftLeaderHealthServiceName)
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
		m.leaderhealth.Deregister(m.raft, metastoreRaftLeaderHealthServiceName)
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