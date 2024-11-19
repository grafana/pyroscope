package fsm

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

// RaftHandler is a function that processes a Raft command.
// The implementation MUST be idempotent.
type RaftHandler[Req, Resp proto.Message] func(*bbolt.Tx, *raft.Log, Req) (Resp, error)

// StateRestorer is called during the FSM initialization
// to restore the state from a snapshot.
// The implementation MUST be idempotent.
type StateRestorer interface {
	// Init is provided with a write transaction to initialize the state.
	// FSM guarantees that Init is called synchronously and has exclusive
	// access to the database.
	Init(*bbolt.Tx) error
	// Restore is provided with a read transaction to restore the state.
	// Restore might be called concurrently with other StateRestorer
	// instances.
	Restore(*bbolt.Tx) error
}

// FSM implements the raft.FSM interface.
type FSM struct {
	logger  log.Logger
	metrics *metrics

	mu   sync.RWMutex
	txns sync.WaitGroup
	db   *boltdb

	handlers  map[RaftLogEntryType]handler
	restorers []StateRestorer

	appliedTerm  uint64
	appliedIndex uint64
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
		req, err := unmarshal[Req](raw)
		if err != nil {
			return nil, err
		}
		return handler(tx, cmd, req)
	}
}

// Init must be called after the FSM is created and all restorers are registered.
func (fsm *FSM) Init() error {
	if err := fsm.init(); err != nil {
		return fmt.Errorf("failed to initialize state: %w", err)
	}
	if err := fsm.restore(); err != nil {
		return fmt.Errorf("failed to restore state: %w", err)
	}
	return nil
}

func (fsm *FSM) init() (err error) {
	tx, err := fsm.db.boltdb.Begin(true)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}()
	if err = fsm.initRaftBucket(tx); err != nil {
		return fmt.Errorf("failed to init raft bucket: %w", err)
	}
	for _, r := range fsm.restorers {
		if err = r.Init(tx); err != nil {
			return err
		}
	}
	return nil
}

func (fsm *FSM) restore() error {
	if err := fsm.db.boltdb.View(fsm.loadAppliedIndex); err != nil {
		return fmt.Errorf("failed to load applied index: %w", err)
	}
	level.Info(fsm.logger).Log("msg", "restoring state", "term", fsm.appliedTerm, "applied_index", fsm.appliedIndex)
	g, _ := errgroup.WithContext(context.Background())
	for _, r := range fsm.restorers {
		g.Go(func() error {
			return fsm.db.boltdb.View(r.Restore)
		})
	}
	return g.Wait()
}

// Restore restores the FSM state from a snapshot.
func (fsm *FSM) Restore(snapshot io.ReadCloser) (err error) {
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
	if err = fsm.db.restore(snapshot); err != nil {
		return fmt.Errorf("failed to restore database from snapshot: %w", err)
	}
	// First we need to initialize the state: each restorer is called
	// synchronously and has exclusive access to the database.
	if err = fsm.init(); err != nil {
		return fmt.Errorf("failed to init state at restore: %w", err)
	}
	// Then we restore the state: each restorer is given its own
	// transaction and run concurrently with others.
	if err = fsm.restore(); err != nil {
		return fmt.Errorf("failed to restore state from snapshot: %w", err)
	}
	return nil
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
	case raft.LogCommand:
		return fsm.applyCommand(log)
	default:
		level.Warn(fsm.logger).Log("msg", "unexpected log entry, ignoring", "type", log.Type.String())
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
	if cmd.Index <= fsm.appliedIndex {
		// Skip already applied commands at WAL restore.
		// Note that the 0 index is a noop and is never applied to FSM.
		return Response{}
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
		// NOTE(kolesnikovae): This has to be a hard failure as we assume
		// that the in-memory state might have not been rolled back properly.
		panic(fmt.Sprint("failed to apply command:", err))
	}

	if err = fsm.storeAppliedIndex(tx, cmd.Term, cmd.Index); err != nil {
		panic(fmt.Sprint("failed to store applied index: %w", err))
	}

	// We can't do anything about the failure at the database level, so we
	// panic here in a hope that other instances will handle the command.
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

var (
	raftBucketName  = []byte("raft")
	appliedIndexKey = []byte("term.applied_index")
	// Value is encoded as [8]term + [8]index.
)

func (fsm *FSM) initRaftBucket(tx *bbolt.Tx) error {
	b := tx.Bucket(raftBucketName)
	if b != nil {
		return nil
	}
	// If no bucket exists, we create a stub with 0 values.
	if _, err := tx.CreateBucket(raftBucketName); err != nil {
		return err
	}
	return fsm.storeAppliedIndex(tx, 0, 0)
}

func (fsm *FSM) storeAppliedIndex(tx *bbolt.Tx, term, index uint64) error {
	b := tx.Bucket(raftBucketName)
	if b == nil {
		return bbolt.ErrBucketNotFound
	}
	v := make([]byte, 16)
	binary.BigEndian.PutUint64(v[0:8], term)
	binary.BigEndian.PutUint64(v[8:16], index)
	fsm.appliedTerm = term
	fsm.appliedIndex = index
	return b.Put(appliedIndexKey, v)
}

var errAppliedIndexInvalid = fmt.Errorf("invalid applied index")

func (fsm *FSM) loadAppliedIndex(tx *bbolt.Tx) error {
	b := tx.Bucket(raftBucketName)
	if b == nil {
		return bbolt.ErrBucketNotFound
	}
	v := b.Get(appliedIndexKey)
	if len(v) < 16 {
		return errAppliedIndexInvalid
	}
	fsm.appliedTerm = binary.BigEndian.Uint64(v[0:8])
	fsm.appliedIndex = binary.BigEndian.Uint64(v[8:16])
	return nil
}
