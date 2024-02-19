## An example for setting up profile-guided optimization (PGO) using Pyroscope

Profile-Guided Optimization (PGO) is a Go compiler feature that uses runtime profiling data to optimize code.
Now fully integrated in Go 1.21, PGO is a powerful tool to boost application performance. 

PGO enhances performance primarily through two mechanisms:
- Inlining hot (frequently executed) methods
- Devirtualization of interface calls

Take a look at https://go.dev/doc/pgo and https://go.dev/blog/pgo for more information on PGO.

### Using PGO in the Rideshare example application

Here are the steps needed to use PGO in the Rideshare example application.

1. Run the application that we will enable PGO for.

    ```shell
    docker-compose up --build --detach
    ```

2. Run the prepared benchmark

    ```shell
    go test -bench=BenchmarkApp -count=10 main_test.go
    ```

3. Extract a profile in pprof format with `profilecli` (see the [Profile CLI documentation](https://grafana.com/docs/pyroscope/latest/ingest-and-analyze-profile-data/profile-cli/#install-profile-cli) for further reference)

    ```shell
    profilecli query merge \
        --query='{service_name="ride-sharing-app"}' \
        --profile-type="process_cpu:cpu:nanoseconds:cpu:nanoseconds" \
        --from="now-5m" \
        --to="now" \
        --output=pprof=./default.pgo
    ```

    This command will create a default.pgo (pprof) file in the current folder (`/examples/golang-pgo/rideshare/`).

4. Rebuild the Rideshare application. This will pick up the newly created PGO file automatically.

    ```shell
    docker-compose down
    docker-compose up --build --detach
    ```

5. (Optional) Verify that the application was built with PGO

    ```shell 
   docker exec -it golang-pgo-rideshare-go-1 /bin/bash
   go version -m main
   exit
    ```
    You should see this line in the output:
    ```shell
   ...
   build	-pgo=/go/src/app/default.pgo
   ...
    ```
6. Run the benchmark again

   ```shell
   go test -bench=BenchmarkApp -count=10 main_test.go
   ```
7. (Optional) Use a tool such as [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) to compare the two benchmarks.
