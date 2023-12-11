// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package pproflite_test

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/pproflite"
)

func TestDecoderEncoder(t *testing.T) {
	data, err := ioutil.ReadFile(filepath.Join("testdata", "heap.pprof"))
	require.NoError(t, err)

	inProf, err := profile.ParseData(data)
	require.NoError(t, err)
	inProf.DropFrames = "some"
	inProf.KeepFrames = "most"
	inProf.TimeNanos = 10
	inProf.DurationNanos = 20
	inProf.Comments = []string{"foo", "bar"}

	var inBuf bytes.Buffer
	require.NoError(t, inProf.WriteUncompressed(&inBuf))
	d := pproflite.NewDecoder(inBuf.Bytes())

	var outBuf bytes.Buffer
	e := pproflite.NewEncoder(&outBuf)

	require.NoError(t, d.FieldEach(e.Encode))

	outProf, err := profile.ParseData(outBuf.Bytes())
	require.NoError(t, err)
	require.Equal(t, len(inProf.SampleType), len(outProf.SampleType)) // 1
	require.Equal(t, len(inProf.Sample), len(outProf.Sample))         // 2
	require.Equal(t, len(inProf.Mapping), len(outProf.Mapping))       // 3
	require.Equal(t, len(inProf.Location), len(outProf.Location))     // 4
	require.Equal(t, len(inProf.Function), len(outProf.Function))     // 5
	// 6 - StringTable is not directly exposed by google/pprof/profile
	require.Equal(t, inProf.DropFrames, outProf.DropFrames)               // 7
	require.Equal(t, inProf.KeepFrames, outProf.KeepFrames)               // 8
	require.Equal(t, inProf.TimeNanos, outProf.TimeNanos)                 // 9
	require.Equal(t, inProf.DurationNanos, outProf.DurationNanos)         // 10
	require.Equal(t, inProf.PeriodType.Type, outProf.PeriodType.Type)     // 11
	require.Equal(t, inProf.PeriodType.Unit, outProf.PeriodType.Unit)     // 11
	require.Equal(t, inProf.Period, outProf.Period)                       // 12
	require.Equal(t, inProf.Comments, outProf.Comments)                   // 13
	require.Equal(t, inProf.DefaultSampleType, outProf.DefaultSampleType) // 14

	require.Equal(t, inProf.String(), outProf.String())
	require.Equal(t, inBuf.Bytes(), outBuf.Bytes())
}

func BenchmarkEncodeDecode(b *testing.B) {
	data, err := ioutil.ReadFile(filepath.Join("testdata", "heap.pprof"))
	require.NoError(b, err)

	d := pproflite.NewDecoder(data)
	e := pproflite.NewEncoder(ioutil.Discard)
	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		if err := d.FieldEach(e.Encode); err != nil {
			require.NoError(b, err)
		}
	}
}
