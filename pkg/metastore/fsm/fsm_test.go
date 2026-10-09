package fsm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const testCommand RaftLogEntryType = 1000

var testBucket = []byte("test")

type testRegistry struct{}

func (testRegistry) Retrieve(string) (context.Context, bool) { return nil, false }
func (testRegistry) Delete(string)                           {}
func (testRegistry) Size() int                               { return 0 }

type testState struct{}

func (testState) Init(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(testBucket)
	return err
}

func (testState) Restore(*bbolt.Tx) error { return nil }

func (testState) put(_ context.Context, tx *bbolt.Tx, _ *raft.Log, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	return req, tx.Bucket(testBucket).Put([]byte("value"), []byte(req.Value))
}

type validatorFunc func(*bbolt.Tx) error

func (f validatorFunc) ValidateSnapshot(tx *bbolt.Tx) error { return f(tx) }

func newTestFSM(t *testing.T, validators ...SnapshotValidator) *FSM {
	t.Helper()
	f, err := New(log.NewNopLogger(), nil, Config{
		SnapshotCompression: "zstd",
		DataDir:             t.TempDir(),
	}, testRegistry{})
	require.NoError(t, err)
	t.Cleanup(f.Shutdown)
	RegisterRaftCommandHandler(f, testCommand, testState{}.put)
	f.RegisterRestorer(testState{})
	f.RegisterSnapshotValidator(validators...)
	require.NoError(t, f.Init())
	return f
}

func applyTestCommand(t *testing.T, f *FSM, index uint64, value string) {
	t.Helper()
	data, err := MarshalEntry(testCommand, wrapperspb.String(value))
	require.NoError(t, err)
	resp := f.Apply(&raft.Log{Type: raft.LogCommand, Index: index, Term: 1, Data: data})
	require.NoError(t, resp.(Response).Err)
}

func readTestValue(t *testing.T, f *FSM) string {
	t.Helper()
	var v string
	require.NoError(t, f.Read(func(tx *bbolt.Tx) {
		v = string(tx.Bucket(testBucket).Get([]byte("value")))
	}))
	return v
}

type memSink struct{ bytes.Buffer }

func (*memSink) ID() string    { return "test" }
func (*memSink) Cancel() error { return nil }
func (*memSink) Close() error  { return nil }

func snapshot(t *testing.T, f *FSM) []byte {
	t.Helper()
	s, err := f.Snapshot()
	require.NoError(t, err)
	defer s.Release()
	var sink memSink
	require.NoError(t, s.Persist(&sink))
	return sink.Bytes()
}

func TestFSM_UnknownCommandPanics(t *testing.T) {
	f := newTestFSM(t)
	data, err := MarshalEntry(testCommand+1, wrapperspb.String("x"))
	require.NoError(t, err)
	assert.PanicsWithValue(t,
		"failed to apply command: unknown command type 1001 at index 1; the binary is likely older than the one that proposed the command",
		func() { f.Apply(&raft.Log{Type: raft.LogCommand, Index: 1, Term: 1, Data: data}) },
	)
}

func TestFSM_UnknownCommandIsSkippedIfAlreadyApplied(t *testing.T) {
	f := newTestFSM(t)
	applyTestCommand(t, f, 5, "a")
	data, err := MarshalEntry(testCommand+1, wrapperspb.String("x"))
	require.NoError(t, err)
	assert.NotPanics(t, func() { f.Apply(&raft.Log{Type: raft.LogCommand, Index: 4, Term: 1, Data: data}) })
}

func TestFSM_RestoreValidatesSnapshot(t *testing.T) {
	source := newTestFSM(t)
	applyTestCommand(t, source, 1, "from-snapshot")
	snap := snapshot(t, source)

	errRejected := errors.New("rejected")
	var seen string
	target := newTestFSM(t, validatorFunc(func(tx *bbolt.Tx) error {
		seen = string(tx.Bucket(testBucket).Get([]byte("value")))
		return errRejected
	}))
	applyTestCommand(t, target, 1, "local")

	err := target.Restore(io.NopCloser(bytes.NewReader(snap)))
	require.ErrorIs(t, err, errRejected)
	assert.Equal(t, "from-snapshot", seen, "the validator reads the snapshot")
	assert.Equal(t, "local", readTestValue(t, target), "a rejected snapshot does not replace the state")
	applyTestCommand(t, target, 2, "still-writable")
	assert.Equal(t, "still-writable", readTestValue(t, target))

	accepting := newTestFSM(t, validatorFunc(func(*bbolt.Tx) error { return nil }))
	applyTestCommand(t, accepting, 1, "local")
	require.NoError(t, accepting.Restore(io.NopCloser(bytes.NewReader(snap))))
	assert.Equal(t, "from-snapshot", readTestValue(t, accepting))
}
