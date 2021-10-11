# Examples
Choose a language folder to select an example for your language of choice

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
![pyroscope_diagram_with_logo](../.github/markdown-images/deployment.svg)
