---
headless: true
description: Shared file for profile types.
---

[//]: # 'Profile types descriptions.'
[//]: # 'This shared file is included in these locations:'
[//]: # '/pyroscope/docs/sources/introduction/profiling-types.md'
[//]: # '/website/content/grafana-cloud/monitor-applications/profiles/introduction/profiling-types.md'
[//]: #
[//]: # 'If you make changes to this file, verify that the meaning and content are not changed in any place where the file is included.'
[//]: # 'Any links should be fully qualified and not relative: /docs/grafana/ instead of ../grafana/.'

<!-- Profile type descriptions -->

## CPU profiling

CPU profiling measures the amount of CPU time consumed by different parts of your application code.
High CPU usage can indicate inefficient code, leading to poor performance and increased operational costs.
It's used to identify and optimize CPU-intensive functions in your application.

- **When to use**: To identify and optimize CPU-intensive functions
- **Flame graph insight**: The width of blocks indicates the CPU time consumed by each function

The UI shows a spike in CPU along with the flame graph associated with that spike.
You may get similar insights from metrics, however, with profiling, you have more details into the specific cause of a spike in CPU usage at the line level.

![Example flame graph](https://grafana.com/static/img/pyroscope/pyroscope-ui-single-2023-11-30.png)

## Memory allocation profiling

Memory allocation profiling tracks the amount and frequency of memory allocations by the application.
Excessive or inefficient memory allocation can lead to memory leaks and high garbage collection overhead, impacting application efficiency.

<!-- vale Grafana.Spelling = NO -->
- **Types**: Alloc Objects, Alloc Space
<!-- vale Grafana.Spelling = YES -->
- **When to use**: For identifying and optimizing memory usage patterns
- **Flame graph insight**: Highlights functions where memory allocation is high

The timeline shows memory allocations over time and is great for debugging memory related issues.
A common example is when a memory leak is created due to improper handling of memory in a function.
This can be identified by looking at the timeline and seeing a gradual increase in memory allocations that never goes down.
This is a clear indicator of a memory leak.

![memory leak example](https://grafana.com/static/img/pyroscope/pyroscope-memory-leak-2023-11-30.png)

Without profiling, this may be something that's exhibited in metrics or out-of-memory errors (OOM) logs but with profiling you have more details into the specific function that's allocating the memory which is causing the leak at the line level.

## Goroutine profiling

Goroutines are lightweight threads in Go, used for concurrent operations.
Goroutine profiling measures the usage and performance of these threads.
Poor management can lead to issues like deadlocks and excessive resource usage.

- **When to use**: Especially useful in Go applications for concurrency management
- **Flame graph insight**: Provides a view of goroutine distribution and issues

## Mutex profiling

Mutex profiling involves analyzing mutex (mutual exclusion) locks, used to prevent simultaneous access to shared resources.
Excessive or long-duration mutex locks can cause delays and reduced application throughput.

- **Types**: Mutex Count, Mutex Duration
- **When to use**: To optimize thread synchronization and reduce lock contention
- **Flame graph insight**: Shows frequency and duration of mutex operations

## Block profiling

Block profiling measures the frequency and duration of blocking operations, where a thread is paused or delayed.
Blocking can significantly slow down application processes, leading to performance bottlenecks.

- **Types**: Block Count, Block Duration
- **When to use**: To identify and reduce blocking delays
- **Flame graph insight**: Identifies where and how long threads are blocked
