## Continuous Profiling for Rust applications

### Profiling a Rust Rideshare App with Pyroscope

![Image](https://github.com/user-attachments/assets/9a6e50a1-b8df-4923-9632-79ace3fea216)

> [!NOTE]  
> For documentation on Pyroscope's Rust integration, refer to the [Rust push mode](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/) documentation.

## Background

This example shows a simplified, basic use case of Pyroscope that uses a "ride share" company which has three
endpoints found in `main.rs`:

- `/bike`    : calls the `order_bike(search_radius)` function to order a bike
- `/car`     : calls the `order_car(search_radius)` function to order a car
- `/scooter` : calls the `order_scooter(search_radius)` function to order a scooter

The example also simulates running 3 distinct servers in 3 different regions (
via [docker-compose.yml](https://github.com/grafana/pyroscope/blob/main/examples/language-sdk-instrumentation/rust/rideshare/docker-compose.yml)):

- us-east
- eu-north
- ap-south

Pyroscope lets you tag your data in a way that is meaningful to you. In
this case, there are two natural divisions, and so data is "tagged" to represent them:

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

Tagging something more dynamically can be done using `PyroscopeAgent#tag_wrapper`. For example, you'd use code like this for the `vehicle` tag:

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

This block does the following:

1. Add the label `vehicle=car`
2. Execute the `order_car` function
3. Remove the label `vehicle=car`

## Resulting flame graph / performance results from the example

### Running the example

To run the example, use the following commands:

```
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
# docker-compose down
```

This example runs all the code mentioned above and also sends some mock-load to the 3 servers as well as
their respective 3 endpoints. If you select `rust-ride-sharing-app` from the dropdown, you should see a
flame graph that looks like this (below). Wait 20-30 seconds for the flame graph to update, and then click the
refresh button to see 3 functions at the bottom of the flame graph taking CPU `resources _proportional` to the `size_`
of their respective `search_radius` parameters.

[//]: # (http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=flame-graph&var-serviceName=rust-ride-sharing-app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-dataSource=local-pyroscope&var-groupBy=all&var-filters=&maxNodes=16384&from=now-5m&to=now&var-filtersBaseline=&var-filtersComparison=)

## Where's the performance bottleneck?

![Image](https://github.com/user-attachments/assets/d4b0f85d-cc8d-4058-b019-1c5198849676)

To analyze a profile outputted from your application, take note of the _largest node_ which is
where your application is spending the most resources. In this case, it happens to be the `order_car` function.

ThePyroscope package lets you investigate further as to _why_ the `order_car()`
function is problematic. Tagging both `region` and `vehicle` allows us to test two good hypotheses:

- Something is wrong with the `/car` endpoint code
- Something is wrong with one of our regions

To analyze this, select one or more tags on the "Labels" page:

![Image](https://github.com/user-attachments/assets/3e5cb3ac-609e-493a-ae4d-248de150a33b)

## Narrowing in on the Issue Using Tags

Since you know there is an issue with the `order_car` function,  select that tag. After inspecting
multiple `region` tags, the timeline shows that there is an issue with the `eu-north` region,
where it alternates between high-cpu times and low-cpu times.

Note that the `mutex_lock()` function is consuming almost 70% of CPU resources during this time period.

![Image](https://github.com/user-attachments/assets/12fc0912-8b65-4c24-9284-b0aa1eef45ba)

## Visualizing Diff Between Two Flame graphs

While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two
flame graphs is better visualized with them overlayed over each other. Without changing any parameters, you can
select the diff view tab and see the difference represented in a color-coded diff flame graph.

![Image](https://github.com/user-attachments/assets/97f6e51c-4211-4a0a-8f11-d2ee0402e396)

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

We'd love to continue to improve our Rust integrations, and so we would love to hear what features _you would like to
see_.
