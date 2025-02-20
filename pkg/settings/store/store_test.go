package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

type testObj struct {
	Name       string
	Data       string
	Generation int64
}

type testObjHelper struct{}

func (_ *testObjHelper) ID(o *testObj) string {
	return o.Name
}

func (_ *testObjHelper) GetGeneration(o *testObj) int64 {
	return o.Generation
}

func (_ *testObjHelper) SetGeneration(o *testObj, gen int64) {
	o.Generation = gen
}

func (_ *testObjHelper) FromStore(data json.RawMessage) (*testObj, error) {
	var obj testObj
	err := json.Unmarshal(data, &obj)
	return &obj, err
}

func (_ *testObjHelper) ToStore(obj *testObj) (json.RawMessage, error) {
	return json.Marshal(obj)
}

func (_ *testObjHelper) TypePath() string {
	return "testobj.v1"
}

type testStore struct {
	*GenericStore[*testObj, *testObjHelper]
	bucketPath string
}

func newTestStore(t testing.TB) *testStore {
	logger := log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	bucketPath := t.TempDir()
	bucket, err := filesystem.NewBucket(bucketPath)
	require.NoError(t, err)
	return &testStore{
		GenericStore: New(
			logger,
			bucket,
			Key{TenantID: "user-a"},
			&testObjHelper{},
		),
		bucketPath: bucketPath,
	}
}

func Test_GenericStore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{}, result.Elements)
	})

	t.Run("one element", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a"}, nil))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a", Generation: 1},
		}, result.Elements)
	})

	t.Run("second element", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "b", Data: "data-b"}, nil))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a", Generation: 1},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("update without generation", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v2"}, nil))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a-v2", Generation: 2},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("update with generation", func(t *testing.T) {
		observedGeneration := int64(2)
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v3"}, &observedGeneration))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a-v3", Generation: 3},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("update with wrong generation", func(t *testing.T) {
		observedGeneration := int64(2)
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v4"}, &observedGeneration))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{}, result.Elements)
	})

	t.Run("delete element that exists", func(t *testing.T) {
		require.NoError(t, s.Delete(ctx, "a"))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})
	t.Run("delete element that doesnt exist", func(t *testing.T) {
		require.NoError(t, s.Delete(ctx, "c"))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})
}
