package timer

import (
	"time"

	"github.com/sirupsen/logrus"
)

// StartWorker news a timer for doing the function
func StartWorker(name string, done chan struct{}, interval time.Duration, fn func() error) {
	go func() {
		// new a timer for doing the function periodly
		ticker := time.NewTimer(interval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// do the function
				if err := fn(); err != nil {
					logrus.Errorf("run worker(%s): %v", name, err)
				}

				// reset the timer
				ticker.Reset(interval)
			}
		}
	}()
}
