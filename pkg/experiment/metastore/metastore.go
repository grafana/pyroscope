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
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/dlq"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	raft "github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/tombstones"
	"github.com/grafana/pyroscope/pkg/util/health"
)

type Config struct {
	Address          string             `yaml:"address"`
	GRPCClientConfig grpcclient.Config  `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the metastore."`
	DataDir          string             `yaml:"data_dir"`
	MinReadyDuration time.Duration      `yaml:"min_ready_duration" category:"advanced"`
	Raft             raft.Config        `yaml:"raft"`
	Index            index.Config       `yaml:",inline" category:"advanced"`
	DLQRecovery      dlq.RecoveryConfig `yaml:",inline" category:"advanced"`
	Compactor        compactor.Config   `yaml:",inline" category:"advanced"`
	Scheduler        scheduler.Config   `yaml:",inline" category:"advanced"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "metastore."
	f.StringVar(&cfg.Address, prefix+"address", "localhost:9095", "")
	f.StringVar(&cfg.DataDir, prefix+"data-dir", "./data-metastore/data", "")
	f.DurationVar(&cfg.MinReadyDuration, prefix+"min-ready-duration", 15*time.Second, "Minimum duration to wait after the internal readiness checks have passed but before succeeding the readiness endpoint. This is used to slowdown deployment controllers (eg. Kubernetes) after an instance is ready and before they proceed with a rolling update, to give the rest of the cluster instances enough time to receive some (DNS?) updates.")
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix+"grpc-client-config", f)
	cfg.Raft.RegisterFlagsWithPrefix(prefix+"raft.", f)
	cfg.Compactor.RegisterFlagsWithPrefix(prefix, f)
	cfg.Scheduler.RegisterFlagsWithPrefix(prefix, f)
	cfg.Index.RegisterFlagsWithPrefix(prefix, f)
	cfg.DLQRecovery.RegisterFlagsWithPrefix(prefix, f)
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

	bucket      objstore.Bucket
	placement   *placement.Manager
	dlqRecovery *dlq.Recovery

	index        *index.Index
	indexHandler *IndexCommandHandler
	indexService *IndexService

	tombstones        *tombstones.Tombstones
	compactor         *compactor.Compactor
	scheduler         *scheduler.Scheduler
	compactionHandler *CompactionCommandHandler
	compactionService *CompactionService

	followerRead    *raft.StateReader[*bbolt.Tx]
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

	m.fsm, err = fsm.New(m.logger, m.reg, m.config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	// Initialization of the base components.
	m.index = index.NewIndex(m.logger, index.NewStore(), &config.Index)
	m.tombstones = tombstones.NewTombstones(tombstones.NewStore())
	m.compactor = compactor.NewCompactor(config.Compactor, compactor.NewStore(), m.tombstones, m.reg)
	m.scheduler = scheduler.NewScheduler(config.Scheduler, scheduler.NewStore(), m.reg)

	// FSM handlers that utilize the components.
	m.indexHandler = NewIndexCommandHandler(m.logger, m.index, m.tombstones, m.compactor)
	fsm.RegisterRaftCommandHandler(m.fsm,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_ADD_BLOCK_METADATA),
		m.indexHandler.AddBlock)

	m.compactionHandler = NewCompactionCommandHandler(m.logger, m.index, m.compactor, m.compactor, m.scheduler, m.tombstones)
	fsm.RegisterRaftCommandHandler(m.fsm,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_GET_COMPACTION_PLAN_UPDATE),
		m.compactionHandler.GetCompactionPlanUpdate)
	fsm.RegisterRaftCommandHandler(m.fsm,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN),
		m.compactionHandler.UpdateCompactionPlan)

	m.fsm.RegisterRestorer(m.tombstones)
	m.fsm.RegisterRestorer(m.compactor)
	m.fsm.RegisterRestorer(m.scheduler)
	m.fsm.RegisterRestorer(m.index)

	// We are ready to start raft as our FSM is fully configured.
	if err = m.buildRaftNode(); err != nil {
		return nil, err
	}

	// Create the read-only interface to the state.
	// We're currently only using the Follower Read pattern, assuming that
	// leader reads are done through the raft log. However, this should be
	// optimized in the future to use the Leader Read pattern.
	m.followerRead = m.newFollowerReader(client, m.raft, m.fsm)

	// Services should be registered after FSM and Raft have been initialized.
	// Services provide an interface to interact with the metastore.
	m.compactionService = NewCompactionService(m.logger, m.raft)
	m.indexService = NewIndexService(m.logger, m.raft, m.followerRead, m.index, m.placement)
	m.tenantService = NewTenantService(m.logger, m.followerRead, m.index)
	m.metadataService = NewMetadataQueryService(m.logger, m.followerRead, m.index)
	m.dlqRecovery = dlq.NewRecovery(logger, config.DLQRecovery, m.indexService, bucket)

	// These are the services that only run on the raft leader.
	// Keep in mind that the node may not be the leader at the moment the
	// service is starting, so it should be able to handle conflicts.
	m.raft.RunOnLeader(m.dlqRecovery)
	m.raft.RunOnLeader(m.placement)

	m.service = services.NewBasicService(m.starting, m.running, m.stopping)
	return m, nil
}

func (m *Metastore) buildRaftNode() (err error) {
	// Raft is configured to always restore the state from the latest snapshot
	// (via FSM.Restore), if it is present. Otherwise, when no snapshots
	// available, the state must be initialized explicitly via FSM.Init before
	// we call raft.Init, which starts applying the raft log.
	if m.raft, err = raft.NewNode(m.logger, m.config.Raft, m.reg, m.fsm); err != nil {
		return fmt.Errorf("failed to create raft node: %w", err)
	}

	// Newly created raft node is not yet initialized and does not alter our
	// FSM in any way. However, it gives us access to the snapshot store, and
	// we can check whether we need to initialize the state (expensive), or we
	// can defer to raft snapshots. This is an optimization: we want to avoid
	// restoring the state twice: once at Init, and then at Restore.
	snapshots, err := m.raft.ListSnapshots()
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to list snapshots", "err", err)
		// We continue trying; in the worst case we will initialize the state
		// and then restore a snapshot received from the leader.
	}

	if len(snapshots) == 0 {
		level.Info(m.logger).Log("msg", "no state snapshots found")
		// FSM won't be restored by raft, so we need to initialize it manually.
		// Otherwise, raft will restore the state from a snapshot using
		// fsm.Restore, which will initialize the state as well.
		if err = m.fsm.Init(); err != nil {
			level.Error(m.logger).Log("msg", "failed to initialize state", "err", err)
			return err
		}
	} else {
		level.Info(m.logger).Log("msg", "skipping state initialization as snapshots found")
	}

	if err = m.raft.Init(); err != nil {
		return fmt.Errorf("failed to initialize raft: %w", err)
	}

	return nil
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
