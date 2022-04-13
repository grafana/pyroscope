## Continuous Profiling for Golang applications
### Profiling a Golang Rideshare App with Pyroscope
![golang_example_architecture_05_1](https://user-images.githubusercontent.com/23323466/149699777-8f73c459-917d-48c5-b2d8-87560a7bfb11.gif)

Note: For documentation on Pyroscope's golang integration visit our website for [golang push mode](https://pyroscope.io/docs/golang/) or [golang pull mode](https://pyroscope.io/docs/golang-pull-mode/)
## Prerequisites
We expect the reader to have a basic understanding of continuous profiling, and have docker set up locally.

## Background
In this example, we show a simplified, basic use case of Pyroscope. Let’s say we created a "ride sharing" company and named the app Rideshare. Rideshare is a ride-sharing application built using the Go programming language (Golang). The app has three endpoints found in `main.go`:
- `/bike`    : calls the `OrderBike(search_radius)` function to order a bike
- `/car`     : calls the `OrderCar(search_radius)` function to order a car
- `/scooter` : calls the `OrderScooter(search_radius)` function to order a scooter

Because we want to ensure the availability of our services, we decide to host our application on the cloud. So currently, Rideshare is running on servers in 3 different regions: 
- us-east-1
- us-west-1
- eu-west-1

Fast forward to 3 months later, our daily user metrics increase. We begin to get a lot of reports from our users: the app is slow, takes like forever to order a ride most especially cars, nearly impossible to find a vehicle nearby. Users are frustrated, investors are panicking, and engineers are in chaos mode.  

Now, to fix the underlying performance issue, we need to view the application profiles to analyze our code in order to track the issue down to its origin. We would also want to go back in time to know exactly when the issue originated. And how do we do that? **Pyroscope** to the rescue!

## Integrating Pyroscope into a Golang application
To integrate Pyroscope into the Rideshare app, we have to install the Pyroscope Golang client package into our codebase: 
```
$ go get github.com/Pyroscope-io/client/Pyroscope
```

The **go-get tool** will find the package, on GitHub in this case, and install it into your **$GOPATH**.
For this example, the code would be installed in this directory:
```
$GOPATH/src/github.com/Pyroscope-io/client/Pyroscope
```

Once installed, we go ahead to import Pyroscope into our `main.go` file:
```
Import ("github.com/Pyroscope-io/client/Pyroscope")
```

## Tagging static region
One of the most useful capabilities of Pyroscope is the ability to tag your data in a way that is meaningful to you. In this case, we have two natural divisions, and so we "tag" our data to represent those:
- `region`: statically tags the region of the server running the code
- `vehicle`: dynamically tags the endpoint (similar to how one might tag a controller).

Tagging something static, like the `region`, can be done in the initialization code in the `main()` function:
```
	pyroscope.Start(pyroscope.Config{
		ApplicationName: "ride-sharing-app",
		ServerAddress:   serverAddress,
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"region": os.Getenv("REGION")},
	})
```

## Tagging dynamically within functions
Tagging something more dynamically, like we do for the `vehicle` tag can be done inside our utility `FindNearestVehicle()` function using `pyroscope.TagWrapper`
```
func FindNearestVehicle(search_radius int64, vehicle string) {
	pyroscope.TagWrapper(context.Background(), pyroscope.Labels("vehicle", vehicle), func(ctx context.Context) {
       
        // Mock "doing work" to find a vehicle
        var i int64 = 0
		start_time := time.Now().Unix()
		for (time.Now().Unix() - start_time) < search_radius {
			i++
		}
	})
}
```

What this block does, is:
1. Add the label `pyroscope.Labels("vehicle", vehicle)`
2. execute the `FindNearestVehicle()` function
3. Before the block ends it will (behind the scenes) remove the `pyroscope.Labels("vehicle", vehicle)` from the application since that block is complete

## Resulting flamegraph / performance results from the example
### Running the example
We have made it easy for you to run the example code yourself. All you have to do is to run the following commands:
```
# Clone the Pyroscope project to your current directory:
git clone https://github.com/pyroscope-io/pyroscope.git

# Navigate to the rideshare directory:
cd examples/golang-push/rideshare

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
# docker-compose down
```
_Note: you must run the `docker-compose up –build` command within the rideshare directory in the Pyroscope project folder._

What this example will do is run all the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. Once you’ve run the command to start Pyroscope docker image, head over to your browser and go to https://localhost:4040 to interact with Pyroscope. 

If you select our application: `ride-sharing-app.cpu` from the dropdown, you should see a flamegraph that looks like this (below). After we give 20-30 seconds for the flamegraph to update and then click the refresh button we see our 3 functions at the bottom of the flamegraph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

## Where's the performance bottleneck?

![golang_first_slide](https://user-images.githubusercontent.com/23323466/149688998-ca94dc82-f1e5-46fd-9a73-233c1e56d8e5.jpg)

The first step when analyzing a profile outputted from your application, is to take note of the _largest node_ which is where your application is spending the most resources. In this case, it happens to be the `OrderCar` function. 

The benefit of using the Pyroscope package, is that now that we can investigate further as to _why_ the `OrderCar()` function is problematic. Tagging both `region` and `vehicle` allows us to test two good hypotheses:
- Something is wrong with the `/car` endpoint code
- Something is wrong with one of our regions

To analyze this we can select one or more tags from the "Select Tag" dropdown:

![image](https://user-images.githubusercontent.com/23323466/135525308-b81e87b0-6ffb-4ef0-a6bf-3338483d0fc4.png)


## Narrowing in on the Issue Using Tags
Knowing there is an issue with the `OrderCar()` function we automatically select that tag. Then, after inspecting multiple `region` tags, it becomes clear by looking at the timeline that there is an issue with the `us-west-1` region, where it alternates between high-cpu times and low-cpu times.

We can also see that the `mutexLock()` function is consuming almost 70% of CPU resources during this time period. 

![golang_second_slide-01](https://user-images.githubusercontent.com/23323466/149689013-2c0afeeb-53e2-4780-b52a-26b140627d9c.jpg)

## Comparing two time periods
Using Pyroscope's "comparison view" we can actually select two different time ranges from the timeline to compare the resulting flamegraphs. The pink section on the left timeline results in the left flamegraph, and the blue section on the right represents the right flamegraph.

When we select a period of low-cpu utilization and a period of high-cpu utilization we can see that there is clearly different behavior in the `mutexLock()` function where it takes **33% of CPU** during low-cpu times and **71% of CPU** during high-cpu times.

![golang_third_slide-01](https://user-images.githubusercontent.com/23323466/149689026-8b4ab3b1-6380-455c-990f-7ff35811f26b.jpg)

## Visualizing Diff Between Two Flamegraphs
While the difference _in this case_ is stark enough to see in the comparison view, sometimes the diff between the two flamegraphs is better visualized with them overlayed over each other. Without changing any parameters, we can simply select the diff view tab and see the difference represented in a color-coded diff flamegraph.

![golang_fourth_slide-01](https://user-images.githubusercontent.com/23323466/149689038-50d12031-2879-470f-a3be-a4c71d8c3b7a.jpg)

### More use cases
We have been beta testing this feature with several different companies and some of the ways that we've seen companies tag their performance data:
- Tagging Kubernetes attributes
- Tagging controllers
- Tagging regions
- Tagging jobs from a queue
- Tagging commits
- Tagging staging / production environments
- Tagging different parts of their testing suites
- Etc...

## Share your flamegraphs!
Pyroscope doesn’t just profile your performance data alone, it allows you to share them in form of flamegraphs! Sharing flamegraphs is one of the fastest way to communicate to people, especially those new to profiling, about the performance of your existing application. It gives them the opportunity to play around the application profiles, without needing to access pyroscope. 

To share your flamegraph, click the hamburger menu and select flamegraph.com: 
![Export dropdown](https://user-images.githubusercontent.com/29557702/163255942-2ebf6637-acdc-4282-8769-45e3c954b033.png)

## Embed Flamegraphs into your application!
Guess you didn’t know about that! With Pyroscope, you can embed your flamegraphs into your web pages! All you need to do is append `/iframe` to the shared flamegraph url and you’re good to go. How cool is that? 

### Summary
We would love for you to try out this example and see what ways you can adapt this to your golang application. While this example focused on CPU debugging, Golang also provides memory profiling as well. Continuous profiling has become an increasingly popular tool for the monitoring and debugging of performance issues (arguably the fourth pillar of observability). 

We'd love to continue to improve our golang integrations and so we would love to hear what features _you would like to see_. 
