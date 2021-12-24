## Prerequisites
* You'll need `docker-compose` installed.
* Ideally you want to be running this on a large enough machine (> 4 cores, 16 GB of RAM), otherwise services might run out of memory

## Usage

To start benchmark run:
```
./start.sh
```

## To configure the benchmarking parameters, edit `config.env` file. It contains variables that are used by pyroscope and pyrobench.

```
```

## Browsing results
To view results open http://localhost:8080/d/tsWRL6ReZQkirFirmyvnWX1akHXJeHT8I8emjGJo/main?orgId=1.

## Configuration
Edit `run-parameters.env` file to change the parameters of the benchmark run.

### Use cases

#### Running indefinitely
Maybe you want to leave the load generator running for an indefine amount of time.

For that, just pick a big enough value for `PYROBENCH_REQUESTS`, like `100000` (the default)


## Troubleshooting

Make sure you have enough memory allocated for docker, e.g on a mac:

![image](https://user-images.githubusercontent.com/662636/128406795-f4a50e4b-03d7-4eed-a637-45f0c638a16b.png)


## Design goals with benchmark project

This benchmark suite attempts to be as flexible as possible while still being simple.


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
Update the `report.yaml` file
