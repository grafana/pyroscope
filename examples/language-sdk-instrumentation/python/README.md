## Continuous Profiling for Python applications

### Profiling a Python Rideshare App with Pyroscope

![python_example_architecture_new_00](https://user-images.githubusercontent.com/23323466/173369382-267af200-6126-4bd0-8607-a933e8400dbb.gif)

#### _Read this in other languages._

<kbd>[简体中文](README_zh.md)</kbd>

Note: For documentation on the Pyroscope pip package visit [our website](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/)

## Interactive Tutorial

Explore our [interactive Ride Share tutorial](https://killercoda.com/grafana-labs/course/pyroscope/ride-share-tutorial) on KillerCoda, where you can learn how to use Pyroscope by profiling a "Ride Share" application.

## Live Demo

Feel free to check out the [live demo](https://play.grafana.org/a/grafana-pyroscope-app/profiles-explorer?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=flame-graph&var-serviceName=pyroscope-rideshare-python&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-dataSource=grafanacloud-profiles) of this example on our demo page.

## Background

In this example we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three endpoints found in `server.py`:

- `/bike`    : calls the `order_bike(search_radius)` function to order a bike
- `/car`     : calls the `order_car(search_radius)` function to order a car
- `/scooter` : calls the `order_scooter(search_radius)` function to order a scooter

We also simulate running 3 distinct servers in 3 different regions:

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

## Resulting flame graph / performance results from the example

### Running the example

Try out one of the Django, Flask, or FastAPI examples located in the `rideshare` directory by running the following commands:

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker compose up --build

# Reset the database (if needed):
docker compose down
```

What this example will do is run all the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. If you select our application: `ride-sharing-app` from the dropdown, you should see a flame graph that looks like this (below). After we give 20-30 seconds for the flame graph to update and then click the refresh button we see our 3 functions at the bottom of the flame graph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

## Where's the performance bottleneck?

![python_slide_1](https://github.com/user-attachments/assets/1d38ddbf-2a9e-4f07-8d70-343cff878307)

The first step when analyzing a profile outputted from your application, is to take note of the _largest node_ which is where your application is spending the most resources. In this case, it happens to be the `order_car` function.

The benefit of using the Pyroscope package, is that now that we can investigate further as to _why_ the `order_car` function is problematic. Tagging both `region` and `vehicle` allows us to test two good hypotheses:
- Something is wrong with the `/car` endpoint code
- Something is wrong with one of our regions

To analyze this we can select one or more labels on the "Labels" page:

![python_slide_2](https://github.com/user-attachments/assets/5a8ee6ed-d2e1-42f3-98f3-d977adfccd08)

## Narrowing in on the Issue Using Labels

Knowing there is an issue with the `order_car` function we automatically select that tag. Then, after inspecting multiple `region` tags, it becomes clear by looking at the timeline that there is an issue with the `eu-north` region, where it alternates between high-cpu times and low-cpu times.

We can also see that the `find_nearest_vehicle` function is consuming almost 70% of CPU resources during this time period.

![python_slide_3](https://github.com/user-attachments/assets/57614064-bced-4363-bdba-b028c132e1e9)

## Visualizing diff between two flame graphs

While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two flame graphs is better visualized with them overlayed over each other. Without changing any parameters, we can simply select the diff view tab and see the difference represented in a color-coded diff flame graph.

![python_slide_4](https://github.com/user-attachments/assets/9c89458f-f7fb-4561-80a2-6a86c7c2ed4c)

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

### Future Roadmap

We would love for you to try out this example and see what ways you can adapt this to your python application. Continuous profiling has become an increasingly popular tool for the monitoring and debugging of performance issues (arguably the fourth pillar of observability).

We'd love to continue to improve this pip package by adding things like integrations with popular tools, memory profiling, etc. and we would love to hear what features _you would like to see_.
