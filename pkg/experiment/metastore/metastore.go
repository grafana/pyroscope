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
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/dlq"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/markers"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raft_node"
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
)

type Config struct {
	Address          string             `yaml:"address"`
	GRPCClientConfig grpcclient.Config  `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the metastore."`
	DataDir          string             `yaml:"data_dir"`
	Raft             RaftConfig         `yaml:"raft"`
	Compaction       CompactionConfig   `yaml:"compaction_config" category:"advanced"`
	MinReadyDuration time.Duration      `yaml:"min_ready_duration" category:"advanced"`
	DLQRecovery      dlq.RecoveryConfig `yaml:"dlq_recovery" category:"advanced"`
	Index            index.Config       `yaml:"index_config" category:"advanced"`
	BlockCleaner     markers.Config     `yaml:"block_cleaner_config" category:"advanced"`
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
	f.StringVar(&cfg.DataDir, prefix+"data-dir", "./data-metastore/data", "")
	f.DurationVar(&cfg.MinReadyDuration, prefix+"min-ready-duration", 15*time.Second, "Minimum duration to wait after the internal readiness checks have passed but before succeeding the readiness endpoint. This is used to slowdown deployment controllers (eg. Kubernetes) after an instance is ready and before they proceed with a rolling update, to give the rest of the cluster instances enough time to receive some (DNS?) updates.")
	cfg.Raft.RegisterFlagsWithPrefix(prefix+"raft.", f)
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix+"grpc-client-config", f)
	cfg.Compaction.RegisterFlagsWithPrefix(prefix+"compaction.", f)
	cfg.Index.RegisterFlagsWithPrefix(prefix+"index.", f)
	cfg.BlockCleaner.RegisterFlagsWithPrefix(prefix+"block-cleaner.", f)
	cfg.DLQRecovery.RegisterFlagsWithPrefix(prefix+"dlq-recovery.", f)
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

	config Config
	logger log.Logger
	reg    prometheus.Registerer
	health health.Service

	// Raft module.
	walDir        string
	wal           *raftwal.WAL
	snapshots     *raft.FileSnapshotStore
	transport     *raft.NetworkTransport
	raft          *raft.Raft
	logStore      raft.LogStore
	stableStore   raft.StableStore
	snapshotStore raft.SnapshotStore

	// Local state machine.
	fsm *fsm.FSM
	// An interface to make proposals.
	proposer *RaftProposer
	client   metastorev1.RaftNodeServiceClient
	observer *raft_node.Observer
	follower *raft_node.Follower
	leader   *raft_node.Leader

	bucket      objstore.Bucket
	placement   *adaptiveplacement.Manager
	dnsProvider *dns.Provider
	dlqRecovery *dlq.Recovery

	index        *index.Index
	markers      *markers.DeletionMarkers
	indexService *IndexService
	indexHandler *IndexCommandHandler

	compactionService *CompactionService
	compactionHandler *CompactionCommandHandler

	cleanerService *CleanerService
	cleanerHandler *CleanerCommandHandler

	tenantService   *TenantService
	raftNodeService *RaftNodeService
	operatorService *OperatorService
	metadataService *MetadataQueryService

	readyOnce  sync.Once
	readySince time.Time
}

func New(
	config Config,
	logger log.Logger,
	reg prometheus.Registerer,
	healthService health.Service,
	client metastorev1.RaftNodeServiceClient,
	bucket objstore.Bucket,
	placementMgr *adaptiveplacement.Manager,
) (*Metastore, error) {
	m := &Metastore{
		config:    config,
		logger:    logger,
		reg:       reg,
		health:    healthService,
		bucket:    bucket,
		placement: placementMgr,
		client:    client,
	}

	var err error
	m.fsm, err = fsm.New(m.logger, m.reg, m.config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	// Initialization of the base components.
	indexStore := index.NewIndexStore(m.logger)
	m.index = index.NewIndex(m.logger, indexStore, &config.Index)
	m.markers = markers.NewDeletionMarkers(m.logger, &config.BlockCleaner, m.reg)

	// FSM handlers that utilize the components.
	m.compactionHandler = NewCompactionCommandHandler(m.logger, m.config.Compaction, m.index, m.markers, m.reg)
	m.indexHandler = NewIndexCommandHandler(m.logger, m.index, m.markers, m.compactionHandler)
	m.cleanerHandler = NewCleanerCommandHandler(m.logger, m.bucket, m.markers, m.reg)

	m.fsm.RegisterRestorer(m.index)
	m.fsm.RegisterRestorer(m.compactionHandler)
	m.fsm.RegisterRestorer(m.markers)

	fsm.RegisterRaftCommandHandler(m.fsm,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_POLL_COMPACTION_JOBS),
		m.compactionHandler.PollCompactionJobs)

	fsm.RegisterRaftCommandHandler(m.fsm,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_ADD_BLOCK),
		m.indexHandler.AddBlock)

	m.fsm.RegisterRestorer(m.markers)
	fsm.RegisterRaftCommandHandler(m.fsm,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_CLEAN_BLOCKS),
		m.cleanerHandler.CleanBlocks)

	if err = m.fsm.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize internal state: %w", err)
	}
	if err = m.initRaft(); err != nil {
		return nil, fmt.Errorf("failed to initialize raft: %w", err)
	}

	// Services should be registered after FSM and Raft have been initialized.
	// Services provide an interface to interact with the metastore.
	m.indexService = NewIndexService(m.logger, m.proposer, m.placement)
	m.compactionService = NewCompactionService(m.logger, m.proposer)
	m.cleanerService = NewCleanerService(m.logger, m.config.BlockCleaner, m.proposer, m.cleanerHandler)
	m.tenantService = NewTenantService(m.logger, m.follower, m.index)
	m.metadataService = NewMetadataQueryService(m.logger, m.follower, m.index)
	m.operatorService = NewOperatorService(m.config, m.raft)
	m.raftNodeService = NewRaftNodeService(m.leader)
	m.dlqRecovery = dlq.NewRecovery(logger, config.DLQRecovery, m.indexService, bucket)

	// These are the services that only run on the raft leader.
	// Keep in mind that the node may not be the leader at the moment the
	// service is starting, so it should be able to handle conflicts.
	m.observer.OnLeader(m.dlqRecovery)
	m.observer.OnLeader(m.placement)

	m.service = services.NewBasicService(m.starting, m.running, m.stopping)
	return m, nil
}

func (m *Metastore) Register(server *grpc.Server) {
	metastorev1.RegisterIndexServiceServer(server, m.indexService)
	metastorev1.RegisterCompactionServiceServer(server, m.compactionService)
	metastorev1.RegisterMetadataQueryServiceServer(server, m.metadataService)
	metastorev1.RegisterTenantServiceServer(server, m.tenantService)
	metastorev1.RegisterRaftNodeServiceServer(server, m.raftNodeService)
}

func (m *Metastore) Service() services.Service { return m.service }

func (m *Metastore) starting(context.Context) error { return nil }

func (m *Metastore) stopping(_ error) error {
	m.cleanerService.Stop()
	m.dlqRecovery.Stop()
	m.shutdownRaft()
	m.fsm.Shutdown()
	return nil
}

func (m *Metastore) running(ctx context.Context) error {
	m.health.SetServing()
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

	m.raft, err = raft.NewRaft(config, m.fsm, m.logStore, m.stableStore, m.snapshotStore, m.transport)
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

	m.proposer = NewRaftProposer(m.logger, m.raft, m.config.Raft.ApplyTimeout)
	m.observer = raft_node.NewRaftStateObserver(m.logger, m.raft, m.reg)
	m.follower = raft_node.NewFollower(m.client, m.raft, m.fsm)
	m.leader = raft_node.NewLeader(m.raft)
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
		// Tell clients to stop sending requests to this node.
		// There are no any guarantees that clients will see or obey this.
		m.health.SetNotServing()
		// We let clients observe the leadership transfer: it's their
		// responsibility to connect to the new leader. We only need to
		// make sure that any error returned to clients includes details
		// about the raft leader, if applicable.
		if err := m.TransferLeadership(); err == nil {
			// We were the leader and managed to transfer leadership.
			// Wait a bit to let the new leader settle.
			_ = level.Info(m.logger).Log("msg", "waiting for leadership transfer to complete")
			// TODO(kolesnikovae): Wait until ReadIndex of
			//  the new leader catches up the local CommitIndex.
			time.Sleep(m.config.MinReadyDuration)
		}
		m.observer.Deregister()
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

func (m *Metastore) TransferLeadership() (err error) {
	switch err = m.raft.LeadershipTransfer().Error(); {
	case err == nil:
	case errors.Is(err, raft.ErrNotLeader):
		// Not a leader, nothing to do.
	case strings.Contains(err.Error(), "cannot find peer"):
		// No peers, nothing to do.
	default:
		_ = level.Error(m.logger).Log("msg", "failed to transfer leadership", "err", err)
	}
	return err
}

// CheckReady verifies if the metastore is ready to serve requests by
// ensuring the node is up-to-date with the leader's commit index.
func (m *Metastore) CheckReady(ctx context.Context) error {
	if _, err := m.follower.WaitLeaderCommitIndexAppliedLocally(ctx); err != nil {
		return err
	}
	m.readyOnce.Do(func() {
		m.readySince = time.Now()
	})
	if w := m.config.MinReadyDuration - time.Since(m.readySince); w > 0 {
		return fmt.Errorf("%v before reporting readiness", w)
	}
	return nil
}
