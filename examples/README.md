# Examples

These are example projects we set up with pyroscope for you to try out. You'll need `docker` + `docker-compose` to run them:

```shell
cd golang
docker-compose up --build
```

These are very simple projects where the application is basically one `while true` loop and inside that loop it calls a slow function and a fast function. Slow function takes about 80% of the time and the fast one takes about 20%.


# How Pyroscope works
Pyroscope is open-source software that allows you to continuously profile your code to debug performance issues down the line of code.
If you haven't used profilers before, then welcome. 
![pyroscope_diagram_no_logo-01](https://user-images.githubusercontent.com/23323466/104868724-1194d680-58f9-11eb-96da-c5a4922a95d5.png)
If you are familiar with profiling / flamegraphs, then you should know that Pyroscope:
- Requires very minimal overhead
- Is designed in a way that **it can store years of your applications perf data down to 10 second granularity.** 
There are two main components that allow Pyroscope to do this:
## Pyroscope agent
The Pyroscope agent wraps around your Python, Ruby, or Go application to poll the stacktrace (100 times per second) to calculate which function is consuming your CPU resources. 
This simply means that 100 times a second Pyroscope records what function is currently using CPU to do some work or calculate something. 
## Pyroscope Server
Pyroscope records and aggregates what your application has been doing, it then sends that data to the Pyroscope server over port `:4040`
Then here, the data is processed, aggregated, and stored ([BadgerDB](https://github.com/dgraph-io/badger)) in a way which allows for you to **quickly** query any time range:
- [x] all of 2020
- [x] that one day that weird thing happened last month
- [x] that time you deployed on a friday and everything went to s*** and nobody knew why
- [x] A random 10 second span anytime ever that you only are querying to see if this is legit
We've been running a demo in production for a while now.  Check out our [Demo Page](https://demo.pyroscope.io/) and select any time range to see how quick it is! 
![image](https://user-images.githubusercontent.com/23323466/104861560-2ebfaa00-58e5-11eb-862e-3481f294cbcf.png)
