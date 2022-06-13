## Continuous Profiling for Java applications
### Profiling a Java Rideshare App with Pyroscope
![java_example_architecture_new_00](https://user-images.githubusercontent.com/23323466/173369880-da9210af-9a60-4ace-8326-f21edf882575.gif)

Note: For documentation on Pyroscope's java integration visit our website for [java](https://pyroscope.io/docs/java/).

## Background
In this example we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three endpoints found in `main.go`:
- `/bike`    : calls the `orderBike(search_radius)` function to order a bike
- `/car`     : calls the `orderCar(search_radius)` function to order a car
- `/scooter` : calls the `orderScooter(search_radius)` function to order a scooter

We also simulate running 3 distinct servers in 3 different regions (via [docker-compose.yml](https://github.com/pyroscope-io/pyroscope/blob/main/examples/java-jfr/rideshare/docker-compose.yml))
- us-east
- eu-north
- ap-south

One of the most useful capabilities of Pyroscope is the ability to tag your data in a way that is meaningful to you. In this case, we have two natural divisions, and so we "tag" our data to represent those:
- `region`: statically tags the region of the server running the code
- `vehicle`: dynamically tags the endpoint


## Tagging static region
Tagging something static, like the `region`, can be done in the initialization code in the `main()` function:
```
@SpringBootApplication
public class Main {
    public static void main(String[] args) {
        Pyroscope.setStaticLabels(Map.of("REGION", System.getenv("REGION")));
        SpringApplication.run(Main.class, args);
    }
}
```

## Tagging dynamically within functions
Tagging something more dynamically, like we do for the `vehicle` tag can be done inside our utility `OrderService.findNearestVehicle()` function using `pyroscope.LabelsWrapper`
```
Pyroscope.LabelsWrapper.run(new LabelsSet("vehicle", vehicle), () -> {
    [ all code here will be attateched to the "vehicle" label ]
});
```

What this block does, is:
1. Add the label `new LabelsSet("vehicle", vehicle)`
2. execute the code to find the nearest `vehicle`
3. Before the block ends it will (behind the scenes) remove the `LabelsSet("vehicle", vehicle)` from the application since that block is complete

## Resulting flamegraph / performance results from the example
### Running the example
To run the example run the following commands:
```
# Pull latest pyroscope image:
docker pull pyroscope/pyroscope:latest

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
# docker-compose down
```

What this example will do is run all the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. If you select our application: `rideshare.java.push.app.itimer` from the dropdown, you should see a flamegraph that looks like this (below). After we give the flamegraph some time to update and then click the refresh button we see our 3 functions at the bottom of the flamegraph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

## Where's the performance bottleneck?
![1_java_first_slide-01](https://user-images.githubusercontent.com/23323466/173278973-9842ffec-4f18-4419-b155-81823e8ec024.jpg)

The first step when analyzing a profile outputted from your application, is to take note of the _largest node_ which is where your application is spending the most resources. In this case, it happens to be the `orderCar` function. 

The benefit of using the Pyroscope package, is that now that we can investigate further as to _why_ the `orderCar()` function is problematic. Tagging both `region` and `vehicle` allows us to test two good hypotheses:
- Something is wrong with the `/car` endpoint code
- Something is wrong with one of our regions

To analyze this we can select one or more tags from the "Select Tag" dropdown:
<img width="529" alt="Screen Shot 2022-06-12 at 6 52 28 PM" src="https://user-images.githubusercontent.com/23323466/173279005-d87ba766-12c6-461f-a74e-9333bb3e7403.png">

## Narrowing in on the Issue Using Tags
Knowing there is an issue with the `orderCar()` function we automatically select that tag. Then, after inspecting multiple `region` tags, it becomes clear by looking at the timeline that there is an issue with the `eu-north` region, where it alternates between high-cpu times and low-cpu times.

We can also see that the `mutexLock()` function is consuming 76% of CPU resources during this time period. 
![2_java_second_slide-01-01](https://user-images.githubusercontent.com/23323466/173279046-1e67bf51-640c-45b8-9e9a-4db0db1c6709.jpg)


## Comparing two time periods
Using Pyroscope's "comparison view" we can actually select two different sets of tags to compare the resulting flamegraphs. The pink section on the left timeline contains all data where to region is **not equal to** eu-north 
```
REGION != "eu-north"
```
and the blue section on the right contains **only** data where region **is equal to** eu-north 
```
REGION = "eu-north"
```

Not only can we see a differing pattern in CPU utilization on the timeline, but we can also see that the `checkDriverAvailability()` and `mutexLock()` functions are responsible for the majority of this difference.
In the graph where `REGION = "eu-north"`, `checkDriverAvailability()` takes ~92% of CPU while it only takes approximately half that when `REGION != "eu-north"`. 
![3_java_third_slide-01](https://user-images.githubusercontent.com/23323466/173279800-388aa1ae-cf36-4b5f-876e-8641834408ce.jpg)


## Visualizing Diff Between Two Flamegraphs
While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two flamegraphs is better visualized via a diff flamegraph, where red represents cpu time added and green represents cpu time removed. Without changing any parameters, we can simply select the diff view tab and see the difference represented in a color-coded diff flamegraph.
![4_java_fourth_slide-01](https://user-images.githubusercontent.com/23323466/173279888-85c9eead-e3cd-48e6-bf73-204e1074ad2b.jpg)


### Future Roadmap
While this is one popular use case, the ability to add tags opens up many possiblities for other use cases such as linking profiles to other observability signals such as logs, metrics, and traces. 
We've already began to make progress on this with our [otel-pyroscope package](https://github.com/pyroscope-io/otelpyroscope#baseline-diffs) for Go... Stay tuned for a version with Java coming soon! 

We'd love to continue to improve our java integration and so we would love to hear what features _you would like to see_. 
