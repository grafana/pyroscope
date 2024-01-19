use std::sync::Arc;

use chrono::prelude::*;
use pyroscope::PyroscopeAgent;
use pyroscope_pprofrs::{pprof_backend, PprofConfig};
use warp::Filter;

// Vehicle enum
#[derive(Debug, PartialEq)]
enum Vehicle {
    Car,
    Bike,
    Scooter,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Force rustc to display the log messages in the console.
    std::env::set_var("RUST_LOG", "trace");

    // Initialize the logger.
    pretty_env_logger::init_timed(); // Get Pyroscope server address from environment variable.

    // Get Pyroscope server address from environment variable.
    let server_address = std::env::var("PYROSCOPE_SERVER_ADDRESS")
        .unwrap_or_else(|_| "http://localhost:4040".to_string());
    // Get Region from environment variable.
    let region = std::env::var("REGION").unwrap_or_else(|_| "us-east".to_string());

    let app_name = std::env::var("PYROSCOPE_APPLICATION_NAME")
        .unwrap_or_else(|_| "rust-ride-sharing-app".to_string());

    let auth_user = std::env::var("PYROSCOPE_BASIC_AUTH_USER")
        .unwrap_or_else(|_| "".to_string());

    let auth_password = std::env::var("PYROSCOPE_BASIC_AUTH_PASSWORD")
        .unwrap_or_else(|_| "".to_string());

    // Configure Pyroscope client.
    let agent = PyroscopeAgent::builder(server_address, app_name.to_owned())
        .basic_auth(auth_user, auth_password)
        .backend(pprof_backend(PprofConfig::new().sample_rate(100)))
        .tags(vec![("region", &region)])
        .build()
        .unwrap();

    // Start the Pyroscope client.
    let agent_running = agent.start()?;

    // Root Route
    let root = warp::path::end().map(|| {
        // iterate throuh all env vars
        let mut vars = String::new();
        for (key, value) in std::env::vars() {
            vars = format!("{} {} \n", vars, format!("{}={}", key, value));
        }
        vars
    });

    let (add_tag, remove_tag) = agent_running.tag_wrapper();
    let add = Arc::new(add_tag);
    let remove = Arc::new(remove_tag);

    // Bike Route
    let bike = warp::path("bike").map(move || {
        add("vehicle".to_string(), "bike".to_string());
        order_bike(1);
        remove("vehicle".to_string(), "bike".to_string());

        "Bike ordered"
    });

    let (add_tag, remove_tag) = agent_running.tag_wrapper();
    let add = Arc::new(add_tag);
    let remove = Arc::new(remove_tag);

    // Scooter Route
    let scooter = warp::path("scooter").map(move || {
        add("vehicle".to_string(), "scooter".to_string());
        order_scooter(2);
        remove("vehicle".to_string(), "scooter".to_string());

        "Scooter ordered"
    });

    let (add_tag, remove_tag) = agent_running.tag_wrapper();
    let add = Arc::new(add_tag);
    let remove = Arc::new(remove_tag);

    // Car Route
    let car = warp::path("car").map(move || {
        add("vehicle".to_string(), "car".to_string());
        order_car(3);
        remove("vehicle".to_string(), "car".to_string());

        "Car ordered"
    });

    // Create a routes filter.
    let routes = warp::get().and(root).or(bike).or(scooter).or(car);

    // Serve the routes.
    warp::serve(routes).run(([0, 0, 0, 0], 5000)).await;

    // Stop the Pyroscope client.
    let agent_ready = agent_running.stop()?;

    // Shutdown PyroscopeAgent
    agent_ready.shutdown();

    Ok(())
}

fn order_bike(n: u64) {
    find_nearest_vehicle(n, Vehicle::Bike);
}

fn order_scooter(n: u64) {
    find_nearest_vehicle(n, Vehicle::Scooter);
}

fn order_car(n: u64) {
    find_nearest_vehicle(n, Vehicle::Car);
}

fn find_nearest_vehicle(search_radius: u64, vehicle: Vehicle) {
    let mut _i: u64 = 0;

    let start_time = std::time::Instant::now();
    while start_time.elapsed().as_secs() < search_radius {
        _i += 1;
    }

    if vehicle == Vehicle::Car {
        check_driver_availability(search_radius);
    }
}

fn check_driver_availability(search_radius: u64) {
    let mut _i: u64 = 0;

    let start_time = std::time::Instant::now();
    while start_time.elapsed().as_secs() < (search_radius / 2) {
        _i += 1;
    }
    // Every 4 minutes this will artificially create make requests in eu-north region slow
    // this is just for demonstration purposes to show how performance impacts show up in the
    // flamegraph
    let time_minutes = Local::now().minute();
    if std::env::var("REGION").unwrap_or_else(|_| "eu-north".to_owned()) == "eu-north"
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
