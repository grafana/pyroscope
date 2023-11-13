package refctr

import "sync"

type Counter struct {
	m sync.Mutex
	c int
}

// Inc increments the counter and calls the init function,
// if this is the first reference. The call returns an
// error only if init call has failed, and the reference
// has not been incremented.
func (r *Counter) Inc(init func() error) (err error) {
	r.m.Lock()
	defer func() {
		// If initialization fails, we need to make sure
		// the next call makes another attempt.
		if err != nil {
			r.c--
		}
		r.m.Unlock()
	}()
	if r.c++; r.c > 1 {
		return nil
	}
	// Mutex is acquired during the call in order to serialize
	// access to the resources, so that the consequent callers
	// only have access to them after initialization finishes.
	return init()
}

// Dec decrements the counter and calls the release function,
// if this is the last reference.
func (r *Counter) Dec(release func()) {
	r.m.Lock()
	if r.c < 0 {
		panic("bug: negative reference counter")
	}
	if r.c--; r.c < 1 {
		release()
	}
	r.m.Unlock()
}
