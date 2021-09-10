# PR Benchmark
This

* runs 2 instances of pyroscope (the one in the PR and the main one) in a docker-compose.
* generates test load against both instances
* takes a screenshot of the dashboard panes
* posts using [dangerjs](https://danger.systems/js/) in the PR body

# Running locally

Create a director `dashboard-screenshots` and `./run-benchmark.sh`

Screenshots will be stored in `dashboard-screenshots`

You may tweak the running time for a quicker feedback loop `BENCH_RUN_FOR=30s ./run-benchmark.sh`
If you want to just leave it running, `BENCH_RUN_FOR=Infinity ./run-benchmark.sh`

# Adding more panes
Just update the dashboard in `monitoring/benchmark-pr.jsonnet`

# Adding more things to the report
Update the `report.tmpl.yaml` file
