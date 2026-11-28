package gzipwriter

import (
	"bytes"
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfiguredThresholdAndReuse(t *testing.T) {
	previous := parallelMinSize.Load()
	t.Cleanup(func() { parallelMinSize.Store(previous) })
	var cfg Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.EqualValues(t, DefaultParallelMinSize, cfg.ParallelGzipMinSize)
	require.NoError(t, fs.Parse([]string{"-compression.parallel-gzip-min-size=128"}))
	require.NoError(t, Configure(cfg))

	w := NewAdaptiveWriter(io.Discard)
	for _, size := range []int{127, 128, 129, 0, 128, 127} {
		var out bytes.Buffer
		w.Reset(&out)
		data := bytes.Repeat([]byte{'x'}, size)
		_, err := w.Write(data)
		require.NoError(t, err)
		if size < 128 {
			require.Same(t, w.serial, w.active)
		} else {
			require.Same(t, w.pgzip, w.active)
		}
		require.NoError(t, w.Close())
		checkRoundTrip(t, out.Bytes(), data)
	}

	w.Reset(io.Discard)
	_, err := w.Write([]byte("small"))
	require.NoError(t, err)
	require.NoError(t, Configure(Config{ParallelGzipMinSize: 0}))
	_, err = w.Write(make([]byte, 256))
	require.NoError(t, err)
	require.Same(t, w.serial, w.active, "selection remains fixed during a stream")
	require.NoError(t, w.Close())
	w.Reset(io.Discard)
	require.NoError(t, w.Close())
	require.Same(t, w.pgzip, w.active, "zero threshold includes empty streams")

	require.Error(t, Configure(Config{ParallelGzipMinSize: -1}))
	require.Zero(t, parallelMinSize.Load(), "invalid configuration leaves policy unchanged")
}
