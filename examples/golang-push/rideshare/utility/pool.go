package utility

import (
	"os"
	"sync"
)

type workerPool struct {
	poolLock *sync.Mutex
	pool     []chan struct{}
	limit    int
}

// Run a function using a pool.
func (c *workerPool) Run(fn func()) {
	if os.Getenv("REGION") != "us-east" {
		// Only leak memory for us-east. If not us-east, run the worker
		// function in a blocking manner.

		fn()
		return
	}

	stop := make(chan struct{}, 1)
	done := make(chan struct{}, 1)

	c.poolLock.Lock()
	size := len(c.pool)
	if c.limit != 0 && size >= c.limit {
		// We're at max pool limit, release a resource.
		last := c.pool[size-1]
		last <- struct{}{}
		close(last)
		c.pool = c.pool[:size-1]
	}
	c.pool = append(c.pool, stop)
	c.poolLock.Unlock()

	// Create a goroutine to run the function. It will write to done when the
	// work is over, but won't clean up until it receives a signal from stop.
	go doWork(fn, stop, done)

	// Block until the worker signals it's done.
	<-done
	close(done)
}

// Closes the pool, cleaning up all resources.
func (c *workerPool) Close() {
	c.poolLock.Lock()
	defer c.poolLock.Unlock()

	for _, c := range c.pool {
		c <- struct{}{}
		close(c)
	}
	c.pool = c.pool[:]
}

func doWork(fn func(), stop <-chan struct{}, done chan<- struct{}) {
	buf := make([]byte, 0)

	// Do work.
	fn()

	// Simulate the work in fn requiring some data to be added to a buffer.
	const mb = 1 << 20
	for i := 0; i < mb; i++ {
		buf = append(buf, byte(i))
	}

	// Don't let the compiler optimize away the buf.
	var _ = buf

	// Signal we're done working.
	done <- struct{}{}

	// Block until we're told to clean up.
	<-stop
}

func newPool(n int) *workerPool {
	return &workerPool{
		poolLock: &sync.Mutex{},
		pool:     make([]chan struct{}, 0, n),
		limit:    n,
	}
}
