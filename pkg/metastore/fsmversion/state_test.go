package fsmversion

import (
	"context"
	"testing"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

func newTestState(t *testing.T, supported Version, stored Version) (*State, *bbolt.DB) {
	t.Helper()
	db := test.BoltDB(t)
	t.Cleanup(func() { _ = db.Close() })
	s := NewState(log.NewNopLogger(), supported, nil)
	require.NoError(t, db.Update(s.Init))
	if stored > Unversioned {
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error { return put(tx, stored) }))
	}
	return s, db
}

func readVersion(t *testing.T, db *bbolt.DB) Version {
	t.Helper()
	var v Version
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		v = Get(tx)
		return nil
	}))
	return v
}

func TestState_SetVersion(t *testing.T) {
	tests := []struct {
		name      string
		supported Version
		stored    Version
		cmdTerm   uint64
		req       *raft_log.SetFSMVersionRequest
		want      Version
		wantErr   error
	}{
		{
			name:      "activates the first version",
			supported: 2,
			cmdTerm:   3,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 1},
			want:      1,
		},
		{
			name:      "activates a newer version",
			supported: 3,
			stored:    1,
			cmdTerm:   3,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 3},
			want:      3,
		},
		{
			name:      "ignores a proposal from a previous term",
			supported: 3,
			stored:    1,
			cmdTerm:   4,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 2},
			want:      1,
		},
		{
			name:      "ignores a stale term even if the version is unsupported",
			supported: 1,
			stored:    1,
			cmdTerm:   4,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 5},
			want:      1,
		},
		{
			name:      "never downgrades",
			supported: 3,
			stored:    2,
			cmdTerm:   3,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 1},
			want:      2,
		},
		{
			name:      "is idempotent",
			supported: 3,
			stored:    2,
			cmdTerm:   3,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 2},
			want:      2,
		},
		{
			name:      "fails on a version the binary does not support",
			supported: 1,
			stored:    1,
			cmdTerm:   3,
			req:       &raft_log.SetFSMVersionRequest{Term: 3, Version: 2},
			want:      1,
			wantErr:   ErrUnsupportedVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, db := newTestState(t, tt.supported, tt.stored)
			require.NoError(t, db.View(s.Restore))

			var resp *raft_log.SetFSMVersionResponse
			err := db.Update(func(tx *bbolt.Tx) error {
				var applyErr error
				resp, applyErr = s.SetVersion(context.Background(), tx, &raft.Log{Index: 10, Term: tt.cmdTerm}, tt.req)
				return applyErr
			})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, uint32(tt.want), resp.Version)
			}
			assert.Equal(t, tt.want, readVersion(t, db))
			assert.Equal(t, tt.want, s.Active())
		})
	}
}

func TestState_Restore(t *testing.T) {
	t.Run("loads the active version", func(t *testing.T) {
		s, db := newTestState(t, 3, 2)
		require.NoError(t, db.View(s.Restore))
		assert.Equal(t, Version(2), s.Active())
	})

	t.Run("treats a state without the version as unversioned", func(t *testing.T) {
		db := test.BoltDB(t)
		t.Cleanup(func() { _ = db.Close() })
		s := NewState(log.NewNopLogger(), 1, nil)
		require.NoError(t, db.View(s.Restore))
		assert.Equal(t, Unversioned, s.Active())
	})

	t.Run("refuses a state with an unsupported version", func(t *testing.T) {
		s, db := newTestState(t, 3, 4)
		require.ErrorIs(t, db.View(s.Restore), ErrUnsupportedVersion)
		assert.Equal(t, Unversioned, s.Active())
	})
}

func TestState_ValidateSnapshot(t *testing.T) {
	s, db := newTestState(t, 2, 2)
	require.NoError(t, db.View(s.ValidateSnapshot))

	older, olderDB := newTestState(t, 2, 1)
	require.NoError(t, olderDB.View(older.ValidateSnapshot))

	newer, newerDB := newTestState(t, 1, 2)
	require.ErrorIs(t, newerDB.View(newer.ValidateSnapshot), ErrUnsupportedVersion)
}

func TestIsActive(t *testing.T) {
	_, db := newTestState(t, 3, 2)
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		assert.True(t, IsActive(tx, Unversioned))
		assert.True(t, IsActive(tx, 1))
		assert.True(t, IsActive(tx, 2))
		assert.False(t, IsActive(tx, 3))
		return nil
	}))
}

func TestState_Metrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	db := test.BoltDB(t)
	t.Cleanup(func() { _ = db.Close() })
	s := NewState(log.NewNopLogger(), 3, reg)
	require.NoError(t, db.Update(s.Init))
	require.NoError(t, db.View(s.Restore))
	assert.Equal(t, float64(3), testutil.ToFloat64(s.metrics.supported))
	assert.Equal(t, float64(0), testutil.ToFloat64(s.metrics.active))

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		_, err := s.SetVersion(context.Background(), tx, &raft.Log{Index: 1, Term: 1}, &raft_log.SetFSMVersionRequest{Term: 1, Version: 2})
		return err
	}))
	assert.Equal(t, float64(2), testutil.ToFloat64(s.metrics.active))
}

func TestConfig_Supported(t *testing.T) {
	var c Config
	assert.Equal(t, Latest, c.Supported())
	v := Unversioned
	c.TestingSupportedVersion = &v
	assert.Equal(t, Unversioned, c.Supported())
}
