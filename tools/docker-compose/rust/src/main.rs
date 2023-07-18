#![warn(rust_2018_idioms)]

use pprof::protos::Message;
use std::io;
use std::task::{Context, Poll};
use std::{thread, time};

use log::{info, warn};

use futures_util::future;
use hyper::header::{CONTENT_LENGTH, CONTENT_TYPE};
use hyper::service::Service;
use hyper::{Body, Request, Response, Server};
use hyper_routing::{Route, RouterBuilder, RouterService};
use libflate::gzip::Encoder;

pub struct MakeSvc;

impl<T> Service<T> for MakeSvc {
    type Response = RouterService;
    type Error = std::io::Error;
    type Future = future::Ready<Result<Self::Response, Self::Error>>;

    fn poll_ready(&mut self, _cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Ok(()).into()
    }

    fn call(&mut self, _: T) -> Self::Future {
        future::ok(router_service())
    }
}

fn pprof_handler(request: Request<Body>) -> Response<Body> {
    let mut duration = time::Duration::from_secs(2);
    if let Some(query) = request.uri().query() {
        for (k, v) in form_urlencoded::parse(query.as_bytes()) {
            if k == "seconds" {
                duration = time::Duration::from_secs(v.parse::<u64>().unwrap());
            }
        }
    }
    info!("pprof handler: duration {:?} seconds", duration);

    let guard = pprof::ProfilerGuard::new(1_000_000).unwrap();

    thread::sleep(duration);

    let mut body = Vec::new();
    if let Ok(report) = guard.report().build() {
        let profile = report.pprof().unwrap();
        profile.write_to_vec(&mut body).unwrap();
    }

    // gzip profile
    let mut encoder = Encoder::new(Vec::new()).unwrap();
    io::copy(&mut &body[..], &mut encoder).unwrap();
    let gzip_body = encoder.finish().into_result().unwrap();

    Response::builder()
        .header(CONTENT_LENGTH, gzip_body.len() as u64)
        .header(CONTENT_TYPE, "application/octet-stream")
        .body(Body::from(gzip_body))
        .unwrap()
}

fn health_handler(_: Request<Body>) -> Response<Body> {
    let body = "ok";
    Response::builder()
        .header(CONTENT_LENGTH, body.len() as u64)
        .header(CONTENT_TYPE, "text/plain")
        .body(Body::from(body))
        .expect("Failed to construct the response")
}

fn router_service() -> RouterService {
    let router = RouterBuilder::new()
        .add(Route::get("/debug/pprof/profile").using(pprof_handler))
        .add(Route::get("/health").using(health_handler))
        .build();

    RouterService::new(router)
}

fn work(to: i64) {
    let mut found: i64 = 0; // Set found count to 0

    for count in 1i64..to {
        // Count integers from zero to ten million
        if count % 2 != 0 {
            // Continue if count is not even
            if check_prime(&count) {
                // Check if odd number if prime, using check_prime
                found = add(found, 1); // Increment found count
            }
        }
    }
    info!("there are {} prime numbers from 1 to {}", &found, to); // Print number, and total found

    fn check_prime(count: &i64) -> bool {
        // Function recieves int, and returns bool
        let stop = ((*count as f64).sqrt() + 1.0) as i64; // Find stopping number to loop to
        for i in 3..stop {
            // Start at 3 and go until stop
            if i % 2 != 0 {
                // Continue if i is not even
                if count % i == 0 {
                    // If count is divisable by i;
                    return false; // Return false
                }
            }
        }
        return true; // Only return true if never returned false
    }

    fn add(number: i64, add: i64) -> i64 {
        // Function adds a number to a number
        number + add // Return number plus number
    }
}

#[tokio::main]
async fn main() {
    pretty_env_logger::init();

    // We'll bind to 127.0.0.1:8080
    let addr = "0.0.0.0:8080".parse().unwrap();

    let server = Server::bind(&addr).serve(MakeSvc);
    info!("serving at {}", addr);

    // we need to do some work to get some samples
    thread::spawn(|| loop {
        work(10_000);
        thread::sleep(time::Duration::from_millis(500));
    });

    // Run this server for... forever!
    if let Err(e) = server.await {
        eprintln!("server error: {}", e);
    }
}
