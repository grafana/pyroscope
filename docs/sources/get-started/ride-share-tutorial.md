---
description: Learn how to get started with Pyroscope using a simple Ride share app.
menuTitle: Ride share tutorial
title: Ride share tutorial with Pyroscope
weight: 250
killercoda:
  title: Ride share tutorial
  description: Learn how to get started with Pyroscope using a simple Ride share app.
  details:
      intro:
         foreground: docker-compose-update.sh
  backend:
    backend:
    imageid: ubuntu
---

<!-- INTERACTIVE page intro.md START -->

# Ride share tutorial with Pyroscope

This tutorial demonstrates a basic use case of Pyroscope by profiling a "Ride Share" application. In this example, you will learn:

- How a application is instrumented with Pyroscope. Including techniques for dynamically tagging functions.
- View the resulting profile data in the Pyroscope UI.
- Integrating Pyroscope with Grafana to visualize the profile data.

<!-- INTERACTIVE ignore START -->

## Before you begin

Before you begin this tutorial you need to have the following prerequisites:
- Git
- [Docker](https://docs.docker.com/compose/install/)
- The Docker Compose plugin (included with Docker Desktop)

{{< admonition type="tip" >}}
Alternatively, you can try out this example in our interactive learning environment: [Ride share tutorial with Pyroscope](https://killercoda.com/grafana-labs/course/pyroscope/ride-share-tutorial).

It's a fully configured environment with all the dependencies installed.

Provide feedback, report bugs, and raise issues in the [Grafana Killercoda repository](https://github.com/grafana/killercoda).
{{< /admonition >}}

<!-- INTERACTIVE ignore END -->

## Background

{{< figure max-width="100%" src="/media/docs/pyroscope/ride-share-demo.gif" caption="Getting started sample application" alt="Getting started sample application" >}}


In this tutorial, you will profile a simple "Ride Share" application. The application is a Python Flask app that simulates a ride-sharing service. The app has three endpoints which are found in the `server.py` file:

- `/bike`    : calls the `order_bike(search_radius)` function to order a bike
- `/car`     : calls the `order_car(search_radius)` function to order a car
- `/scooter` : calls the `order_scooter(search_radius)` function to order a scooter

To simulate a highly available and distributed system, the app is deployed on three distinct servers in 3 different regions: 
- us-east
- eu-north
- ap-south

This is simulated by running three instances of the server in Docker containers. Each server instance is tagged with the region it represents.

In this scenario a load generator will send mock-load to the 3 servers as well as their respective endpoints. This will allow us to see how the application is performing per region and per vehicle type.

{{<docs/ignore>}}
{{< admonition type="tip" >}}
A setup script is running in the background to install the necessary dependencies. This should take no longer than 30 seconds. Your instance will be ready to use once you `Setup complete. You may now begin the tutorial`.
{{< /admonition >}}
{{</docs/ignore>}}

<!-- INTERACTIVE page intro.md END -->

<!-- INTERACTIVE page step1.md START -->

## Clone the repository
Clone the repository to your local machine:

```bash
git clone https://github.com/grafana/pyroscope.git && cd pyroscope
```

Navigate to the tutorial directory:

```bash
cd examples/language-sdk-instrumentation/python/rideshare/flask
```

## Start the application

Start the application using Docker Compose:

```bash
docker compose up -d
```

This may take a few minutes to download the required images and build the demo application. Once ready, you will see the following output:

```console
 ✔ Network flask_default  Created
 ✔ Container flask-ap-south-1  Started
 ✔ Container flask-grafana-1  Started
 ✔ Container flask-pyroscope-1  Started     
 ✔ Container flask-load-generator-1 Started 
 ✔ Container flask-eu-north-1 Started       
 ✔ Container flask-us-east-1 Started  
```

(Optional) To verify the containers are running run:

```bash
docker ps -a
```
<!-- INTERACTIVE page step1.md END -->

<!-- INTERACTIVE page step2.md START -->

## Accessing the Pyroscope UI

Pyroscope includes a web-based UI that you can use to view the profile data. To access the Pyroscope UI, open a browser and navigate to [http://localhost:4040](http://localhost:4040).

### How tagging works

In this example, the application is instrumented with Pyroscope using the Python SDK. The SDK allows you to tag functions with metadata that can be used to filter and group the profile data in the Pyroscope UI. In this example we have used two forms of tagging; static and dynamic.

To start lets take a look at a static tag use case. Within the `server.py` file we can find the Pyroscope configuration:

```python
pyroscope.configure(
	application_name = app_name,
	server_address   = server_addr,
    basic_auth_username = basic_auth_username, # for grafana cloud
    basic_auth_password = basic_auth_password, # for grafana cloud
	tags             = {
        "region":   f'{os.getenv("REGION")}',
	}
)
```
The reason this tag is considered static is due to the fact that the tag is set at the start of the application and doesn't change. In our case this is useful for grouping profiles on a per region basis. Allowing us to see the performance of the application per region.

Lets take a look within the Pyroscope UI to see how this tag is used:

1. Open the Pyroscope UI in your browser at [http://localhost:4040](http://localhost:4040).
1. Click on `Tag Explorer` in the left-hand menu.
1. Select the `region` tag from the dropdown menu.

You should now see a list of regions that the application is running in. You can see that `eu-north` is experiencing the most load. 

{{< figure max-width="100%" src="/media/docs/pyroscope/ride-share-tag-region.png" caption="Region Tag" alt="Region Tag" >}}

Next lets take a look at a dynamic tag use case. Within the `utils.py` file we can find the following function:

```python
def find_nearest_vehicle(n, vehicle):
    with pyroscope.tag_wrapper({ "vehicle": vehicle}):
        i = 0
        start_time = time.time()
        while time.time() - start_time < n:
            i += 1
        if vehicle == "car":
            check_driver_availability(n)
```

In this example we are `tag_wrapper` to tag the function with the vehicle type. Notice that the tag is dynamic as it changes based on the vehicle type. This is useful for grouping profiles on a per vehicle basis. Allowing us to see the performance of the application per vehicle type being requested.

Lets take a look within the Pyroscope UI to see how this tag is used:
1. Open the Pyroscope UI in your browser at [http://localhost:4040](http://localhost:4040).
1. Click on `Tag Explorer` in the left-hand menu.
1. Select the `vehicle` tag from the dropdown menu.

You should now see a list of vehicle types that the application is using. You can see that `car` is experiencing the most load. 

<!-- INTERACTIVE page step2.md END -->

<!-- INTERACTIVE page step3.md START -->

## Identifying the performance bottleneck

The first step when analyzing a profile outputted from your application, is to take note of the largest node which is where your application is spending the most resources. To discover this, you can use the `Flame Graph` view within the Pyroscope UI:

1. Open the Pyroscope UI in your browser at [http://localhost:4040](http://localhost:4040).
1. Select the `Single View` tab.
1. Make sure `flask-ride-sharing-app:process_cpu:cpu` is selected in the dropdown menu.

It should look something like this:

{{< figure max-width="100%" src="/media/docs/pyroscope/ride-share-bottle-neck.jpg" caption="Bottleneck" alt="Bottleneck" >}}

The flask `dispatch_request` function is the parent to three functions that correspond to the three endpoints of the application:
- `order_bike`
- `order_car`
- `order_scooter`

The benefit of using Pyroscope, is that by tagging both `region` and `vehicle` and looking at the Tag Explorer page we can hypothesize:

- Something is wrong with the `/car` endpoint code where `car` vehicle tag is consuming **68% of CPU**
- Something is wrong with one of our regions where `eu-north` region tag is consuming **54% of CPU**

From the flame graph we can see that for the `eu-north` tag the biggest performance impact comes from the `find_nearest_vehicle()` function which consumes close to **68% of cpu**. To analyze this we can go directly to the comparison page using the comparison dropdown.

### Comparing two time periods

The comparison page allows you to compare two time periods side by side. This is useful for identifying changes in performance over time. In this example we will compare the performance of the `eu-north` region within a given time period against the other regions.

1. Open the Pyroscope UI in your browser at [http://localhost:4040](http://localhost:4040).
1. Click on `Comparison` in the left-hand menu.
1. Within `Baseline time range` copy and paste the following query:
   ```console
   process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="flask-ride-sharing-app", vehicle="car", region!="eu-north"}
   ```
1. Within `Comparison time range` copy and paste the following query:
   ```console
   process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="flask-ride-sharing-app", vehicle="car", region="eu-north"}
   ```
1. Execute both queries by clicking the `Execute` buttons.

If we scroll down to compare the two time periods side by side we can see that the `eu-north` region (right hand side) we can see an excessive amount of time spent in the `find_nearest_vehicle` function. This looks to be caused by a mutex lock that is causing the function to block.

{{< figure max-width="100%" src="/media/docs/pyroscope/ride-share-time-comparison.png" caption="Time Comparison" alt="Time Comparison" >}}

To confirm our suspicions we can use the `Diff` view to see the difference between the two time periods.

### Viewing the difference between two time periods

The `Diff` view allows you to see the difference between two time periods. This is useful for identifying changes in performance over time. In this example we will compare the performance of the `eu-north` region within a given time period against the other regions.

1. Open the Pyroscope UI in your browser at [http://localhost:4040](http://localhost:4040).
1. Click on `Diff` in the left-hand menu.
1. Make sure to have set the `Baseline time range` and `Comparison time range` queries as per the previous step.
1. Click on the `Execute` buttons.

If we scroll down to compare the two time periods side by side we can see that the `eu-north` region (right hand side) we can see an excessive amount of time spent in the `find_nearest_vehicle` function. This confirms our suspicions that the mutex lock that is causing the function to block.

{{< figure max-width="100%" src="/media/docs/pyroscope/ride-share-diff-page.png" caption="Diff" alt="Diff" >}}

<!-- INTERACTIVE page step3.md END -->

<!-- INTERACTIVE page step4.md START -->

{{<docs/ignore>}}
{{< admonition type="tip" >}}
Unfortunately, due to a bug within the Sandbox environment, the profile explorer app is currently unavailable. We are working on a fix and will update this tutorial once resolved. If you would like to try out the profile explorer app, you can run the example locally on your machine. Or you can try out this example in [Grafana Play](https://play.grafana.org/a/grafana-pyroscope-app/profiles-explorer?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=labels&var-serviceName=pyroscope-rideshare-python&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-dataSource=grafanacloud-profiles&var-groupBy=all&var-filters=)
{{< /admonition >}}
{{</docs/ignore>}}

## Integrating Pyroscope with Grafana

As part of the `docker-compose.yml` file, we have included a Grafana container that's pre-configured with the Pyroscope plugin:
    
```yaml
  grafana:
    image: grafana/grafana:latest
    environment:
    - GF_INSTALL_PLUGINS=grafana-pyroscope-app
    - GF_AUTH_ANONYMOUS_ENABLED=true
    - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
    - GF_AUTH_DISABLE_LOGIN_FORM=true
    volumes:
    - ./grafana-provisioning:/etc/grafana/provisioning
    ports:
    - 3000:3000
```

We've also pre-configured the Pyroscope data source in Grafana. 

To access the Pyroscope app in Grafana, navigate to [http://localhost:3000/a/grafana-pyroscope-app](http://localhost:3000/a/grafana-pyroscope-app). 

### Challenge

As a challenge see if you can generate the same comparison we achieved in the Pyroscope UI within Grafana. It should look something like this:

{{< figure max-width="100%" src="/media/docs/pyroscope/ride-share-grafana.png" caption="Grafana" alt="Grafana" >}}

<!-- INTERACTIVE page step4.md END -->

<!-- INTERACTIVE page finish.md START -->

## Summary

In this tutorial, you learned how to profile a simple "Ride Share" application using Pyroscope. You have learned some of the core instrumentation concepts such as tagging and how to use the Pyroscope UI to identify performance bottlenecks. You also learned how to integrate Pyroscope with Grafana to visualize the profile data.

### Next steps

- Learn more about the Pyroscope SDKs and how to [instrument your application with Pyroscope](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/).
- Deploy Pyroscope in a production environment using the [Pyroscope Helm chart](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/deploy-kubernetes/).

<!-- INTERACTIVE page finish.md END -->









