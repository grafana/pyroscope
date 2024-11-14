## Continuous Profiling for Node applications
### Profiling a Node Rideshare App with Pyroscope

![golang_example_architecture_new_00](https://user-images.githubusercontent.com/23323466/173370161-f8ba5c0a-cacf-4b3b-8d84-dd993019c486.gif)

Note: For documentation on Pyroscope's Node integration visit [our website](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/).

## Background
In this example, we show a simplified, basic use case of Pyroscope. We simulate a "ride share" company which has three endpoints found in `main.js`:
- `/bike`    : calls the `bikeSearchHandler()` function to order a bike
- `/car`     : calls the `carSearchHandler()` function to order a car
- `/scooter` : calls the `scooterSearchHandler()` function to order a scooter

We also simulate running 3 distinct servers in 3 different regions (via [docker-compose.yml](./express/docker-compose.yml))
- us-east
- eu-north
- ap-south

One of the most useful capabilities of Pyroscope is the ability to tag your data in a way that is meaningful to you. In this case, we have two natural divisions, and so we "tag" our data to represent those:
- `region`: statically tags the region of the server running the code


## Tagging static region
Tagging something static, like the `region`, can be done in the initialization code in the `main()` function:
```js
  Pyroscope.init({
    appName: 'nodejs',
    serverAddress: process.env['PYROSCOPE_SERVER'] || 'http://pyroscope:4040',
    tags: { region: process.env['REGION'] || 'default' }
  });
```

## Resulting flame graph / performance results from the example
### Running the example

There are 3 examples:
* `express` - basic integration example
* `express-ts` - type script example
* `express-pull` — pull mode example

To use any of the examples, run the following commands:
```shell
# change directory
cd express # or cd express-ts / cs express-pull

# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker-compose up --build

# Reset the database (if needed):
docker-compose down
```

This example runs all the code mentioned above and also sends some mock-load to the 3 servers as well as their respective 3 endpoints. If you select `nodejs.wall` from the dropdown, you should see a flame graph that looks like this (below).
After 20-30 seconds, the flame graph updates. Click the refresh button. We see our 3 functions at the bottom of the flame graph taking CPU resources _proportional to the size_ of their respective `search_radius` parameters.
