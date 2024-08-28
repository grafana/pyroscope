---
title: Pyroscope memory overhead
menuTitle: Memory overhead
description: Learn about memory overhead for the Pyroscope client.
weight: 200
---

# Pyroscope memory overhead

Pyroscope has very low memory overhead, usually less than 50 MB per pod.

## How the profiler works

Stacktraces are captured at regular intervals (~100Hz).
Memory allocations and lock contention events are sampled.

These stacktraces and memory allocation events are temporarily stored in memory.

The stored profiling data is periodically (default is every 15 seconds) sent to the server.

Memory overhead consists of:

* The temporary storage of profiles in memory is the primary contributor to memory overhead.
* Memory usage typically scales up sublinearly with the number of CPUs.

## What happens if the Pyroscope backend is down?

The guiding principle of Pyroscope clients is to never cause the user application to crash.

Profiles are uploaded using multiple threads. The default value is `5` and can be adjusted using the `Threads` variable.

If the backend is down or slow, the profiler discards new profiles to prevent running out of memory.

## Real-world example

The exact overhead can vary based on the application, so direct testing is recommended.

At Grafana Labs we continuously profile all of our workloads.
With all profiling types enabled (CPU, alloc, goroutine, mutex, block), the observed memory overhead per pod is typically less than 50 MB.

The overhead is often so minimal that it becomes challenging to accurately measure.