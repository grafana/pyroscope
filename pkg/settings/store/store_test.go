package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

type testObj struct {
	Name       string
	Data       string
	Generation int64
}

const storeJSON = `{
  "generation":"4",
  "elements":[
    {"Name":"a","Data":"data-a-v3","Generation":3},
    {"Name":"b","Data":"data-b","Generation":1}
  ]
}`

type testObjHelper struct{}

func (*testObjHelper) ID(o *testObj) string {
	return o.Name
}

func (*testObjHelper) GetGeneration(o *testObj) int64 {
	return o.Generation
}

func (*testObjHelper) SetGeneration(o *testObj, gen int64) {
	o.Generation = gen
}

func (*testObjHelper) FromStore(data json.RawMessage) (*testObj, error) {
	var obj testObj
	err := json.Unmarshal(data, &obj)
	return &obj, err
}

func (*testObjHelper) ToStore(obj *testObj) (json.RawMessage, error) {
	return json.Marshal(obj)
}

func (*testObjHelper) TypePath() string {
	return "testobj.v1"
}

type testStore struct {
	*GenericStore[*testObj, *testObjHelper]
	bucketPath string
}

func newTestStore(t testing.TB, tenantID string) *testStore {
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
			Key{TenantID: tenantID},
			&testObjHelper{},
		),
		bucketPath: bucketPath,
	}
}

func Test_GenericStore(t *testing.T) {
	s := newTestStore(t, "user-a")
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{}, result.Elements)
	})

	t.Run("one element", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a"}, nil, 0))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a", Generation: 1},
		}, result.Elements)
	})

	t.Run("second element", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "b", Data: "data-b"}, nil, 0))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a", Generation: 1},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("update without generation", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v2"}, nil, 0))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a-v2", Generation: 2},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("update with generation", func(t *testing.T) {
		observedGeneration := int64(2)
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v3"}, &observedGeneration, 0))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a-v3", Generation: 3},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("validate stored data is as expected", func(t *testing.T) {
		storePath := filepath.Join(s.bucketPath, "user-a/testobj.v1.json")
		actual, err := os.ReadFile(storePath)
		require.NoError(t, err)
		require.JSONEq(t, storeJSON, string(actual))
	})

	t.Run("restore from stored data", func(t *testing.T) {
		newS := newTestStore(t, "user-b")
		storePath := filepath.Join(newS.bucketPath, "user-b/testobj.v1.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o755))
		require.NoError(t, os.WriteFile(
			storePath,
			[]byte(storeJSON),
			0o644,
		))
		result, err := newS.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a-v3", Generation: 3},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})

	t.Run("update with wrong generation", func(t *testing.T) {
		observedGeneration := int64(2)
		require.ErrorContains(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v4"}, &observedGeneration, 0), "conflicting update, please try again: observed_generation=2, store_generation=3")
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "a", Data: "data-a-v3", Generation: 3},
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
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
		require.ErrorContains(t, s.Delete(ctx, "c"), "element not found")
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, []*testObj{
			{Name: "b", Data: "data-b", Generation: 1},
		}, result.Elements)
	})
}

func Test_GenericStore_Upsert_MaxElements(t *testing.T) {
	s := newTestStore(t, "user-a")
	ctx := context.Background()
	const max = 2

	t.Run("insert up to the limit", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a"}, nil, max))
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "b", Data: "data-b"}, nil, max))
	})

	t.Run("inserting beyond the limit is rejected", func(t *testing.T) {
		err := s.Upsert(ctx, &testObj{Name: "c", Data: "data-c"}, nil, max)
		var mErr *ErrMaxElementsExceeded
		require.ErrorAs(t, err, &mErr)
		require.Equal(t, max, mErr.Max)

		// the rejected element was not stored
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Len(t, result.Elements, max)
	})

	t.Run("updating an existing element is allowed at the limit", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "a", Data: "data-a-v2"}, nil, max))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Len(t, result.Elements, max)
	})

	t.Run("0 disables the limit", func(t *testing.T) {
		require.NoError(t, s.Upsert(ctx, &testObj{Name: "c", Data: "data-c"}, nil, 0))
		result, err := s.Get(ctx)
		require.NoError(t, err)
		require.Len(t, result.Elements, 3)
	})
}
