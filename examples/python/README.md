### Pyroscope Rideshare Example
![python_example_architecture_05_00](https://user-images.githubusercontent.com/23323466/135728737-0c5e54ca-1e78-4c6d-933c-145f441c96a9.gif)


Note: For documentation on the Pyroscope pip package visit [our website](https://pyroscope.io/docs/python/)
## Backround
In this example we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three endpoints found in `server.rb`:
- `/bike`    : calls the `order_bike(search_radius)` function to order a bike
- `/car`     : calls the `order_car(search_radius)` function to order a car
- `/scooter` : calls the `order_scooter(search_radius)` function to order a scooter

We also simulate running 3 distinct servers in 3 different regions (via docker-compose.yml)
- us-east-1
- us-west-1
- eu-west-1

One of the most useful capabilities of Pyroscope is the ability to tag your data in a way that is meaningful to you. In this case, we have two natural divisions and so we "tag" our data to represent those:
- `region`: statically tags the region of the server running the code
- `vehicle`: dynamically tags the endpoint (similar to how one might tag a controller rails)


## Tagging static region
Tagging something static, like the `region`, can be done in the initialization code in the `config.tags` variable:
```
pyroscope.configure(
	app_name       = "ride-sharing-app",
	server_address = "http://pyroscope:4040",
	tags           = {
    "region":   f'{os.getenv("REGION")}',        # Tags the region based off the environment variable
	}
)
```

## Tagging dynamically within functions
Tagging something more dynamically, like we do for the `vehicle` tag can be done inside our utility `find_nearest_vehicle()` function using a `with pyroscope.tag_wrapper()` block
```
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

## Resulting flamgraph / performance results from the example
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

What this example will do is run all of the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. If you select our application: `ride-sharing-app.cpu` from the dropdown, you should see a flamegraph that looks like this (below). After we give 20-30 seconds for the flamegraph to update and then click the refresh button we see our 3 functions at the bottom of the flamegraph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

![image](https://user-images.githubusercontent.com/23323466/135525201-b50d819a-278f-4693-a523-a4731b9c0306.png)


In the real world, it's possible that _the region_ of a server is, for some reason, causing difference performance behavior than other regions. To inspect this, we can select our various regions from the "tag" dropdown:

![image](https://user-images.githubusercontent.com/23323466/135525308-b81e87b0-6ffb-4ef0-a6bf-3338483d0fc4.png)

If we wanted to select both a specific `region` and and a specific `vehicle` then we can simply select both from the dropdown and see the performance characteristics of that combination. Notice that we can also see how much CPU utilization was attributed to this specific combination of tags via the timeline at the top of the page.

![image](https://user-images.githubusercontent.com/23323466/135525626-3d558bf3-169f-4295-989f-b422fff3f87f.png)


### More use cases
We have been beta testing this feature with several different companies and some of the ways that we've seen companies tag their performance data:
- Tagging controllers
- Tagging regions
- Tagging jobs from a redis or sidekiq queue
- Tagging commits
- Tagging staging / production environments
- Tagging different parts of their testing suites
- Etc...

### Future Roadmap
We would love for you to try out this example and see what ways you can adapt this to your python application. Continuous profiling has become an increasingly popular tool for the monitoring and debugging of performance issues (arguably the fourth pillar of observability). 

We'd love to continue to improve this pip package by adding things like integrations with popular tools, memory profiling, etc. and we would love to hear what features _you would like to see_. 
