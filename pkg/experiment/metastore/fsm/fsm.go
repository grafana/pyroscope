package fsm

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

type RaftHandler[Req, Resp proto.Message] func(*bbolt.Tx, *raft.Log, Req) (Resp, error)

type StateRestorer interface {
	Restore(*bbolt.Tx) error
}

type FSM struct {
	logger  log.Logger
	metrics *metrics

	mu   sync.RWMutex
	txns sync.WaitGroup
	db   *boltdb

	handlers  map[RaftLogEntryType]handler
	restorers []StateRestorer
}

type handler func(tx *bbolt.Tx, cmd *raft.Log, raw []byte) (proto.Message, error)

func New(logger log.Logger, reg prometheus.Registerer, dir string) (*FSM, error) {
	fsm := FSM{
		logger:   logger,
		metrics:  newMetrics(reg),
		handlers: make(map[RaftLogEntryType]handler),
	}
	db := newDB(logger, fsm.metrics, dir)
	if err := db.open(false); err != nil {
		return nil, err
	}
	fsm.db = db
	return &fsm, nil
}

func (fsm *FSM) RegisterRestorer(r ...StateRestorer) {
	fsm.restorers = append(fsm.restorers, r...)
}

func RegisterRaftCommandHandler[Req, Resp proto.Message](fsm *FSM, t RaftLogEntryType, handler RaftHandler[Req, Resp]) {
	fsm.handlers[t] = func(tx *bbolt.Tx, cmd *raft.Log, raw []byte) (proto.Message, error) {
		var err error
		req := newProto[Req]()
		vt, ok := any(req).(interface{ UnmarshalVT([]byte) error })
		if ok {
			err = vt.UnmarshalVT(raw)
		} else {
			err = proto.Unmarshal(raw, req)
		}
		if err != nil {
			return nil, err
		}
		return handler(tx, cmd, req)
	}
}

func newProto[T proto.Message]() T {
	var msg T
	msgType := reflect.TypeOf(msg).Elem()
	return reflect.New(msgType).Interface().(T)
}

type fsmError struct {
	cmd *raft.Log
	err error
}

func errResponse(cmd *raft.Log, err error) Response {
	return Response{Err: &fsmError{cmd: cmd, err: err}}
}

func (e *fsmError) Error() string {
	if e.err == nil {
		return ""
	}
	if e.cmd == nil {
		return e.err.Error()
	}
	return fmt.Sprintf("term: %d; index: %d; appended_at: %v; error: %v",
		e.cmd.Index, e.cmd.Term, e.cmd.AppendedAt, e.err)
}

func (fsm *FSM) Apply(log *raft.Log) any {
	switch log.Type {
	case raft.LogNoop:
	case raft.LogBarrier:
	case raft.LogConfiguration:
		// TODO(kolesnikovae): applyConfiguration
	case raft.LogCommand:
		return fsm.applyCommand(log)
	default:
		_ = level.Warn(fsm.logger).Log("msg", "unexpected log entry, ignoring", "type", log.Type.String())
	}
	return nil
}

// applyCommand receives raw command from the raft log (FSM.Apply),
// and calls the corresponding handler on the _local_ FSM, based on
// the command type.
func (fsm *FSM) applyCommand(cmd *raft.Log) any {
	start := time.Now()
	var e RaftLogEntry
	if err := e.UnmarshalBinary(cmd.Data); err != nil {
		return errResponse(cmd, err)
	}

	cmdType := strconv.FormatUint(uint64(e.Type), 10)
	fsm.db.metrics.fsmApplyCommandSize.WithLabelValues(cmdType).Observe(float64(len(cmd.Data)))
	defer func() {
		fsm.db.metrics.fsmApplyCommandDuration.WithLabelValues(cmdType).Observe(time.Since(start).Seconds())
	}()
	handle, ok := fsm.handlers[e.Type]
	if !ok {
		return errResponse(cmd, fmt.Errorf("unknown command type: %d", e.Type))
	}

	// Apply is never called concurrently with Restore, so we don't need
	// to lock the FSM: db.boltdb is guaranteed to be in a consistent state.
	tx, err := fsm.db.boltdb.Begin(true)
	if err != nil {
		panic(fmt.Sprint("failed to begin write transaction:", err))
	}

	data, err := handle(tx, cmd, e.Data)
	if err != nil {
		_ = tx.Rollback()
		// TODO(kolesnikovae): This has to be a hard failure as we assume that
		//  the in-memory state might have not been rolled back properly.
		//  Handle more gracefully: handoff leadership, close the database, etc.
		panic(fmt.Sprint("failed to apply command:", err))
	}

	// We can't do anything about the failure at the database level,
	// so we panic here in a hope that other instances will handle
	// the command.
	if err = tx.Commit(); err != nil {
		panic(fmt.Sprint("failed to commit transaction:", err))
	}

	return Response{Data: data, Err: err}
}

func (fsm *FSM) Read(fn func(*bbolt.Tx)) error {
	fsm.mu.RLock()
	tx, err := fsm.db.boltdb.Begin(false)
	fsm.txns.Add(1)
	fsm.mu.RUnlock()
	if err != nil {
		fsm.txns.Done()
		return fmt.Errorf("failed to begin read transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
		fsm.txns.Done()
	}()
	fn(tx)
	return nil
}

func (fsm *FSM) Restore(snapshot io.ReadCloser) error {
	start := time.Now()
	_ = level.Info(fsm.logger).Log("msg", "restoring snapshot")
	defer func() {
		_ = snapshot.Close()
		fsm.db.metrics.fsmRestoreSnapshotDuration.Observe(time.Since(start).Seconds())
	}()
	// Block all new transactions until we restore the snapshot.
	// TODO(kolesnikovae): set not-serving service status to not
	//  block incoming requests.
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.txns.Wait()
	if err := fsm.db.restore(snapshot); err != nil {
		return fmt.Errorf("failed to restore from snapshot: %w", err)
	}
	tx, err := fsm.db.boltdb.Begin(false)
	if err != nil {
		return fmt.Errorf("failed to open transaction at restore: %w", err)
	}
	for _, r := range fsm.restorers {
		if err = r.Restore(tx); err != nil {
			return err
		}
	}
	// This is a read-only transaction.
	return tx.Rollback()
}

func (fsm *FSM) Snapshot() (raft.FSMSnapshot, error) {
	// Snapshot should only capture a pointer to the state, and any
	// expensive IO should happen as part of FSMSnapshot.Persist.
	s := snapshot{logger: fsm.logger, metrics: fsm.metrics}
	tx, err := fsm.db.boltdb.Begin(false)
	if err != nil {
		return nil, fmt.Errorf("failed to open a transaction for snapshot: %w", err)
	}
	s.tx = tx
	return &s, nil
}

func (fsm *FSM) Shutdown() {
	if fsm.db.boltdb != nil {
		fsm.db.shutdown()
	}
}
