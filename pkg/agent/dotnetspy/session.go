// +build dotnetspy

package dotnetspy

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/pyroscope-io/dotnetdiag"
	"github.com/pyroscope-io/dotnetdiag/nettrace"
	"github.com/pyroscope-io/dotnetdiag/nettrace/profiler"
)

type session struct {
	client  *dotnetdiag.Client
	config  dotnetdiag.CollectTracingConfig
	session *dotnetdiag.Session

	ch   chan line
	m    sync.Mutex
	stop bool
}

type line struct {
	name []byte
	val  int
}

func newSession(pid int) *session {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	return &session{
		client: dotnetdiag.NewClient(waitDiagnosticServer(ctx, pid)),
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

func (s *session) Start() error {
	ns, err := s.client.CollectTracing(s.config)
	if err != nil {
		return err
	}

	s.session = ns
	stream := nettrace.NewStream(ns)
	trace, err := stream.Open()
	if err != nil {
		_ = ns.Close()
		return err
	}

	p := profiler.NewSampleProfiler(trace)
	stream.EventHandler = p.EventHandler
	stream.MetadataHandler = p.MetadataHandler
	stream.StackBlockHandler = p.StackBlockHandler
	stream.SequencePointBlockHandler = p.SequencePointBlockHandler

	s.ch = make(chan line)
	go func() {
		defer close(s.ch)
		for {
			switch err = stream.Next(); err {
			default:
			case nil:
				continue
			case io.EOF:
				for k, v := range p.Samples() {
					s.ch <- line{
						name: []byte(k),
						val:  int(v.Milliseconds() / 10),
					}
				}
			}
			return
		}
	}()

	return nil
}

func (s *session) Flush(cb func([]byte, uint64)) error {
	s.session.Close()
	for v := range s.ch {
		cb(v.name, uint64(v.val))
	}
	s.m.Lock()
	defer s.m.Unlock()
	if s.stop {
		return nil
	}
	return s.Start()
}

func (s *session) Stop() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.session.Close()
	s.stop = true
	return nil
}

// .Net runtime requires some time to initialize diagnostic IPC server and
// start accepting connections.
func waitDiagnosticServer(ctx context.Context, pid int) string {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	var failures int
	for {
		select {
		case <-ctx.Done():
			return ""
		case <-ticker.C:
			if addr := dotnetdiag.DefaultServerAddress(pid); addr != "" {
				if failures > 0 {
					time.Sleep(time.Millisecond * 100)
				}
				return addr
			}
			failures++
		}
	}
}
