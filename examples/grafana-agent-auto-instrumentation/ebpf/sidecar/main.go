package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"
)

//go:noinline
func work(n int) {
	// revive:disable:empty-block this is fine because this is a example app, not real production code
	for i := 0; i < n; i++ {
	}
	// revive:enable:empty-block
}

func workUntil(c context.Context) {
	for {
		if c.Err() != nil {
			break
		}
		work(100_000)
	}
}

func orchestrate(ctx context.Context) error {
	tasks := []*struct {
		lck       sync.Mutex
		frequency time.Duration
		length    time.Duration
		due       time.Time
		cmd       *exec.Cmd
	}{
		{frequency: 5 * time.Second, length: 100 * time.Millisecond},
		{frequency: 5 * time.Second, length: 500 * time.Millisecond},
		{frequency: 10 * time.Second, length: 1 * time.Second},
		{frequency: 15 * time.Second, length: 3 * time.Second},
		{frequency: 15 * time.Second, length: 5 * time.Second},
	}

	waitCh := make(chan struct {
		idx int
		err error
	}, len(tasks))

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case w := <-waitCh:
			// handle finished tasks
			if w.err != nil {
				return fmt.Errorf("error waiting for command %v: %w", tasks[w.idx].cmd.Args, w.err)
			}
			log.Printf("finished with %v", tasks[w.idx].cmd.Args)
			tasks[w.idx].cmd = nil
			tasks[w.idx].due = time.Now().Add(tasks[w.idx].frequency)
		case <-ctx.Done():
			// handle context cancellation
			for _, t := range tasks {
				if t.cmd != nil && t.cmd.Process != nil {
					t.cmd.Process.Signal(os.Interrupt)
				}
			}

		case <-ticker.C:
			// check if we need to start any tasks
			for idx, t := range tasks {
				// already running
				if t.cmd != nil {
					continue
				}

				// is due
				if time.Now().After(t.due) {
					t.cmd = exec.Command(os.Args[0], t.length.String())
					log.Printf("starting with %v", t.cmd.Args)
					err := t.cmd.Start()
					if err != nil {
						return fmt.Errorf("error starting command: %v", err)
					}
					go func(idx int) {
						err := tasks[idx].cmd.Wait()
						waitCh <- struct {
							idx int
							err error
						}{idx: idx, err: err}
					}(idx)

				}

			}
		}
	}

	return nil
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// when argument is provided, sleep for that amount
	if len(os.Args) == 2 {
		d, err := time.ParseDuration(os.Args[1])
		if err != nil {
			return fmt.Errorf("error parsing duration: %v", err)
		}

		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		log.Printf("starting work for %s\n", d)
		workUntil(ctx)
		return nil
	}
	if len(os.Args) > 2 {
		return errors.New("too many arguments")
	}

	return orchestrate(ctx)
}

func main() {
	err := run()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
