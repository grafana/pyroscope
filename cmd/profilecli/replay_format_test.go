package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestReplayFormat_WriteReadRoundtrip(t *testing.T) {
	t.Parallel()

	header := replayHeader{
		SourceQuery: `{service_name="foo"}`,
		Tenants:     []string{"tenant-a"},
		From:        1000,
		To:          2000,
		CreatedAt:   1234,
	}

	records := []replayRecord{
		{
			Labels: []*typesv1.LabelPair{
				{Name: "service_name", Value: "foo"},
				{Name: "__profile_type__", Value: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"},
			},
			TimestampNanos: 1_700_000_000_000_000_000,
			Pprof:          []byte{0x1f, 0x8b, 0x08, 0x00, 0x01, 0x02, 0x03},
		},
		{
			Labels: []*typesv1.LabelPair{
				{Name: "service_name", Value: "foo"},
			},
			TimestampNanos: -42, // negative timestamps must round-trip correctly (varint zigzag)
			Pprof:          []byte{},
		},
		{
			Labels:         nil,
			TimestampNanos: 0,
			Pprof:          bytes.Repeat([]byte{0xAB}, 4096),
		},
	}

	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, header)
	require.NoError(t, err)
	for _, rec := range records {
		require.NoError(t, rw.WriteRecord(rec))
	}
	require.NoError(t, rw.Flush())

	rr, err := newReplayReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	assert.Equal(t, replayFormatVersion, rr.Header.Version)
	assert.Equal(t, header.SourceQuery, rr.Header.SourceQuery)
	assert.Equal(t, header.Tenants, rr.Header.Tenants)
	assert.Equal(t, header.From, rr.Header.From)
	assert.Equal(t, header.To, rr.Header.To)
	assert.Equal(t, header.CreatedAt, rr.Header.CreatedAt)

	var got []replayRecord
	for {
		rec, err := rr.ReadRecord()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		got = append(got, rec)
	}

	require.Len(t, got, len(records))
	for i, want := range records {
		assert.Equal(t, want.TimestampNanos, got[i].TimestampNanos)
		assert.Equal(t, want.Pprof, got[i].Pprof)
		require.Len(t, got[i].Labels, len(want.Labels))
		for j, wl := range want.Labels {
			assert.Equal(t, wl.Name, got[i].Labels[j].Name)
			assert.Equal(t, wl.Value, got[i].Labels[j].Value)
		}
	}
}

func TestReplayFormat_BadMagic(t *testing.T) {
	t.Parallel()

	_, err := newReplayReader(bytes.NewReader([]byte("NOTAREPLAYFILE")))
	require.Error(t, err)
}

func TestReplayFormat_EmptyFile(t *testing.T) {
	t.Parallel()

	_, err := newReplayReader(bytes.NewReader(nil))
	require.Error(t, err)
}
