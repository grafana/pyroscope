# `zeropool` is a zero-allocation type-safe `sync.Pool`

## TL;DR

```go
// Zero-value of zeropool.Pool is valid, although the constructor zeropool.New(item func() T) can be used if we want zero values to be initialized.
var pool zeropool.Pool[[]byte]

// This is a []byte, no need to make type-assertion, no need to de-reference.
buf := pool.Get()

// This does not allocate.
pool.Put(buf)
```

## Why?

[Go provides](https://pkg.go.dev/sync#Pool) `sync.Pool` pool implementation that allows storing `any` values (`interface{}` values). It is great but has two major drawbacks:
- It's not type-safe, a type-assertion is needed on the elements provided by `Get()`.
- Since it stores `interface{}` values, it means that your value will escape[^1] to the heap unless you store a pointer (it would escape, but maybe just once).

The second drawback is a major one, and actually is the reason why [Staticcheck SA6002](https://staticcheck.io/docs/checks#SA6002) exists. It's not unusual, for example, to use `sync.Pool` to store allocated byte slices, in which case one would do:

```go
var pool = sync.Pool{New: func() any { return new([]byte) }}

func do() {
	buf := pool.Get().(*[]byte)

	*buf = somethingThatNeedsABuffer(*buf)
	pool.Put(buf[:0])
}

func somethingThatNeedsABuffer(buf []byte) buf {
	buf = append(buf, []byte("something uselesss")...)
	return buf
}
```

Not great (we have to do a type assertion), not terrible (the scope is small).

However, sometimes[^2][^3][^4] we would want to pass that buffer to a function that would only accept `[]byte`, and that has its own lifecycle so it would take the responsibility of putting it back to the pool:

```go
var pool = sync.Pool{New: func() any { return new([]byte) }}

func do() {
	buf := pool.Get().(*[]byte)
	go somethingThatNeedsABuffer(*buf)
}

func somethingThatNeedsABuffer(buf []byte) {
	buf = append(buf, []byte("something uselesss")...)
	pool.Put(&buf)
}
```

Note that in this case, our function `somethingThatNeedsABuffer` allocates a new pointer to that slice.

Enter `zeropool`:

```go
var pool = zeropool.New(func() []byte { return nil })

func do() {
	buf := pool.Get()
	go somethingThatNeedsABuffer(buf)
}

func somethingThatNeedsABuffer(buf []byte) {
	buf = append(buf, []byte("something uselesss")...)
	pool.Put(buf)
}
```

## How to solve "SA6002 - Storing non-pointer values in sync.Pool allocates memory"

Replace your `sync.Pool` implementation by `zeropool.Pool`, and you also get the type-safety for free.

## How does it work?

`zeropool` maintains two `sync.Pool` instances: one is used as the main pool for pointers to the stored items.
The second pool is used to hold the pointers while the code is using the items from the pool.

## Performance

It is approximately ~2x slower than `sync.Pool` if what you are storing are pointers: it doesn't make sense to pay the price in that case.
However, if you have no option but to store elements, and you need to allocate new pointers to store into `sync.Pool`, `zeropool` becomes 2-3x faster.

```
go test -run=X -bench=. -count=10 -benchmem | tee /tmp/zeropool.bench && benchstat -col .name /tmp/zeropool.bench
goos: darwin
goarch: amd64
pkg: github.com/colega/zeropool
cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
     │ ZeropoolPool │            SyncPoolValue            │         SyncPoolNewPointer          │           SyncPoolPointer            │
     │    sec/op    │   sec/op     vs base                │   sec/op     vs base                │    sec/op     vs base                │
*-12    38.28n ± 2%   63.17n ± 0%  +65.00% (p=0.000 n=10)   62.77n ± 2%  +63.97% (p=0.000 n=10)   25.99n ± 38%  -32.13% (p=0.000 n=10)
```

Note that we're talking about nanoseconds here, and if you found this library you were probably more worried about that extra allocation we save:

```
     │ ZeropoolPool │        SyncPoolValue         │      SyncPoolNewPointer      │        SyncPoolPointer        │
     │     B/op     │    B/op     vs base          │    B/op     vs base          │   B/op     vs base            │
*-12      0.00 ± 0%   24.00 ± 0%  ? (p=0.000 n=10)   24.00 ± 0%  ? (p=0.000 n=10)   0.00 ± 0%  ~ (p=1.000 n=10) ¹
¹ all samples are equal

     │ ZeropoolPool │        SyncPoolValue         │      SyncPoolNewPointer      │        SyncPoolPointer         │
     │  allocs/op   │ allocs/op   vs base          │ allocs/op   vs base          │ allocs/op   vs base            │
*-12     0.000 ± 0%   1.000 ± 0%  ? (p=0.000 n=10)   1.000 ± 0%  ? (p=0.000 n=10)   0.000 ± 0%  ~ (p=1.000 n=10) ¹
¹ all samples are equal
```

[^1]: Some smaller types, like scalar values, can be stored in an interface type without allocation, but you wouldn't use a `sync.Pool` for those, right?
[^2]: [SA6002 ignored in Prometheus' head_append.go](https://github.com/prometheus/prometheus/blob/211ae4f1f0a2cdaae09c4c52735f75345c1817c6/tsdb/head_append.go#L206)
[^3]: [SA6002 ignored in Kubernetes' client.go](https://github.com/kubernetes-sigs/metrics-server/blob/c9bc643883fbb438e2e128caab1e3498f1528cfd/pkg/scraper/client/resource/client.go#L95)
[^4]: [GitHub search for SA6002](https://github.com/search?q=SA6002&type=code)
