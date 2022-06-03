# Mango Cache
[![GoDoc](https://godoc.org/github.com/goburrow/cache?status.svg)](https://godoc.org/github.com/goburrow/cache)
![Go](https://github.com/goburrow/cache/workflows/Go/badge.svg)

Partial implementations of [Guava Cache](https://github.com/google/guava) in Go.

Supported cache replacement policies:

- LRU
- Segmented LRU (default)
- TinyLFU (experimental)

The TinyLFU implementation is inspired by
[Caffeine](https://github.com/ben-manes/caffeine) by Ben Manes and
[go-tinylfu](https://github.com/dgryski/go-tinylfu) by Damian Gryski.

## Download

```
go get -u github.com/goburrow/cache
```

## Example

```go
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/goburrow/cache"
)

func main() {
	load := func(k cache.Key) (cache.Value, error) {
		time.Sleep(100 * time.Millisecond) // Slow task
		return fmt.Sprintf("%d", k), nil
	}
	// Create a loading cache
	c := cache.NewLoadingCache(load,
		cache.WithMaximumSize(100),                 // Limit number of entries in the cache.
		cache.WithExpireAfterAccess(1*time.Minute), // Expire entries after 1 minute since last accessed.
		cache.WithRefreshAfterWrite(2*time.Minute), // Expire entries after 2 minutes since last created.
	)

	getTicker := time.Tick(100 * time.Millisecond)
	reportTicker := time.Tick(5 * time.Second)
	for {
		select {
		case <-getTicker:
			_, _ = c.Get(rand.Intn(200))
		case <-reportTicker:
			st := cache.Stats{}
			c.Stats(&st)
			fmt.Printf("%+v\n", st)
		}
	}
}
```

## Performance

See [traces](traces/) and [benchmark](https://github.com/goburrow/cache/wiki/Benchmark)

![report](traces/report.png)
