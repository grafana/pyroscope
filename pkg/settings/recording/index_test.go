package recording

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func upsertRule(t *testing.T, r *RecordingRules, tenant string) *settingsv1.RecordingRule {
	t.Helper()
	ctx := user.InjectOrgID(context.Background(), tenant)
	rule := RandomRule()
	rule.Id = ""
	resp, err := r.UpsertRecordingRule(ctx, connect.NewRequest(upsertReqFromRule(rule)))
	require.NoError(t, err)
	return resp.Msg.Rule
}

func TestBuildIndex(t *testing.T) {
	bucket := objstore.NewInMemBucket()
	r := New(bucket, log.NewNopLogger(), validation.MockOverrides(func(*validation.Limits, map[string]*validation.Limits) {}))

	upsertRule(t, r, "tenant-a")
	upsertRule(t, r, "tenant-b")
	upsertRule(t, r, "tenant-b")
	deleted := upsertRule(t, r, "tenant-c")
	_, err := r.DeleteRecordingRule(user.InjectOrgID(context.Background(), "tenant-c"), connect.NewRequest(&settingsv1.DeleteRecordingRuleRequest{Id: deleted.Id}))
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), "blocks/some-block/meta.json", bytes.NewReader([]byte("{}"))))
	require.NoError(t, bucket.Upload(context.Background(), "adaptive_placement/rules.binpb", bytes.NewReader(nil)))

	idx, err := BuildIndex(context.Background(), bucket, log.NewNopLogger())
	require.NoError(t, err)

	storedA, err := ReadStoredRules(context.Background(), bucket, log.NewNopLogger(), "tenant-a")
	require.NoError(t, err)
	storedB, err := ReadStoredRules(context.Background(), bucket, log.NewNopLogger(), "tenant-b")
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{
		"tenant-a": storedA.Generation,
		"tenant-b": storedB.Generation,
	}, idx.Tenants)
	assert.NotZero(t, idx.GeneratedAt)
}

type failingGetBucket struct {
	objstore.Bucket
	failPrefix string
}

func (b failingGetBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if len(name) >= len(b.failPrefix) && name[:len(b.failPrefix)] == b.failPrefix {
		return nil, errors.New("boom")
	}
	return b.Bucket.Get(ctx, name)
}

func TestBuildIndex_ReadFailureFailsTheBuild(t *testing.T) {
	inner := objstore.NewInMemBucket()
	r := New(inner, log.NewNopLogger(), validation.MockOverrides(func(*validation.Limits, map[string]*validation.Limits) {}))
	upsertRule(t, r, "tenant-a")
	upsertRule(t, r, "tenant-b")

	_, err := BuildIndex(context.Background(), failingGetBucket{Bucket: inner, failPrefix: "tenant-b/"}, log.NewNopLogger())
	require.ErrorContains(t, err, "tenant-b")
}

func TestIndexStore(t *testing.T) {
	bucket := objstore.NewInMemBucket()
	store := NewIndexStore(bucket)

	_, err := store.Load(context.Background())
	require.ErrorIs(t, err, ErrIndexNotFound)

	want := &Index{GeneratedAt: 42, Tenants: map[string]int64{"tenant-a": 3}}
	require.NoError(t, store.Store(context.Background(), want))
	got, err := store.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)

	require.NoError(t, bucket.Upload(context.Background(), IndexPath, bytes.NewReader([]byte("not json"))))
	_, err = store.Load(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrIndexNotFound)
}

func TestIndexBuilder_Run(t *testing.T) {
	bucket := objstore.NewInMemBucket()
	r := New(bucket, log.NewNopLogger(), validation.MockOverrides(func(*validation.Limits, map[string]*validation.Limits) {}))
	upsertRule(t, r, "tenant-a")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		NewIndexBuilder(bucket, log.NewNopLogger(), 10*time.Millisecond).Run(ctx)
	}()

	require.Eventually(t, func() bool {
		idx, err := NewIndexStore(bucket).Load(context.Background())
		return err == nil && len(idx.Tenants) == 1
	}, time.Second, 5*time.Millisecond)

	upsertRule(t, r, "tenant-b")
	require.Eventually(t, func() bool {
		idx, err := NewIndexStore(bucket).Load(context.Background())
		return err == nil && len(idx.Tenants) == 2
	}, time.Second, 5*time.Millisecond)

	cancel()
	<-done
}
