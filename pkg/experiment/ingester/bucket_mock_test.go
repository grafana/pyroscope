package ingester

import (
	"context"
	objstore2 "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/thanos-io/objstore"
	"io"
)

type mockBucket struct {
	upload func(ctx context.Context, name string, r io.Reader) error
}

func (m mockBucket) Close() error {

	//TODO implement me
	panic("implement me")
}

func (m mockBucket) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) Exists(ctx context.Context, name string) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) IsObjNotFoundErr(err error) bool {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) IsAccessDeniedErr(err error) bool {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) Upload(ctx context.Context, name string, r io.Reader) error {
	return m.upload(ctx, name, r)
}

func (m mockBucket) Delete(ctx context.Context, name string) error {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) Name() string {
	//TODO implement me
	panic("implement me")
}

func (m mockBucket) ReaderAt(ctx context.Context, filename string) (objstore2.ReaderAtCloser, error) {
	//TODO implement me
	panic("implement me")
}
