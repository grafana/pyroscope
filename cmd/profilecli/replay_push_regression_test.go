package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type replayRoundTripFunc func(*http.Request) (*http.Response, error)

func (f replayRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type replayInterruptedBody struct{ reads int }

func (b *replayInterruptedBody) Read([]byte) (int, error) { b.reads++; return 0, io.ErrUnexpectedEOF }
func (b *replayInterruptedBody) Close() error             { return nil }

func TestReplayHTTPResumeReconnectFailure(t *testing.T) {
	for _, recover := range []bool{true, false} {
		t.Run(map[bool]string{true: "recover", false: "exhaust"}[recover], func(t *testing.T) {
			original := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = original })
			networkErr := errors.New("reconnect failed")
			attempts := 0
			http.DefaultClient = &http.Client{Transport: replayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				require.Equal(t, "bytes=0-", r.Header.Get("Range"))
				if !recover || attempts == 1 {
					return nil, networkErr
				}
				return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Range": {"bytes 0-0/1"}}, ContentLength: 1, Body: io.NopCloser(bytes.NewReader([]byte("x")))}, nil
			})}
			body := &replayInterruptedBody{}
			r := &resumableReplayBody{ctx: context.Background(), url: "http://example.test", body: body, size: 1}
			defer r.Close()
			got, err := io.ReadAll(r)
			if recover {
				require.NoError(t, err)
				require.Equal(t, []byte("x"), got)
				require.Equal(t, 2, attempts)
			} else {
				require.ErrorIs(t, err, networkErr)
				require.Equal(t, 5, attempts)
			}
			require.Equal(t, 1, body.reads)
		})
	}
}

func TestReplayReaderCycleInterruptedBuildFailures(t *testing.T) {
	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, replayHeader{})
	require.NoError(t, err)
	require.NoError(t, rw.WriteRecord(replayRecord{TimestampNanos: int64(time.Hour), Pprof: testPprofBytes(t)}))
	require.NoError(t, rw.Flush())
	rr, err := newReplayReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	pushed, failed, interrupted, err := runReplayReaderCycle(ctx, &fakePusherClient{}, rr, replayRecord{Pprof: []byte("invalid")}, time.Now(), &replayPushParams{Speed: 1, BatchSize: 1})
	require.NoError(t, err)
	require.True(t, interrupted)
	require.Zero(t, pushed)
	require.Equal(t, 1, failed)
}
