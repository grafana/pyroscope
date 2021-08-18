package throttle

import (
	"sync"
	"time"
)

type Throttler struct {
	m        sync.Mutex
	t        time.Time
	Duration time.Duration
}

func New(d time.Duration) *Throttler {
	return &Throttler{
		Duration: d,
	}
}

func (t *Throttler) Run(cb func()) {
	t.m.Lock()
	defer t.m.Unlock()

	now := time.Now()
	if t.t.IsZero() || t.t.Before(now.Add(-t.Duration)) {
		cb()
		t.t = now
	}
}
