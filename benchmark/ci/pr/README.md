# PR Benchmark
This

* runs 2 instances of pyroscope (the one in the PR and the main one) in a docker-compose.
* generates test load against both instances
* takes a screenshot of the dashboard panes
* posts using [dangerjs](https://danger.systems/js/) in the PR body



# Running locally

create a folder `dashboard-screenshots`
and `./run-benchmark.sh`

You may tweak the running time for quicker feedback loop `BENCH_RUN_FOR=30s ./run-benchmark.sh`
