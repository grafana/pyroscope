package cache

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache/lfu"
	"github.com/valyala/bytebufferpool"
)

type Cache struct {
	db        *badger.DB
	ClickConn clickhouse.Conn
	lfu       *lfu.Cache
	metrics   *Metrics
	codec     Codec

	prefix string
	ttl    time.Duration

	evictionsDone chan struct{}
	writeBackDone chan struct{}
	flushOnce     sync.Once
}

type Config struct {
	*badger.DB
	ClickConn clickhouse.Conn
	*Metrics
	Codec

	// Prefix for badger DB keys.
	Prefix string
	// TTL specifies number of seconds an item can reside in cache after
	// the last access. An obsolete item is evicted. Setting TTL to less
	// than a second disables time-based eviction.
	TTL time.Duration
}

// Codec is a shorthand of coder-decoder. A Codec implementation
// is responsible for type conversions and binary representation.
type Codec interface {
	Serialize(w io.Writer, key string, value interface{}) error
	Deserialize(r io.Reader, key string) (interface{}, error)
	// New returns a new instance of the type. The method is
	// called by GetOrCreate when an item can not be found by
	// the given key.
	New(key string) interface{}
}

type Metrics struct {
	MissesCounter     prometheus.Counter
	ReadsCounter      prometheus.Counter
	DBWrites          prometheus.Observer
	DBReads           prometheus.Observer
	WriteBackDuration prometheus.Observer
	EvictionsDuration prometheus.Observer
}

func New(c Config) *Cache {
	cache := &Cache{
		lfu:           lfu.New(),
		db:            c.DB,
		codec:         c.Codec,
		metrics:       c.Metrics,
		prefix:        c.Prefix,
		ttl:           c.TTL,
		evictionsDone: make(chan struct{}),
		writeBackDone: make(chan struct{}),
	}

	evictionChannel := make(chan lfu.Eviction)
	writeBackChannel := make(chan lfu.Eviction)

	// eviction channel for saving cache items to disk
	cache.lfu.EvictionChannel = evictionChannel
	cache.lfu.WriteBackChannel = writeBackChannel
	cache.lfu.TTL = int64(c.TTL.Seconds())

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"127.0.0.1:9000"},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "clickhouse",
			Password: "clickhouse",
		},
	})
	if err != nil {
		fmt.Println("connection err:", err)
	}

	cache.ClickConn = conn

	// start a goroutine for saving the evicted cache items to disk
	go func() {
		for e := range evictionChannel {
			// TODO(kolesnikovae): these errors should be at least logged.
			//  Perhaps, it will be better if we move it outside of the cache.
			//  Taking into account that writes almost always happen in batches,
			//  We should definitely take advantage of BadgerDB write batch API.
			//  Also, WriteBack and Evict could be combined. We also could
			//  consider moving caching to storage/db.
			cache.saveToDisk(e.Key, e.Value)
		}
		close(cache.evictionsDone)
	}()

	// start a goroutine for saving the evicted cache items to disk
	go func() {
		for e := range writeBackChannel {
			cache.saveToDisk(e.Key, e.Value)
		}
		close(cache.writeBackDone)
	}()

	return cache
}

func (cache *Cache) Put(key string, val interface{}) {
	cache.lfu.Set(key, val)
}

func (cache *Cache) saveToDisk(key string, val interface{}) error {
	ctx := context.Background()
	b := bytebufferpool.Get()
	defer bytebufferpool.Put(b)
	if err := cache.codec.Serialize(b, key, val); err != nil {
		return fmt.Errorf("serialization: %w", err)
	}
	cache.metrics.DBWrites.Observe(float64(b.Len()))

	q := fmt.Sprintf("INSERT INTO cache (key, val) VALUES ('%s', '%s')", key, b.Bytes())
	err := cache.ClickConn.Exec(ctx, q)
	return err
}

func (cache *Cache) Sync() error {
	return cache.lfu.Iterate(func(k string, v interface{}) error {
		return cache.saveToDisk(k, v)
	})
}

func (cache *Cache) Flush() {
	cache.flushOnce.Do(func() {
		// Make sure there is no pending items.
		close(cache.lfu.WriteBackChannel)
		<-cache.writeBackDone
		// evict all the items in cache
		cache.lfu.Evict(cache.lfu.Len())
		close(cache.lfu.EvictionChannel)
		// wait until all evictions are done
		<-cache.evictionsDone
	})
}

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (cache *Cache) Evict(percent float64) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(cache.metrics.EvictionsDuration.Observe))
	cache.lfu.Evict(int(float64(cache.lfu.Len()) * percent))
	timer.ObserveDuration()
}

func (cache *Cache) WriteBack() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(cache.metrics.WriteBackDuration.Observe))
	cache.lfu.WriteBack()
	timer.ObserveDuration()
}

func (cache *Cache) Delete(key string) error {
	ctx := context.Background()
	cache.lfu.Delete(key)
	q := fmt.Sprintf("DELETE FROM cache WHERE key='%s'", key)
	err := cache.ClickConn.Exec(ctx, q)
	return err
}

func (cache *Cache) Discard(key string) {
	cache.lfu.Delete(key)
}

// DiscardPrefix deletes all data that matches a certain prefix
// In both cache and database
func (cache *Cache) DiscardPrefix(prefix string) error {
	cache.lfu.DeletePrefix(prefix)
	return dropPrefix(cache.ClickConn, cache.prefix+prefix)
}

const defaultBatchSize = 1 << 10 // 1K items

func dropPrefix(ClickConn clickhouse.Conn, p string) error {
	var err error
	ctx := context.Background()

	// Fetch the keys to delete in batches of 1000
	var keys []string
	offset := 0
	for {
		q := fmt.Sprintf("SELECT key FROM cache WHERE key STARTS WITH '%s' LIMIT 1000 OFFSET %d", p, offset)
		rows, err := ClickConn.Query(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()

		var k string
		for rows.Next() {
			if err = rows.Scan(&k); err != nil {
				return err
			}
			keys = append(keys, k)
		}
		if err = rows.Err(); err != nil {
			return err
		}

		// If we fetched fewer than 1000 rows, we're done
		if len(keys) < 1000 {
			break
		}

		offset += 1000
	}

	// Delete the keys in batches of 1000
	for len(keys) > 0 {
		batchSize := 1000
		if len(keys) < batchSize {
			batchSize = len(keys)
		}
		batch := keys[:batchSize]
		keys = keys[batchSize:]

		q := fmt.Sprintf("ALTER TABLE cache DELETE WHERE key IN ('%s')", strings.Join(batch, "', '"))
		if err = ClickConn.Exec(ctx, q); err != nil {
			return err
		}
	}

	return nil
}

func (cache *Cache) GetOrCreate(key string) (interface{}, error) {
	return cache.get(key, true)
}

func (cache *Cache) Lookup(key string) (interface{}, bool) {
	v, err := cache.get(key, false)
	return v, v != nil && err == nil
}

func (cache *Cache) get(key string, createNotFound bool) (interface{}, error) {
	ctx := context.Background()
	cache.metrics.ReadsCounter.Inc()
	return cache.lfu.GetOrSet(key, func() (interface{}, error) {
		cache.metrics.MissesCounter.Inc()

		q := fmt.Sprintf("SELECT val FROM cache WHERE key='%s'", key)
		row, err := cache.ClickConn.Query(ctx, q)
		if err != nil {
			fmt.Println("yeh hai error", err)
		}

		var buf []byte
		err = row.Scan(&buf)

		switch {
		default:
			return nil, err
		case err == nil:
		case errors.Is(err, sql.ErrNoRows):
			if createNotFound {
				return cache.codec.New(key), nil
			}
			return nil, nil
		}

		cache.metrics.DBReads.Observe(float64(len(buf)))
		return cache.codec.Deserialize(bytes.NewReader(buf), key)
	})
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}
