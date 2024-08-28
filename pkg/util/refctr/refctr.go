package refctr

import "sync"

type Counter struct {
	m   sync.Mutex
	c   int
	err error
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

// IncErr is identical to Inc, with the only difference that if the
// function fails, the error is returned on any further IncErr call,
// preventing from calling the faulty initialization function again.
func (r *Counter) IncErr(init func() error) (err error) {
	r.m.Lock()
	if r.err != nil {
		err = r.err
		r.m.Unlock()
		return err
	}
	defer func() {
		// If initialization fails, we need to make sure
		// the next call makes another attempt.
		if err != nil {
			r.err = err
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
	defer r.m.Unlock()
	if r.c < 0 {
		panic("bug: negative reference counter")
	}
	if r.c--; r.c < 1 {
		release()
	}
}
