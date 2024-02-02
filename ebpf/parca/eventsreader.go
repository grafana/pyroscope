package parca

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type EventsReader struct {
	l      log.Logger
	rd     *perf.Reader
	m      *ebpf.Map
	events chan []byte
	drop   bool
}

func NewEventsReader(l log.Logger, m *ebpf.Map, events chan []byte, drop bool) (*EventsReader, error) {
	rd, err := perf.NewReader(m, 8*0x1000)
	if err != nil {
		return nil, err
	}
	e := &EventsReader{
		l:      l,
		rd:     rd,
		m:      m,
		drop:   drop,
		events: events,
	}
	return e, nil
}

func (e *EventsReader) Start() {
	go func() {
		defer e.rd.Close()
		for {
			record, err := e.rd.Read()
			if err != nil {
				close(e.events)
				return
			}
			if e.drop {
				select {
				case e.events <- record.RawSample:
				default:
					_ = level.Warn(e.l).Log("msg", "EventsReader dropping event")
				}
			} else {
				e.events <- record.RawSample
			}
		}
	}()
}
