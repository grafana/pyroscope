package alignedticker

import "time"

type AlignedTicker struct {
	C      <-chan time.Time
	ch     chan time.Time
	stopCh chan struct{}
	d      time.Duration
}

func NewAlignedTicker(d time.Duration) *AlignedTicker {
	ch := make(chan time.Time)
	stopCh := make(chan struct{})
	res := &AlignedTicker{
		C:      ch,
		ch:     ch,
		stopCh: stopCh,
		d:      d,
	}
	go res.loop()
	return res
}

func (t *AlignedTicker) Stop() {
	close(t.stopCh)
}

func (t *AlignedTicker) loop() {
	now := time.Now()
	prev := now.Truncate(t.d)
	next := prev.Add(t.d)
	first := next.Sub(now)
	time.Sleep(first)
	t.ch <- next
	impl := time.NewTicker(t.d)
	defer func() {
		impl.Stop()
	}()
	for {
		select {
		case <-t.stopCh:
			return
		case now, ok := <-impl.C:
			if !ok {
				return
			}
			prev := now.Truncate(t.d)
			next := prev.Add(t.d)
			if now.Sub(prev) < next.Sub(now) {
				t.ch <- prev
			} else {
				t.ch <- next
			}
		}
	}
}
