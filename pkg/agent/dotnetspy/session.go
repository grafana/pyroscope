// +build dotnetspy

package dotnetspy

import (
	"io"
	"strings"
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
	return &session{
		client: dotnetdiag.NewClient(dotnetdiag.DefaultServerAddress(pid)),
		config: dotnetdiag.CollectTracingConfig{
			CircularBufferSizeMB: 10,
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
	r := newRenderer(func(name []byte, val int) {
		s.ch <- line{
			name: name,
			val:  val,
		}
	})

	go func() {
		defer close(s.ch)
		for {
			switch err = stream.Next(); err {
			default:
			case nil:
				continue
			case io.EOF:
				p.Walk(r.visitor)
				r.flush()
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

type renderer struct {
	callBack func(name []byte, val int)
	names    []string
	val      time.Duration
	prev     int
}

func newRenderer(cb func(name []byte, val int)) *renderer {
	return &renderer{callBack: cb}
}

func (r *renderer) visitor(frame profiler.FrameInfo) {
	if frame.Level > r.prev || (frame.Level == 0 && r.prev == 0) {
		r.names = append(r.names, frame.Name)
	} else {
		r.flush()
		if frame.Level == 0 {
			r.names = []string{frame.Name}
		} else {
			r.names = append(r.names[:frame.Level], frame.Name)
		}
	}
	r.val = frame.SampledTime
	r.prev = frame.Level
}

func (r *renderer) flush() {
	r.callBack([]byte(strings.Join(r.names, ";")), int(r.val.Milliseconds()))
}
