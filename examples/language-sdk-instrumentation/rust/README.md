## Continuous Profiling for Rust applications

### Profiling a Rust Rideshare App with Pyroscope

[//]: # (todo)

[//]: # (![golang_example_architecture_new_00]&#40;https://user-images.githubusercontent.com/23323466/173370161-f8ba5c0a-cacf-4b3b-8d84-dd993019c486.gif&#41;)

Note: For documentation on Pyroscope's Rust integration visit our website
for [rust push mode](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/)

## Background

In this example we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three
endpoints found in `main.rs`:

- `/bike`    : calls the `order_bike(search_radius)` function to order a bike
- `/car`     : calls the `order_car(search_radius)` function to order a car
- `/scooter` : calls the `order_scooter(search_radius)` function to order a scooter

We also simulate running 3 distinct servers in 3 different regions (
via [docker-compose.yml](https://github.com/grafana/pyroscope/blob/main/examples/language-sdk-instrumentation/rust/rideshare/docker-compose.yml))

- us-east
- eu-north
- ap-south

One of the most useful capabilities of Pyroscope is the ability to tag your data in a way that is meaningful to you. In
this case, we have two natural divisions, and so we "tag" our data to represent those:

- `region`: statically tags the region of the server running the code
- `vehicle`: dynamically tags the endpoint (similar to how one might tag a controller)

## Tagging static region

Tagging something static, like the `region`, can be done using `PyroscopeAgentBuilder#tags` method in the initialization
code in the `main` function:

```rust
let agent = PyroscopeAgent::builder(server_address, app_name.to_owned())
    .backend(pprof_backend(PprofConfig::new().sample_rate(100)))
    .tags(vec![("region", &region)])
    .build()?;
```

## Tagging dynamically within functions

Tagging something more dynamically, like we do for the `vehicle` tag can be done using `PyroscopeAgent#tag_wrapper`

```rust
let (add_tag, remove_tag) = agent_running.tag_wrapper();
let add = Arc::new(add_tag);
let remove = Arc::new(remove_tag);
let car = warp::path("car").map(move || {
    add("vehicle".to_string(), "car".to_string());
    order_car(3);
    remove("vehicle".to_string(), "car".to_string());
    "Car ordered"
});
```

What this block does, is:

1. Add the label `vehicle=car`
2. Execute the `order_car` function
3. Remove the label `vehicle=car`

## Resulting flame graph / performance results from the example

### Running the example

To run the example run the following commands:

```
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
# docker-compose down
```

What this example will do is run all the code mentioned above and also send some mock-load to the 3 servers as well as
their respective 3 endpoints. If you select our application: `rust-ride-sharing-app` from the dropdown, you should see a
flame graph that looks like this (below). After we give 20-30 seconds for the flame graph to update and then click the
refresh button we see our 3 functions at the bottom of the flame graph taking CPU resources _proportional to the size_
of their respective `search_radius` parameters.

## Where's the performance bottleneck?

![golang_first_slide](https://user-images.githubusercontent.com/23323466/149688998-ca94dc82-f1e5-46fd-9a73-233c1e56d8e5.jpg)

The first step when analyzing a profile outputted from your application, is to take note of the _largest node_ which is
where your application is spending the most resources. In this case, it happens to be the `order_car` function.

The benefit of using the Pyroscope package, is that now that we can investigate further as to _why_ the `order_car()`
function is problematic. Tagging both `region` and `vehicle` allows us to test two good hypotheses:

- Something is wrong with the `/car` endpoint code
- Something is wrong with one of our regions

To analyze this we can select one or more tags from the "Select Tag" dropdown:

![image](https://user-images.githubusercontent.com/23323466/135525308-b81e87b0-6ffb-4ef0-a6bf-3338483d0fc4.png)

## Narrowing in on the Issue Using Tags

Knowing there is an issue with the `order_car` function we automatically select that tag. Then, after inspecting
multiple `region` tags, it becomes clear by looking at the timeline that there is an issue with the `eu-north` region,
where it alternates between high-cpu times and low-cpu times.

We can also see that the `mutex_lock()` function is consuming almost 70% of CPU resources during this time period.

![golang_second_slide-01](https://user-images.githubusercontent.com/23323466/149689013-2c0afeeb-53e2-4780-b52a-26b140627d9c.jpg)

## Comparing two time periods

Using Pyroscope's "comparison view" we can actually select two different time ranges from the timeline to compare the
resulting flame graphs. The pink section on the left timeline results in the left flame graph, and the blue section on
the right represents the right flame graph.

When we select a period of low-cpu utilization and a period of high-cpu utilization we can see that there is clearly
different behavior in the `mutex_lock()` function where it takes **33% of CPU** during low-cpu times and **71% of CPU**
during high-cpu times.

![golang_third_slide-01](https://user-images.githubusercontent.com/23323466/149689026-8b4ab3b1-6380-455c-990f-7ff35811f26b.jpg)

## Visualizing Diff Between Two Flame graphs

While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two
flame graphs is better visualized with them overlayed over each other. Without changing any parameters, we can simply
select the diff view tab and see the difference represented in a color-coded diff flame graph.

![golang_fourth_slide-01](https://user-images.githubusercontent.com/23323466/149689038-50d12031-2879-470f-a3be-a4c71d8c3b7a.jpg)

### More use cases

We have been beta testing this feature with several different companies and some of the ways that we've seen companies
tag their performance data:

- Tagging Kubernetes attributes
- Tagging controllers
- Tagging regions
- Tagging jobs from a queue
- Tagging commits
- Tagging staging / production environments
- Tagging different parts of their testing suites
- Etc...

### Future Roadmap

We would love for you to try out this example and see what ways you can adapt this to your Rust application. Continuous
profiling has become an increasingly popular tool for the monitoring and debugging of performance issues (arguably the
fourth pillar of observability).

We'd love to continue to improve our RUst integrations, and so we would love to hear what features _you would like to
see_.
