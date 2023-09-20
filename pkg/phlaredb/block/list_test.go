package block

import (
	"bytes"
	"context"
	"crypto/rand"
	"path"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	objstore_testutil "github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func TestIterBlockMetas(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())

	u := ulid.MustNew(uint64(model.Now()), rand.Reader).String()
	err := bucketClient.Upload(context.Background(), path.Join(u, "index"), bytes.NewBufferString("foo"))
	require.NoError(t, err)
	meta := Meta{
		Version: MetaVersion3,
		ULID:    ulid.MustNew(ulid.Now(), rand.Reader),
	}
	buf := bytes.NewBuffer(nil)
	_, err = meta.WriteTo(buf)
	require.NoError(t, err)

	err = bucketClient.Upload(context.Background(), path.Join(meta.ULID.String(), MetaFilename), buf)
	require.NoError(t, err)
	found := false
	err = IterBlockMetas(context.Background(), bucketClient, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), func(m *Meta) {
		found = true
		require.Equal(t, meta.ULID, m.ULID)
	})
	require.NoError(t, err)
	require.True(t, found, "expected to find block meta")
}
