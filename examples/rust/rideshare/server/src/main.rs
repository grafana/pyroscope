use chrono::prelude::*;
use pyroscope::PyroscopeAgent;
use warp::Filter;

// Vehicule enum
#[derive(Debug, PartialEq)]
enum Vehicule {
    Car,
    Bike,
    Scooter,
}

#[tokio::main]
async fn main() {
    // Force rustc to display the log messages in the console.
    std::env::set_var("RUST_LOG", "trace");

    // Initialize the logger.
    pretty_env_logger::init_timed(); // Get Pyroscope server address from environment variable.

    // Get Pyroscope server address from environment variable.
    let server_address = std::env::var("PYROSCOPE_SERVER_ADDRESS")
        .unwrap_or_else(|_| "http://localhost:4040".to_string());
    // Get Region from environment variable.
    let region = std::env::var("REGION").unwrap_or_else(|_| "us-east-1".to_string());

    // Configure Pyroscope client.
    let mut agent = PyroscopeAgent::builder(server_address, "ride-sharing-rust".to_owned())
        .sample_rate(100)
        .tags(&[("region", &region)])
        .build()
        .unwrap();

    // Start the Pyroscope client.
    agent.start();

    // Root Route
    let root = warp::path::end().map(|| {
        // iterate throuh all env vars
        let mut vars = String::new();
        for (key, value) in std::env::vars() {
            vars = format!("{} {} \n", vars, format!("{}={}", key, value));
        }
        vars
    });

    // Bike Route
    let bike = warp::path("bike").map(|| {
        order_bike(1);
        "Bike ordered"
    });

    // Scooter Route
    let scooter = warp::path("scooter").map(|| {
        order_scooter(2);
        "Scooter ordered"
    });

    // Car Route
    let car = warp::path("car").map(|| {
        order_car(3);
        "Car ordered"
    });

    // Create a routes filter.
    let routes = warp::get().and(root).or(bike).or(scooter).or(car);

    // Serve the routes.
    warp::serve(routes).run(([0, 0, 0, 0], 5000)).await;

    // Stop the Pyroscope client.
    agent.stop();
}

fn order_bike(n: u64) {
    find_nearest_vehicule(n, Vehicule::Bike);
}

fn order_scooter(n: u64) {
    find_nearest_vehicule(n, Vehicule::Scooter);
}

fn order_car(n: u64) {
    find_nearest_vehicule(n, Vehicule::Car);
}

fn find_nearest_vehicule(search_radius: u64, vehicule: Vehicule) {
    let mut _i: u64 = 0;

    let start_time = std::time::Instant::now();
    while start_time.elapsed().as_secs() < search_radius {
        _i += 1;
    }

    if vehicule == Vehicule::Car {
        check_driver_availability(search_radius);
    }
}

fn check_driver_availability(search_radius: u64) {
    let mut _i: u64 = 0;

    let start_time = std::time::Instant::now();
    while start_time.elapsed().as_secs() < (search_radius / 2) {
        _i += 1;
    }
    // Every 4 minutes this will artificially create make requests in us-west-1 region slow
    // this is just for demonstration purposes to show how performance impacts show up in the
    // flamegraph
    let time_minutes = Local::now().minute();
    if std::env::var("REGION").unwrap_or_else(|_| "us-west-1".to_owned()) == "us-west-1"
        && (time_minutes * 8 % 4 == 0)
    {
        mutex_lock(search_radius);
    }
}

fn mutex_lock(search_radius: u64) {
    let mut _i: u64 = 0;

    let start_time = std::time::Instant::now();
    while start_time.elapsed().as_secs() < (search_radius * 10) {
        _i += 1;
    }
}
