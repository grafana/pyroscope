## Prerequisites

* docker-compose
* puppeteer and Google Chrome for screenshots (optional, `npm install -g puppeteer`)

## Troubleshooting

Make sure you have enough memory allocated for docker, e.g on a mac:

![image](https://user-images.githubusercontent.com/662636/128406795-f4a50e4b-03d7-4eed-a637-45f0c638a16b.png)

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
