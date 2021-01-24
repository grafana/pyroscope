# Examples

We set up these examples to help you try out Pyroscope. You'll need `docker` + `docker-compose` to run them:

```shell
cd python
docker-compose up --build
```

These are very simple projects where the application is basically one `while true` loop and inside that loop it calls a slow function and a fast function. Slow function takes about 75% of the time and the fast one takes about 25%. See [how_to_debug_python.md](https://github.com/pyroscope-io/pyroscope/blob/main/examples/how_to_debug_python.md) for a full example of how improving one function can decrease overall CPU utilization and ultimately save cut server costs by 66%!


# How Pyroscope works
Pyroscope identifies performance issues in your application by continuously profiling the code.

If you've never used a profiler before, then welcome! 

If you are familiar with profiling and flame graphs, then you'll be happy to know that Pyroscope:
- Requires very minimal overhead
- Can store years of perf data down to 10 second granularity 
- Uses a unique, inverted flame graph for increased readability

There are two main components that allow Pyroscope to run smoothly and quickly:
## Pyroscope agent
Every .01 seconds, the Pyroscope agent wraps around your Python, Ruby, or Go application to poll the stacktrace and calculate which function is consuming your CPU resources. 
![pyroscope_diagram_no_logo-01](https://user-images.githubusercontent.com/23323466/104868724-1194d680-58f9-11eb-96da-c5a4922a95d5.png)
## Pyroscope Server
Pyroscope records and aggregates what your application has been doing, then sends that data to the Pyroscope server over port `:4040`([BadgerDB](https://github.com/dgraph-io/badger)) to be processed, aggregated, and stored  for speedy queries of any time range, including:
- [x] all of 2020
- [x] that one day last month when that weird thing happened
- [x] that time you deployed on a Friday and messed up everything without knowing why
- [x] A random 10 seconds you are only querying to see if Pyroscope is legit

Check out our [Demo Page](https://demo.pyroscope.io/) and select any time range to see how quickly Pyroscope works! 
![image](https://user-images.githubusercontent.com/23323466/104861560-2ebfaa00-58e5-11eb-862e-3481f294cbcf.png)
