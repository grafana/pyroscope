package metastore

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
	"github.com/grafana/pyroscope/pkg/util"
)

// The map is used to determine the handler for the given command,
// read from the Raft log entry.
var commandHandlers = map[raftlogpb.CommandType]commandHandler{
	raftlogpb.CommandType_COMMAND_TYPE_ADD_BLOCK: func(fsm *FSM, cmd *raft.Log, raw []byte) fsmResponse {
		return handleCommand(raw, cmd, fsm.applyAddBlock)
	},
	raftlogpb.CommandType_COMMAND_TYPE_POLL_COMPACTION_JOBS: func(fsm *FSM, cmd *raft.Log, raw []byte) fsmResponse {
		return handleCommand(raw, cmd, fsm.applyPollCompactionJobs)
	},
	raftlogpb.CommandType_COMMAND_TYPE_CLEAN_BLOCKS: func(fsm *FSM, cmd *raft.Log, raw []byte) fsmResponse {
		return handleCommand(raw, cmd, fsm.applyCleanBlocks)
	},
}

type FSM struct {
	logger log.Logger
	db     *boltdb
	*metastoreState
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

type commandCall[Req, Resp proto.Message] func(*raft.Log, Req) (Resp, error)

func newFSM(logger log.Logger, state *metastoreState) *FSM {
	return &FSM{
		logger:         logger,
		metastoreState: state,
	}
}

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
			var f fatalCommandError
			if errors.As(r.(error), &f) {
				panic(f)
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
