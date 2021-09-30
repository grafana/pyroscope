### Pyroscope Rideshare Example
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
- `vehicle`: dynamically tags the endpoint (similar to tagging a controller rails)

### Tagging static region
Tagging something static, like the `region`, can be done in the initialization code in the `config.tags` variable:
```
Pyroscope.configure do |config|
  config.app_name = "ride-sharing-app"
  config.server_address = "http://pyroscope:4040"
  config.tags = {
    "region": ENV["REGION"],                     # Tags the region based of the environment variable
  }
end
```

### Tagging dynamically within functions
Tagging something more dynamically, like we do for the `vehicle` tag can be done inside our utility `find_nearest_vehicle()` function using a `Pyroscope.tag_wrapper` block
```
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

### Resulting flamgraph from the example
To run the example run the following commands:
```
# Pull latest pyroscope image:
docker pull pyroscope/pyroscope:latest

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
# docker-compose down
```

What this example will do is run all of the code mentioned above and also send some mock-load to the 3 servers as well as their respective 3 endpoints. If you select our application: `ride-sharing-app.cpu` from the dropdown, you should see a flamegraph that looks like this:

[ Picture ]

After we give 20-30 seconds for the flamegraph to update and then click the refresh button we see our 3 functions at the bottom of the flamegraph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.

In the real world, it's possible that _the region_ of a server is, for some reason, causing difference performance behavior than other regions. To inspect this, we can select our various regions from the "tag" dropdown:

[ Picture of region selected ]


### Usage
to pull latest pyroscope image:
```
docker pull pyroscope/pyroscope:latest
```

to run this example project:
```
docker-compose up --build
```

to reset the database:
```
docker-compose down
```
