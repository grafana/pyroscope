## Continuous Profiling for Ruby applications

### Profiling a Ruby Rideshare App with Pyroscope

![ruby_example_architecture_new_00](https://user-images.githubusercontent.com/23323466/173369670-ba6fe5ce-eab0-4824-94dd-c72255efc063.gif)

Note: For documentation on the Pyroscope ruby gem visit [our website](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/).

## Live Demo

Feel free to check out the [live demo](https://play.grafana.org/a/grafana-pyroscope-app/profiles-explorer?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=flame-graph&var-serviceName=pyroscope-rideshare-ruby&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-dataSource=grafanacloud-profiles) of this example on our demo page.

## Background

In this example we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three endpoints found in `server.rb`:

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

```ruby
Pyroscope.configure do |config|
  config.app_name = "ride-sharing-app"
  config.server_address = "http://pyroscope:4040"
  config.tags = {
    "region": ENV["REGION"],                     # Tags the region based of the environment variable
  }
end
```

## Tagging dynamically within functions

Tagging something more dynamically, like we do for the `vehicle` tag can be done inside our utility `find_nearest_vehicle()` function using a `Pyroscope.tag_wrapper` block

```ruby
def find_nearest_vehicle(n, vehicle)
  Pyroscope.tag_wrapper({ "vehicle" => vehicle }) do
    ...code to find nearest vehicle
  end
end
```

What this block does, is:

1. Add the tag `{ "vehicle" => "car" }`
2. execute the `find_nearest_vehicle()` function
3. Before the block ends it will (behind the scenes) remove the `{ "vehicle" => "car" }` from the application since that block is complete

## Resulting flame graph / performance results from the example

### Running the example

To run the example run the following commands in the `rideshare` directory:

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker compose up --build

# Reset the database (if needed):
docker compose down
```

The Rails version of the example is available in the `raidshare_rails` directory.

What this example will do is run all the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. If you select our application: `ride-sharing-app` from the dropdown, you should see a flame graph that looks like this. After we give 20-30 seconds for the flame graph to update and then click the refresh button we see our 3 functions at the bottom of the flame graph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

## Where's the performance bottleneck?

![ruby_slide_1](https://github.com/user-attachments/assets/d1304b3d-f2a0-4bce-88cf-9113ecdd1a14)

The first step when analyzing a profile outputted from your application, is to take note of the _largest node_ which is where your application is spending the most resources. In this case, it happens to be the `order_car` function.

The benefit of using the Pyroscope package, is that now that we can investigate further as to _why_ the `order_car()` function is problematic. Tagging both `region` and `vehicle` allows us to test two good hypotheses:

- Something is wrong with the `/car` endpoint code
- Something is wrong with one of our regions

To analyze this we can select one or more tags from the "Select Tag" dropdown:

![ruby_slide_2](https://github.com/user-attachments/assets/e3d44542-a953-4419-a67e-a61214de5396)

## Narrowing in on the Issue Using Tags

Knowing there is an issue with the `order_car()` function we automatically select that tag. Then, after inspecting multiple `region` tags, it becomes clear by looking at the timeline that there is an issue with the `eu-north` region, where it alternates between high-cpu times and low-cpu times.

We can also see that the `mutex_lock()` function is consuming almost 70% of CPU resources during this time period.

![ruby_slide_3](https://github.com/user-attachments/assets/3eef7cfb-c008-4443-a2d1-ca58d8ce2421)

## Visualizing diff between two flame graphs

While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two flame graphs is better visualized with them overlayed over each other. Without changing any parameters, we can simply select the diff view tab and see the difference represented in a color-coded diff flame graph.

![ruby_slide_4](https://github.com/user-attachments/assets/33857a48-3942-429d-9abf-63f4619ad605)

### More use cases

We have been beta testing this feature with several different companies and some of the ways that we've seen companies tag their performance data:
- Linking profiles with trace data
- Tagging controllers
- Tagging regions
- Tagging jobs from a redis or sidekiq queue
- Tagging commits
- Tagging staging / production environments
- Tagging different parts of their testing suites
- Etc...

### Future Roadmap

We would love for you to try out this example and see what ways you can adapt this to your ruby application. Continuous profiling has become an increasingly popular tool for the monitoring and debugging of performance issues (arguably the fourth pillar of observability).

We'd love to continue to improve this gem by adding things like integrations with popular tools, memory profiling, etc. and we would love to hear what features _you would like to see_.
