//go:build dotnetspy
// +build dotnetspy

package dotnetspy

import (
	"context"
	"io"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/pyroscope-io/dotnetdiag"
	"github.com/pyroscope-io/dotnetdiag/nettrace"
	"github.com/pyroscope-io/dotnetdiag/nettrace/profiler"
)

type session struct {
	pid     int
	timeout time.Duration

	config  dotnetdiag.CollectTracingConfig
	session *dotnetdiag.Session

	ch      chan line
	stopped bool
}

type line struct {
	name []byte
	val  int
}

func newSession(pid int) *session {
	return &session{
		pid:     pid,
		timeout: 3 * time.Second,
		config: dotnetdiag.CollectTracingConfig{
			CircularBufferSizeMB: 100,
			Providers: []dotnetdiag.ProviderConfig{
				{
					Keywords:     0x0000F00000000000,
					LogLevel:     4,
					ProviderName: "Microsoft-DotNETCore-SampleProfiler",
				},
			},
		},
	}
}

// start opens a new diagnostic session to the process given, and asynchronously
// processes the event stream.
func (s *session) start() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	// If the process does not create Diagnostic Server, the next call will
	// fail, and a session won't be created.
	client := dotnetdiag.NewClient(waitDiagnosticServer(ctx, s.pid))
	ns, err := client.CollectTracing(s.config)
	if err != nil {
		return err
	}

	stream := nettrace.NewStream(ns)
	trace, err := stream.Open()
	if err != nil {
		_ = ns.Close()
		return err
	}

	p := profiler.NewSampleProfiler(trace, profilerOptions...)
	stream.EventHandler = p.EventHandler
	stream.MetadataHandler = p.MetadataHandler
	stream.StackBlockHandler = p.StackBlockHandler
	stream.SequencePointBlockHandler = p.SequencePointBlockHandler

	s.session = ns
	s.ch = make(chan line)
	go func() {
		defer func() {
			s.session = nil
			close(s.ch)
		}()
		for {
			switch err = stream.Next(); err {
			default:
			case nil:
				continue
			case io.EOF:
				// The session is closed by us (on flush or stop call),
				// or the target process has exited.
				for k, v := range p.Samples() {
					// dotnet profiler reports total time v per call stack k.
					// Meanwhile, pyroscope agent expects number of samples is
					// reported. Every sample is a time fraction of second
					// according to sample rate: 1000ms/100 = 10ms by default.
					// To represent reported time v as a number of samples,
					// we divide it by sample duration.
					//
					// Taking into account that under the hood dotnet spy uses
					// Microsoft-DotNETCore-SampleProfiler, which captures a
					// snapshot of each thread's managed callstack every 10 ms,
					// we cannot manage sample rate from outside.
					s.ch <- line{
						name: []byte(k),
						val:  int(v.Milliseconds()) / 10,
					}
				}
			}
			return
		}
	}()

	return nil
}

// flush closes NetTrace stream in order to retrieve samples,
// and starts a new session, if not in stopped state.
func (s *session) flush(cb func([]byte, uint64) error) error {
	// Ignore call, if NetTrace session has not been established.
	var errs error
	if s.session != nil {
		_ = s.session.Close()
		for v := range s.ch {
			if err := cb(v.name, uint64(v.val)); err != nil {
				errs = multierror.Append(errs, err)
			}
		}
	}
	if s.stopped {
		return errs
	}
	if err := s.start(); err != nil {
		errs = multierror.Append(errs, err)
	}
	return errs
}

// stop closes diagnostic session, if it was established, and sets the
// flag preventing session to start again.
func (s *session) stop() error {
	if s.session != nil {
		_ = s.session.Close()
	}
	s.stopped = true
	return nil
}

// .Net runtime requires some time to initialize diagnostic IPC server and
// start accepting connections. If it fails before context cancel, an empty
// string will be returned.
func waitDiagnosticServer(ctx context.Context, pid int) string {
	// Do not wait for the timer to fire for the first time.
	if addr := dotnetdiag.DefaultServerAddress(pid); addr != "" {
		return addr
	}
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ""
		case <-ticker.C:
			if addr := dotnetdiag.DefaultServerAddress(pid); addr != "" {
				return addr
			}
		}
	}
}
