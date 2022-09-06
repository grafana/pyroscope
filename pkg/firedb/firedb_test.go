package firedb

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	googlev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

func TestCreateLocalDir(t *testing.T) {
	dataPath := t.TempDir()
	localFile := dataPath + "/local"
	require.NoError(t, ioutil.WriteFile(localFile, []byte("d"), 0o644))
	_, err := New(&Config{
		DataPath:      dataPath,
		BlockDuration: 30 * time.Minute,
	}, log.NewNopLogger(), nil)
	require.Error(t, err)
	require.NoError(t, os.Remove(localFile))
	_, err = New(&Config{
		DataPath:      dataPath,
		BlockDuration: 30 * time.Minute,
	}, log.NewNopLogger(), nil)
	require.NoError(t, err)
}

var cpuProfileGenerator = func(tsNano int64, t testing.TB) (*googlev1.Profile, string) {
	p := parseProfile(t, "testdata/profile")
	p.TimeNanos = tsNano
	return p, "process_cpu"
}

func ingestProfiles(b testing.TB, db *FireDB, generator func(tsNano int64, t testing.TB) (*googlev1.Profile, string), from, to int64, step time.Duration, externalLabels ...*commonv1.LabelPair) {
	b.Helper()
	for i := from; i <= to; i += int64(step) {
		p, name := generator(i, b)
		require.NoError(b, db.Head().Ingest(
			context.Background(), p, uuid.New(), append(externalLabels, &commonv1.LabelPair{Name: model.MetricNameLabel, Value: name})...))
	}
}

func BenchmarkDBSelectProfile(b *testing.B) {
	var (
		testDir = b.TempDir()
		end     = time.Unix(0, int64(time.Hour))
		start   = end.Add(-time.Hour)
		step    = 15 * time.Second
	)

	db, err := New(&Config{
		DataPath:      testDir,
		BlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, log.NewNopLogger(), nil)
	require.NoError(b, err)

	ingestProfiles(b, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step)

	require.NoError(b, db.Flush(context.Background()))

	db.runBlockQuerierSync(context.Background())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := db.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
			LabelSelector: "{}",
			Type:          mustParseProfileSelector(b, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
			Start:         int64(model.TimeFromUnixNano(start.UnixNano())),
			End:           int64(model.TimeFromUnixNano(end.UnixNano())),
		}))
		require.NoError(b, err)
		require.True(b, len(resp.Msg.Profiles) != 0)
	}
}

func TestCloseFile(t *testing.T) {
	var (
		dataPath = t.TempDir()
		end      = time.Unix(0, int64(time.Hour))
		start    = end.Add(-time.Hour)
		step     = 15 * time.Second
	)
	db, err := New(&Config{
		DataPath:      dataPath,
		BlockDuration: 30 * time.Minute,
	}, log.NewNopLogger(), nil)
	require.NoError(t, err)
	require.NoError(t, db.StartAsync(context.Background()))
	require.NoError(t, db.AwaitRunning(context.Background()))

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step)
	require.NoError(t, db.Flush(context.Background()))
	db.runBlockQuerierSync(context.Background())
	_, err = db.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
		Start:         int64(model.TimeFromUnixNano(start.UnixNano())),
		End:           int64(model.TimeFromUnixNano(end.UnixNano())),
	}))

	require.NoError(t, err)
	db.StopAsync()
	require.NoError(t, db.AwaitTerminated(context.Background()))
	require.NoError(t, os.RemoveAll(dataPath))
}
