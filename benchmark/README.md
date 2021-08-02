## Prerequisites

* docker-compose
* puppeteer and Google Chrome for screenshots


## Usage

To start benchmark run:
```
sh start.sh
```
Pass `--wait` to make the system continue running after benchmarking is over:
```
sh start.sh --wait
```
Pass `--keep-data` to keep existing data:
```
sh start.sh --keep-data
```


## Configuration

Edit `run-parameters.env` file to change the parameters of the benchmark run.


## Browsing results

To view results open [http://localhost:8080/d/65gjqY3Mk/main?orgId=1](http://localhost:8080/d/65gjqY3Mk/main?orgId=1).

You will also be able to see screenshots of the runs in `./runs` directory
