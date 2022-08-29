package firedb

import (
	"context"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	googlev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func BenchmarkDBSelectProfile(b *testing.B) {
	testDir := b.TempDir()
	end := time.Now()
	start := end.Add(-time.Hour)
	step := 15 * time.Second

	db, err := New(&Config{
		DataPath:      testDir,
		BlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, log.NewNopLogger(), nil)
	require.NoError(b, err)

	ingestProfiles(b, db, func(tsNano int64) (*googlev1.Profile, string) {
		p := parseProfile(b, "testdata/profile")
		p.TimeNanos = tsNano
		return p, "process_cpu"
	}, start.UnixNano(), end.UnixNano(), step)

	require.NoError(b, db.Flush(context.Background()))

	// reopen db to include flushed block and add new head block
	db, err = New(&Config{
		DataPath:      testDir,
		BlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, log.NewNopLogger(), nil)
	require.NoError(b, err)
	ingestProfiles(b, db, func(tsNano int64) (*googlev1.Profile, string) {
		p := parseProfile(b, "testdata/profile")
		p.TimeNanos = tsNano
		return p, "process_cpu"
	}, start.UnixNano(), end.UnixNano(), step)

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

func ingestProfiles(b *testing.B, db *FireDB, generator func(tsNano int64) (*googlev1.Profile, string), from, to int64, step time.Duration, externalLabels ...*commonv1.LabelPair) {
	b.Helper()
	for i := from; i <= to; i += int64(step) {
		p, name := generator(i)
		require.NoError(b, db.Head().Ingest(
			context.Background(), p, uuid.New(), append(externalLabels, &commonv1.LabelPair{Name: model.MetricNameLabel, Value: name})...))
	}
}
