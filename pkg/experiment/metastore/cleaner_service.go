package metastore

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/markers"
)

type LocalCleaner interface {
	ExpectRequest(request string)
}

type CleanerService struct {
	config  markers.Config
	logger  log.Logger
	raftLog Raft
	local   LocalCleaner

	m       sync.Mutex
	started bool
	cancel  context.CancelFunc
}

func NewCleanerService(
	logger log.Logger,
	config markers.Config,
	raftLog Raft,
	local LocalCleaner,
) *CleanerService {
	return &CleanerService{
		config:  config,
		logger:  logger,
		raftLog: raftLog,
		local:   local,
	}
}

func (svc *CleanerService) Start() {
	svc.m.Lock()
	defer svc.m.Unlock()
	if svc.started {
		svc.logger.Log("msg", "cleaner already started")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc.cancel = cancel
	svc.started = true
	go svc.runLoop(ctx)
	svc.logger.Log("msg", "cleaner started")
}

func (svc *CleanerService) Stop() {
	svc.m.Lock()
	defer svc.m.Unlock()
	if !svc.started {
		svc.logger.Log("msg", "cleaner already stopped")
		return
	}
	svc.cancel()
	svc.started = false
	svc.logger.Log("msg", "cleaner stopped")
}

func (svc *CleanerService) runLoop(ctx context.Context) {
	t := time.NewTicker(svc.config.CompactedBlocksCleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			requestID := ulid.MustNew(ulid.Now(), rand.Reader).String()
			svc.local.ExpectRequest(requestID)
			req := &raft_log.CleanBlocksRequest{RequestId: requestID}
			_, err := svc.raftLog.Propose(fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_CLEAN_BLOCKS), req)
			if err != nil {
				level.Error(svc.logger).Log("msg", "failed to apply clean blocks command", "err", err)
			}
		}
	}
}
