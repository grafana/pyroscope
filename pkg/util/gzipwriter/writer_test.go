package gzipwriter

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"math/rand"
	"testing"

	kgzip "github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestWriterRoundTripAndReuse(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, ignore)
	data := make([]byte, 6*blockSize+17)
	_, err := rand.New(rand.NewSource(1)).Read(data)
	require.NoError(t, err)
	var out bytes.Buffer
	w := NewWriter(&out)
	for _, size := range []int{0, 1, len(data), 32, len(data)} {
		out.Reset()
		w.Reset(&out)
		for offset := 0; offset < size; {
			end := min(offset+64<<10, size)
			n, err := w.Write(data[offset:end])
			require.NoError(t, err)
			require.Equal(t, end-offset, n)
			offset = end
		}
		require.NoError(t, w.Flush())
		require.NoError(t, w.Close())
		require.NoError(t, w.Close())
		checkRoundTrip(t, out.Bytes(), data[:size])
		_, err := w.Write(data[:1])
		require.ErrorIs(t, err, io.ErrClosedPipe)
		w.Reset(io.Discard)
	}
}

func TestAdaptiveWriterKeepsSmallMessagesSerial(t *testing.T) {
	var out bytes.Buffer
	w := NewAdaptiveWriter(&out)
	for _, size := range []int{0, 100, blockSize, 100, 2 * blockSize} {
		out.Reset()
		w.Reset(&out)
		data := bytes.Repeat([]byte{'x'}, size)
		_, err := w.Write(data)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		checkRoundTrip(t, out.Bytes(), data)
		if size < blockSize {
			require.Same(t, w.serial, w.active)
			var serial bytes.Buffer
			z := kgzip.NewWriter(&serial)
			_, err := z.Write(data)
			require.NoError(t, err)
			require.NoError(t, z.Close())
			require.Equal(t, serial.Bytes(), out.Bytes())
		} else {
			require.Same(t, w.pgzip, w.active)
		}
	}
}

func TestWriterErrorsDrainAndAllowReuse(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, ignore)
	want := errors.New("destination failed")
	data := bytes.Repeat([]byte("profile data"), blockSize)
	for _, factory := range []func(io.Writer) *Writer{NewWriter, NewAdaptiveWriter} {
		for _, failAfter := range []int{0, 10, 1024} {
			for _, failure := range []error{want, nil} {
				w := factory(&failingWriter{remaining: failAfter, err: failure})
				_, _ = w.Write(data)
				err := w.Close()
				if failure == nil {
					require.ErrorIs(t, err, io.ErrShortWrite)
				} else {
					require.ErrorIs(t, err, failure)
				}
				var out bytes.Buffer
				w.Reset(&out)
				_, err = w.Write(data)
				require.NoError(t, err)
				require.NoError(t, w.Close())
				checkRoundTrip(t, out.Bytes(), data)
				w.Reset(io.Discard)
			}
		}
	}
}

func TestResetDrainsUnclosedWriter(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, ignore)
	var old, next bytes.Buffer
	w := NewWriter(&old)
	data := bytes.Repeat([]byte{'x'}, 5*blockSize)
	_, err := w.Write(data)
	require.NoError(t, err)
	w.Reset(&next)
	checkRoundTrip(t, old.Bytes(), data)
	_, err = w.Write([]byte("next"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	checkRoundTrip(t, next.Bytes(), []byte("next"))
}

type failingWriter struct {
	remaining int
	err       error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	n := min(len(p), w.remaining)
	w.remaining -= n
	if n < len(p) {
		return n, w.err
	}
	return n, nil
}

func checkRoundTrip(t *testing.T, compressed, want []byte) {
	t.Helper()
	for _, newReader := range []func(io.Reader) (io.ReadCloser, error){
		func(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) },
		func(r io.Reader) (io.ReadCloser, error) { return kgzip.NewReader(r) },
	} {
		r, err := newReader(bytes.NewReader(compressed))
		require.NoError(t, err)
		got, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		require.Equal(t, want, got)
	}
}
