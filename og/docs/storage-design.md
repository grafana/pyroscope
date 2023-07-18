![image](https://user-images.githubusercontent.com/23323466/110414341-8ad0c000-8044-11eb-9628-7b24e50295b2.png)

# O(log n) makes continuous profiling possible

#### _Read this in other languages._
<kbd>[<img title="中文 (Simplified)" alt="中文 (Simplified)" src="https://cdn.staticaly.com/gh/hjnilsson/country-flags/master/svg/cn.svg" width="22">](storage-design-ch.md)</kbd>

Pyroscope is software that lets you **continuously** profile your code to debug performance issues down to a line of code. With just a few lines of code it will do the following:

### Pyroscope Agent
- Polls the stack trace every 0.01 seconds to see which functions are consuming resources
- Batches that data into 10s blocks and sends it to Pyroscope server

### Pyroscope Server
- Receives data from the Pyroscope agent and processes it to be stored efficiently
- Pre-aggregates profiling data for fast querying when data needs to be retrieved

## Storage Efficiency

The challenge with continuous profiling is that if you just take frequent chunks of profiling data, compress it, and store it somewhere, it becomes:
1. Too much data to store efficiently
2. Too much data to query quickly

We solve these problems by:
1. Using a combination of tries and trees to compress data efficiently
2. Using segment trees to return queries for any timespan of data in O(log n) vs O(n) time complexity

## Step 1: Turning the profiling data into a tree

The simplest way to represent profiling data is in a list of string each one representing a stack trace and a number of times this particular stack trace was seen during a profiling session:

```bash
server.py;fast_function;work 2
server.py;slow_function;work 8
```

The first obvious thing we do is we turn this data into a tree. Conveniently, this representation also makes it easy to later generate flamegraphs.

![raw_vs_flame_graph](https://user-images.githubusercontent.com/23323466/110378930-0f065180-800b-11eb-9357-71724bc7258c.gif)

Compressing the stack traces into trees saves space on repeated elements. By using trees, we go from having to store common paths like `net/http.request` in the db multiple times to only having to store it 1 time and saving a reference to the location at which it's located. This is fairly standard with profiling libraries since its the lowest hanging fruit when it comes to optimizing storage with profiling data.

![fast-compress-stack-traces](https://user-images.githubusercontent.com/23323466/110227218-e109fb80-7eaa-11eb-81a8-cdf2b3944f1c.gif)

## Step 2: Adding tries to store individual symbols more efficiently

So now that we've compressed the raw profiling data by converting into a tree, many of the nodes in this compressed tree contain symbols that also share repeated elements with other nodes. For example:

```
net/http.request;net/io.read 100 samples
net/http.request;net/io.write 200 samples
```

While the `net/http.request`, `net/io.read`, and `net/io.write` functions differ they share the same common ancestor of `net/`.

Each of these lines can be serialized using a prefix tree as follows. This means that instead of storing the same prefixes multiple times, we can now just store them once in a trie and access them by storing a pointer to their position in memory:

![storage-design-0](https://user-images.githubusercontent.com/23323466/110520399-446e7600-80c3-11eb-84e9-ecac7c0dbf23.gif)

In this basic example we save ~80% of space going from 39 bytes to 8 bytes. Typically, symbol names are much longer and as the number of symbols grows, storage requirements grow logarithmically rather than linearly.

## Step 1 + 2: Combining the trees with the tries

In the end, by using a tree to compress the raw profiling data and then using tries to compress the symbols we get the following storage amounts for our simple example:

```
| data type           | bytes |
|---------------------|-------|
| raw data            | 93    |
| tree                | 58    |
| tree + trie         | 10    |
```

As you can see this is a 9x improvement for a fairly trivial case. In real world scenarios the compression factor gets much larger.

![combine-segment-and-prefix_1](https://user-images.githubusercontent.com/23323466/110262208-ca75aa00-7f67-11eb-8f16-0572a4641ee1.gif)

## Step 3: Optimizing for fast reads using Segment Trees

Now that we have a way of storing the data efficiently the next problem that arises is how do we query it efficiently. The way we solve this problem is by pre-aggregating the profiling data and storing it in a special segment tree.

Every 10s Pyroscope agent sends a chunk of profiling data to the server which writes the data into the db with the corresponding timestamp. You'll notice that each write happens once, but is replicated multiple times.

**Each layer represents a time block of larger units so in this case for every two 10s time blocks, one 20s time block is created. This is to make reading the data more efficient (more on that in a second)**.

![segment_tree_animation_1](https://user-images.githubusercontent.com/23323466/110259555-196a1200-7f5d-11eb-9223-218bb4b34c6b.gif)

## Turn reads from O(n) to O(log n)

If you don't use segment trees and just write data in 10 second chunks the time complexity for the reads becomes a function of how many 10s units the query asks for. If you want 1 year of data, you'll have to then merge 3,154,000 trees representing the profiling data. By using segment trees you can effectively decrease the amount of merge operations from O(n) to O(log n).

![segment_tree_reads](https://user-images.githubusercontent.com/23323466/110277713-b98a6000-7f8a-11eb-942f-3a924a6e0b09.gif)


## Help us add more profilers

We spent a lot of time on solving this storage / querying problem because we wanted to make software that can do truly continuous profiling in production without causing too much overhead.

While Pyroscope currently supports 4 languages, we would love to add more.

Any sampling profiler that can export data in the "raw" format linked above can become a Profiling agent with Pyroscope. We'd love your help building out profilers for other languages!

- [x] [Go](https://pyroscope.io/docs/golang)
- [x] [Python](https://pyroscope.io/docs/python)
- [x] [eBPF](https://pyroscope.io/docs/ebpf)
- [x] [Ruby](https://pyroscope.io/docs/ruby)
- [x] [PHP](https://pyroscope.io/docs/php)
- [x] [Java](https://pyroscope.io/docs/java)
- [x] [.NET](https://pyroscope.io/docs/dotnet)
- [ ] [Rust](https://github.com/pyroscope-io/pyroscope/issues/83#issuecomment-784947654)
- [ ] [Node](https://github.com/pyroscope-io/pyroscope/issues/8)

If you want to help contribute or need help setting up Pyroscope here's how you can reach us:
- Join our [Slack](https://pyroscope.io/slack)
- Set up a time to meet with us [here](https://pyroscope.io/setup-call)
- Write an [issue](https://github.com/pyroscope-io/pyroscope/issues)
- Follow us on [Twitter](https://twitter.com/PyroscopeIO)
