package objstore_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockobjstore"
)

func TestReadOnlyFileError(t *testing.T) {
	m := new(mockobjstore.MockBucket)
	m.EXPECT().Get(mock.Anything, "test").Return(failReader{}, nil).Once()
	_, err := objstore.Download(context.Background(), "test", m, t.TempDir())
	require.ErrorIs(t, err, io.ErrNoProgress)
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, io.ErrNoProgress }
func (failReader) Close() error             { return nil }
