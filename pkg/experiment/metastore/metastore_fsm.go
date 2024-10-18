package metastore

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftleader"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
	"github.com/grafana/pyroscope/pkg/util"
)

// The map is used to determine the type of the given command,
// when the request is converted to a Raft log entry.
var commandTypeMap = map[reflect.Type]raftlogpb.CommandType{
	reflect.TypeOf(new(metastorev1.AddBlockRequest)):           raftlogpb.CommandType_COMMAND_TYPE_ADD_BLOCK,
	reflect.TypeOf(new(compactorv1.PollCompactionJobsRequest)): raftlogpb.CommandType_COMMAND_TYPE_POLL_COMPACTION_JOBS_STATUS,
	reflect.TypeOf(new(raftlogpb.CleanBlocksRequest)):          raftlogpb.CommandType_COMMAND_TYPE_CLEAN_BLOCKS,
}

// The map is used to determine the handler for the given command,
// read from the Raft log entry.
var commandHandlers = map[raftlogpb.CommandType]commandHandler{
	raftlogpb.CommandType_COMMAND_TYPE_ADD_BLOCK: func(fsm *FSM, cmd *raft.Log, raw []byte) fsmResponse {
		return handleCommand(raw, cmd, fsm.state.applyAddBlock)
	},
	raftlogpb.CommandType_COMMAND_TYPE_POLL_COMPACTION_JOBS_STATUS: func(fsm *FSM, cmd *raft.Log, raw []byte) fsmResponse {
		return handleCommand(raw, cmd, fsm.state.applyPollCompactionJobs)
	},
	raftlogpb.CommandType_COMMAND_TYPE_CLEAN_BLOCKS: func(fsm *FSM, cmd *raft.Log, raw []byte) fsmResponse {
		return handleCommand(raw, cmd, fsm.state.applyCleanBlocks)
	},
}

// TODO: Add registration functions.

type FSM struct {
	logger log.Logger
	state  *metastoreState
	db     *boltdb
}

type fsmResponse struct {
	msg proto.Message
	err error
}

type fsmError struct {
	log *raft.Log
	err error
}

type fatalCommandError struct {
	err error
}

func (e fatalCommandError) Error() string {
	return fmt.Sprintf("fatal FSM command error: %v", e.err)
}

func (e fatalCommandError) Unwrap() error { return e }

func errResponse(l *raft.Log, err error) fsmResponse {
	return fsmResponse{err: &fsmError{log: l, err: err}}
}

func (e *fsmError) Error() string {
	if e.err == nil {
		return ""
	}
	if e.log == nil {
		return e.err.Error()
	}
	return fmt.Sprintf("term: %d; index: %d; appended_at: %v; error: %v",
		e.log.Index, e.log.Term, e.log.AppendedAt, e.err)
}

type commandHandler func(*FSM, *raft.Log, []byte) fsmResponse

// TODO(kolesnikovae): replace commandCall with interface:
// type command[Req, Resp proto.Message] interface {
//   apply(Req) (Resp, error)
// }

type commandCall[Req, Resp proto.Message] func(*raft.Log, Req) (Resp, error)

func newFSM(logger log.Logger, db *boltdb, state *metastoreState) *FSM {
	return &FSM{
		logger: logger,
		db:     db,
		state:  state,
	}
}

// TODO(kolesnikovae): Implement BatchingFSM.

func (fsm *FSM) Apply(l *raft.Log) interface{} {
	switch l.Type {
	case raft.LogNoop:
	case raft.LogBarrier:
	case raft.LogConfiguration:
	case raft.LogCommand:
		return fsm.applyCommand(l)
	default:
		_ = level.Warn(fsm.logger).Log("msg", "unexpected log entry, ignoring", "type", l.Type.String())
	}
	return nil
}

// applyCommand receives raw command from the raft log (FSM.Apply),
// and calls the corresponding handler on the _local_ FSM, based on
// the command type.
func (fsm *FSM) applyCommand(l *raft.Log) interface{} {
	t1 := time.Now()
	defer func() {
		fsm.db.metrics.fsmApplyCommandHandlerDuration.Observe(time.Since(t1).Seconds())
	}()
	var e raftlogpb.RaftLogEntry
	if err := proto.Unmarshal(l.Data, &e); err != nil {
		return errResponse(l, err)
	}
	if handler, ok := commandHandlers[e.Type]; ok {
		return handler(fsm, l, e.Payload)
	}
	return errResponse(l, fmt.Errorf("unknown command type: %v", e.Type.String()))
}

// handleCommand receives payload of the command from the raft log (FSM.Apply),
// and the function that processes the command. Returned response is wrapped in
// fsmResponse and is available to the FSM.Apply caller.
func handleCommand[Req, Resp proto.Message](raw []byte, cmd *raft.Log, call commandCall[Req, Resp]) fsmResponse {
	var resp fsmResponse
	defer func() {
		if r := recover(); r != nil {
			var fCommandError fatalCommandError
			if errors.As(r.(error), &fCommandError) {
				panic(fCommandError)
			}
			resp.err = util.PanicError(r)
		}
	}()
	req := newProto[Req]()
	if resp.err = proto.Unmarshal(raw, req); resp.err != nil {
		return resp
	}
	resp.msg, resp.err = call(cmd, req)
	return resp
}

func newProto[T proto.Message]() T {
	var msg T
	msgType := reflect.TypeOf(msg).Elem()
	return reflect.New(msgType).Interface().(T)
}

func (fsm *FSM) Snapshot() (raft.FSMSnapshot, error) {
	// Snapshot should only capture a pointer to the state, and any
	// expensive IO should happen as part of FSMSnapshot.Persist.
	return fsm.db.createSnapshot()
}

func (fsm *FSM) Restore(snapshot io.ReadCloser) error {
	t1 := time.Now()
	_ = level.Info(fsm.logger).Log("msg", "restoring snapshot")
	defer func() {
		_ = snapshot.Close()
		fsm.db.metrics.fsmRestoreSnapshotDuration.Observe(time.Since(t1).Seconds())
	}()
	if err := fsm.db.restore(snapshot); err != nil {
		return fmt.Errorf("failed to restore from snapshot: %w", err)
	}
	if err := fsm.state.restore(fsm.db); err != nil {
		return fmt.Errorf("failed to restore state: %w", err)
	}
	return nil
}

// applyCommand issues the command to the raft log based on the request type,
// and returns the response of FSM.Apply.
func applyCommand[Req, Resp proto.Message](
	log *raft.Raft,
	req Req,
	timeout time.Duration,
) (
	future raft.ApplyFuture,
	resp Resp,
	err error,
) {
	defer func() {
		if r := recover(); r != nil {
			err = util.PanicError(r)
		}
	}()
	raw, err := marshallRequest(req)
	if err != nil {
		return nil, resp, err
	}
	future = log.Apply(raw, timeout)
	if err = future.Error(); err != nil {
		// todo (korniltsev) write a test to spawn multiple metastores and verify this error returned with correct details
		return nil, resp, wrapRetryableErrorWithRaftDetails(err, log)
	}
	fsmResp := future.Response().(fsmResponse)
	if fsmResp.msg != nil {
		resp, _ = fsmResp.msg.(Resp)
	}
	return future, resp, fsmResp.err
}

func marshallRequest[Req proto.Message](req Req) ([]byte, error) {
	cmdType, ok := commandTypeMap[reflect.TypeOf(req)]
	if !ok {
		return nil, fmt.Errorf("unknown command type: %T", req)
	}
	var err error
	entry := raftlogpb.RaftLogEntry{Type: cmdType}
	entry.Payload, err = proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(&entry)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func wrapRetryableErrorWithRaftDetails(err error, raft *raft.Raft) error {
	if err == nil || !shouldRetryCommand(err) {
		return err
	}
	_, serverID := raft.LeaderWithID()
	s := status.New(codes.Unavailable, err.Error())
	if serverID != "" {
		s, _ = s.WithDetails(&typesv1.RaftDetails{Leader: string(serverID)})
	}
	return s.Err()
}

func shouldRetryCommand(err error) bool {
	return raftleader.IsRaftLeadershipError(err)
}
