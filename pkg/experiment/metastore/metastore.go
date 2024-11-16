package metastore

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	placement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/dlq"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/markers"
	raft "github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/util/health"
)

type Config struct {
	Address          string             `yaml:"address"`
	GRPCClientConfig grpcclient.Config  `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the metastore."`
	DataDir          string             `yaml:"data_dir"`
	Raft             raft.Config        `yaml:"raft"`
	Compaction       CompactionConfig   `yaml:"compaction_config" category:"advanced"`
	MinReadyDuration time.Duration      `yaml:"min_ready_duration" category:"advanced"`
	DLQRecovery      dlq.RecoveryConfig `yaml:"dlq_recovery" category:"advanced"`
	Index            index.Config       `yaml:"index_config" category:"advanced"`
	BlockCleaner     markers.Config     `yaml:"block_cleaner_config" category:"advanced"`
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

type Metastore struct {
	service services.Service

	config Config
	logger log.Logger
	reg    prometheus.Registerer
	health health.Service

	raft *raft.Node
	fsm  *fsm.FSM

	followerRead *raft.StateReader[*bbolt.Tx]

	bucket      objstore.Bucket
	placement   *placement.Manager
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
	metadataService *MetadataQueryService

	readyOnce  sync.Once
	readySince time.Time
}

func New(
	config Config,
	logger log.Logger,
	reg prometheus.Registerer,
	healthService health.Service,
	client raftnodepb.RaftNodeServiceClient,
	bucket objstore.Bucket,
	placementMgr *placement.Manager,
) (*Metastore, error) {
	m := &Metastore{
		config:    config,
		logger:    logger,
		reg:       reg,
		health:    healthService,
		bucket:    bucket,
		placement: placementMgr,
	}

	var err error
	reg = prometheus.WrapRegistererWithPrefix("pyroscope_metastore_", reg)
	m.fsm, err = fsm.New(m.logger, reg, m.config.DataDir)
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

	if m.raft, err = raft.NewNode(m.logger, m.config.Raft, reg, m.fsm); err != nil {
		return nil, fmt.Errorf("failed to initialize raft: %w", err)
	}

	// Create the read-only interface to the state.
	// We're currently only using the Follower Read pattern, assuming that
	// leader reads are done through the raft log. However, this should be
	// optimized in the future to use the Leader Read pattern.
	m.followerRead = m.newFollowerReader(client, m.raft, m.fsm)

	// Services should be registered after FSM and Raft have been initialized.
	// Services provide an interface to interact with the metastore.
	m.indexService = NewIndexService(m.logger, m.raft, m.placement)
	m.compactionService = NewCompactionService(m.logger, m.raft)
	m.cleanerService = NewCleanerService(m.logger, m.config.BlockCleaner, m.raft, m.cleanerHandler)
	m.tenantService = NewTenantService(m.logger, m.followerRead, m.index)
	m.metadataService = NewMetadataQueryService(m.logger, m.followerRead, m.index)
	m.dlqRecovery = dlq.NewRecovery(logger, config.DLQRecovery, m.indexService, bucket)

	// These are the services that only run on the raft leader.
	// Keep in mind that the node may not be the leader at the moment the
	// service is starting, so it should be able to handle conflicts.
	m.raft.RunOnLeader(m.dlqRecovery)
	m.raft.RunOnLeader(m.placement)
	m.raft.RunOnLeader(m.cleanerService)

	m.service = services.NewBasicService(m.starting, m.running, m.stopping)
	return m, nil
}

func (m *Metastore) Register(server *grpc.Server) {
	metastorev1.RegisterIndexServiceServer(server, m.indexService)
	metastorev1.RegisterCompactionServiceServer(server, m.compactionService)
	metastorev1.RegisterMetadataQueryServiceServer(server, m.metadataService)
	metastorev1.RegisterTenantServiceServer(server, m.tenantService)
	m.raft.Register(server)
}

func (m *Metastore) Service() services.Service { return m.service }

func (m *Metastore) starting(context.Context) error { return nil }

func (m *Metastore) stopping(_ error) error {
	m.cleanerService.Stop()
	m.dlqRecovery.Stop()

	// We let clients observe the leadership transfer: it's their
	// responsibility to connect to the new leader. We only need to
	// make sure that any error returned to clients includes details
	// about the raft leader, if applicable.
	if err := m.raft.TransferLeadership(); err == nil {
		// We were the leader and managed to transfer leadership – wait a bit
		// to let the new leader settle. During this period we're still serving
		// requests, but return an error with the new leader address.
		level.Info(m.logger).Log("msg", "waiting for leadership transfer to complete")
		time.Sleep(m.config.MinReadyDuration)
	}

	// Tell clients to stop sending requests to this node. There are no any
	// guarantees that clients will see or obey this. Normally, we would have
	// stopped the gRPC server here, but we can't: it's managed by the service
	// framework. Because of that we sleep another MinReadyDuration to let new
	// client to discover that the node is not serving anymore.
	m.health.SetNotServing()
	time.Sleep(m.config.MinReadyDuration)

	m.raft.Shutdown()
	m.fsm.Shutdown()
	return nil
}

func (m *Metastore) running(ctx context.Context) error {
	m.health.SetServing()
	<-ctx.Done()
	return nil
}

// CheckReady verifies if the metastore is ready to serve requests by
// ensuring the node is up-to-date with the leader's commit index.
func (m *Metastore) CheckReady(ctx context.Context) error {
	if _, err := m.followerRead.WaitLeaderCommitIndexApplied(ctx); err != nil {
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
