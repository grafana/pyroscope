## Prerequisites
* docker-compose

## Usage

To start benchmark run:
```
./start.sh
```


Pass `--wait` to make the system continue running after benchmarking is over:
```
sh start.sh --wait
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
