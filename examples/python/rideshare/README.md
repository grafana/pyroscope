## Continuous Profiling for Python applications

### Profiling a Python Rideshare App with Pyroscope

![python_example_architecture_new_00](https://user-images.githubusercontent.com/23323466/173369382-267af200-6126-4bd0-8607-a933e8400dbb.gif)

#### _Read this in other languages._

<kbd>[简体中文](README_zh.md)</kbd>

Note: For documentation on the Pyroscope pip package visit [our website](https://pyroscope.io/docs/python/)

## Background

In this example we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three endpoints found in `server.py`:

- `/bike`    : calls the `order_bike(search_radius)` function to order a bike
- `/car`     : calls the `order_car(search_radius)` function to order a car
- `/scooter` : calls the `order_scooter(search_radius)` function to order a scooter

We also simulate running 3 distinct servers in 3 different regions (via [docker-compose.yml](https://github.com/pyroscope-io/pyroscope/blob/main/examples/python/docker-compose.yml))

- us-east
- eu-north
- ap-south

One of the most useful capabilities of Pyroscope is the ability to tag your data in a way that is meaningful to you. In this case, we have two natural divisions, and so we "tag" our data to represent those:

- `region`: statically tags the region of the server running the code
- `vehicle`: dynamically tags the endpoint (similar to how one might tag a controller rails)

## Tagging static region

Tagging something static, like the `region`, can be done in the initialization code in the `config.tags` variable:

```python
pyroscope.configure(
    application_name       = "ride-sharing-app",
    server_address         = "http://pyroscope:4040",
    tags                   = {
        "region":   f'{os.getenv("REGION")}', # Tags the region based off the environment variable
    }
)
```

## Tagging dynamically within functions

Tagging something more dynamically, like we do for the `vehicle` tag can be done inside our utility `find_nearest_vehicle()` function using a `with pyroscope.tag_wrapper()` block

```python
def find_nearest_vehicle(n, vehicle):
    with pyroscope.tag_wrapper({ "vehicle": vehicle}):
        i = 0
        start_time = time.time()
        while time.time() - start_time < n:
            i += 1
```

What this block does, is:

1. Add the tag `{ "vehicle" => "car" }`
2. execute the `find_nearest_vehicle()` function
3. Before the block ends it will (behind the scenes) remove the `{ "vehicle" => "car" }` from the application since that block is complete

## Resulting flamegraph / performance results from the example

### Running the example

To run the example run the following commands:

```shell
# Pull latest pyroscope image:
docker pull grafana/pyroscope:latest

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
# docker-compose down
```

What this example will do is run all the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. If you select our application: `ride-sharing-app.cpu` from the dropdown, you should see a flamegraph that looks like this (below). After we give 20-30 seconds for the flamegraph to update and then click the refresh button we see our 3 functions at the bottom of the flamegraph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

## Where's the performance bottleneck?

Profiling is most effective for applications that contain tags. The first step when analyzing performance from your application, is to use the Tag Explorer page in order to determine if any tags are consuming more resources than others.

![vehicle_tag_breakdown](https://user-images.githubusercontent.com/23323466/191306637-a601f463-a247-4588-a285-639424a08b87.png)

![image](https://user-images.githubusercontent.com/23323466/191319887-8fff2605-dc74-48ba-b0b7-918e3c95ed91.png)

The benefit of using Pyroscope, is that by tagging both `region` and `vehicle` and looking at the Tag Explorer page we can hypothesize:

- Something is wrong with the `/car` endpoint code where `car` vehicle tag is consuming **68% of CPU**
- Something is wrong with one of our regions where `eu-north` region tag is consuming **54% of CPU**

From the flamegraph we can see that for the `eu-north` tag the biggest performance impact comes from the `find_nearest_vehicle()` function which consumes close to **68% of cpu**. To analyze this we can go directly to the comparison page using the comparison dropdown.

## Comparing two time periods

Using Pyroscope's "comparison view" we can actually select two different queries and compare the resulting flamegraphs:
- Left flamegraph: `{ region != "eu-north", ... }`
- Right flamegraph: `{ region = "eu-north", ... }`

When we select a period of low-cpu utilization and a period of high-cpu utilization we can see that there is clearly different behavior in the `find_nearest_vehicle()` function where it takes:
- Left flamegraph: **22% of CPU** when `{ region != "eu-north", ... }`
- right flamgraph: **82% of CPU** when `{ region = "eu-north", ... }`

![python_pop_out_library_comparison_00](https://user-images.githubusercontent.com/23323466/191374975-d374db02-4cb1-48d5-bc1a-6194193a9f09.png)

## Visualizing Diff Between Two Flamegraphs

While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two flamegraphs is better visualized with them overlayed over each other. Without changing any parameters, we can simply select the diff view tab and see the difference represented in a color-coded diff flamegraph.
![find_nearest_vehicle_diff](https://user-images.githubusercontent.com/23323466/191320888-b49eb7de-06d5-4e6b-b9ac-198d7c9e2fcf.png)


### More use cases

We have been beta testing this feature with several different companies and some of the ways that we've seen companies tag their performance data:
- Linking profiles with trace data
- Tagging controllers
- Tagging regions
- Tagging jobs from a redis / sidekiq / rabbitmq queue
- Tagging commits
- Tagging staging / production environments
- Tagging different parts of their testing suites
- Etc...

### Live Demo

Feel free to check out the [live demo](https://demo.pyroscope.io/explore?query=rideshare-app-python.cpu%7B%7D&groupBy=region&groupByValue=All) of this example on our demo page.

### Future Roadmap

We would love for you to try out this example and see what ways you can adapt this to your python application. Continuous profiling has become an increasingly popular tool for the monitoring and debugging of performance issues (arguably the fourth pillar of observability).

We'd love to continue to improve this pip package by adding things like integrations with popular tools, memory profiling, etc. and we would love to hear what features _you would like to see_.
