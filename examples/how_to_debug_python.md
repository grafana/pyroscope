# How to Debug Performance Issues in Python
#### Using Flamgraphs to get to the root of the problem

I know from personal experience that debugging performance issues on Python servers can be incredibly hard. Usually, there was some event like increased traffic or a transient bug that causes end users to report that somethings wrong. 

More often than not, its _impossible_ to exactly replicate the conditions under which the bug occured and so I was stuck trying to figure out which part of our code/infrastructure is responsible for this performance issue on our server.

This article explains how to use Flamegraphs to continuously monitor you code and show you exactly which lines are responsible for these performance issues.

## Why You should care about CPU performance
CPU performance is one of the main indicators used by pretty much every company that runs their software in the cloud (i.e. on AWS, Google Cloud, etc). 

In fact, Netflix performance architect, Brendan Gregg, mentioned that decreasing CPU usage even just 1% is seen as an enormous improvement because of the resource savings that occur at that scale. However, smaller companies also see similar benefits when improving performance, because regardless of size, CPU is often directly correlated with two very important facets of a software business:
1. How much money you're spending on servers - The more CPU resources you need the more it costs to run servers
2. End-user experience - The more load that is placed on your servers CPUs, the slower your website or server becomes 

So, when you see a graph that looks like this, which is what you typically see in a tool like AWS that shows high-level metrics:
![image](https://user-images.githubusercontent.com/23323466/105274459-1a341980-5b52-11eb-9807-cf91351d9bf2.png)

You can, assume that during this period of 100% CPU utilization:
- your end-users are likely having a diminished experience (i.e. App / Website is loading slow) 
- your server costs are going to increase because you need to provision new servers to handle the increased load

But, the main problem is that you don't know _why_ these things or happening. **Which part of the code is responsible?** That's where Flamegraphs come in.

## How to use Flame graphs to debug performance issues and save money
Let's say that this Flamegraph represents the timespan that corresponds with the "incident" where CPU usage spiked in the picture above. What that would indicate is that during this spike, you're servers CPUs were spending:
- 75% of time in `foo()`
- 25% of time in `bar()`

![pyro_python_blog_example_00-01](https://user-images.githubusercontent.com/23323466/105620812-75197b00-5db5-11eb-92af-33e356d9bb42.png)

You can think of a Flamegraph like a super detailed pie chart, where the biggest nodes are taking up most of the CPU resources. 
- The width represents 100% of the time range
- Each node represents a function
- Each node is called by the node above it

In this case, `foo()` is taking up the bulk of the time (75%), so we can look at it improving `foo()` and the functions it calls in order to decrease our CPU usage (and $$).

## Creating a Flamegraph and Table with Pyroscope
To create this example in actual code we'll use Pyroscope - an open-source continuous profiler that was built specifically for the use case of debugging performance issues. To simulate the server doing work, I've created a `work(duration)` function that simply simulates doing work for the duration passed in. This way, we can replicate `foo()` taking 75% of time and `bar()` taking 25% of the time by producing this flamegraph from the code beneath it.

![image](https://user-images.githubusercontent.com/23323466/105621021-f96cfd80-5db7-11eb-8ceb-055ffd4bbcd1.png)


```python
# where each iteration simulates CPU time
def work(n):
    i = 0
    while i < n:
        i += 1

# This would simulate a CPU running for 7.5 seconds
def foo():
    work(75000)

# This would simulate a CPU running for 2.5 seconds
def bar():
    work(25000)
```
Then, let's say you optimize your code to decrease `foo()` time from 75000 to 8000, but left all other portions of the code the same. The new code and  flamegraph would look like:

![image](https://user-images.githubusercontent.com/23323466/105621075-a9db0180-5db8-11eb-9716-a9b643b9ff5e.png)

```python
# where each iteration simulates CPU time
def work(n):
    i = 0
    while i < n:
        i += 1

# This would simulate a CPU running for 0.8 seconds
def a():
    # work(75000)
    work(8000)

# This would simulate a CPU running for 2.5 seconds
def b():
    work(25000)
```

What this means is that your total cpu utilization decreased 66%. If you were paying $100,000 dollars for your servers, you could now manage the same load for $66,000. 

![image](https://user-images.githubusercontent.com/23323466/105621350-659d3080-5dbb-11eb-8a25-bf358458e5ac.png)
