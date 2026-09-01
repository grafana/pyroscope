---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/profile-cli/
  - /docs/phlare/latest/profile-cli/
  - /docs/pyroscope/latest/configure-server/profile-cli/
  - ../ingest-and-analyze-profile-data/profile-cli/
description: Getting started with the profile CLI tool.
menuTitle: Profile CLI
title: Profile CLI
weight: 500
---

# Profile CLI

Pyroscope provides a command-line interface (CLI), `profilecli`.
This utility enables various productivity flows such as:

- Interacting with a running Pyroscope server to upload profiles, query data, and more
- Inspecting [Parquet](https://parquet.apache.org/docs/) files

{{< admonition type="tip">}}
Use the `help` command (`profilecli help`) for a full list of capabilities and help information.
{{< /admonition>}}

## Install Profile CLI

You can install Profile CLI using a package or by compiling the code.

### Install using a package

On macOS, you can install Profile CLI using [Homebrew](https://brew.sh):

```bash
brew install pyroscope-io/brew/profilecli
```

For other platforms, you can manually [download the `profilecli` release asset](https://github.com/grafana/pyroscope/releases/latest) for your operating system and architecture and make it executable.

For example, for Linux with the AMD64 architecture:

1. Download and extract the package (archive).

    ```bash
    curl -fL https://github.com/grafana/pyroscope/releases/download/v1.13.2/profilecli_1.13.2_linux_amd64.tar.gz | tar xvz
    ```

1. Make `profilecli` executable:

    ```bash
    chmod +x profilecli
    ```

1. Optional: Make `profilecli` reachable from anywhere:

    ```bash
    sudo mv profilecli /usr/local/bin
    ```

### Build from source code

To build from source code, you must have:

- Go 1.24.6 or later installed.
- Either `$GOPATH` or `$GOBIN` configured and added to your `PATH` environment variable.

To build the source code:

1. Clone the repository.

   ```bash
   git clone git@github.com:grafana/pyroscope.git
   ```

1. Run the Go install command to build and install the package.

   ```bash
   cd pyroscope
   go install ./cmd/profilecli
   ```

   The command places the `profilecli` executable in `$GOPATH/bin/` (or `$GOBIN/`) and makes it available to use.


## Common flags and environment variables

The `profilecli` commands that interact with a Pyroscope server use the same connection and authentication flags. You can pass them as command flags or environment variables.

| Purpose | Flag | Environment variable | Default | When it helps |
| --- | --- | --- | --- | --- |
| Pyroscope endpoint | `--url` | `PROFILECLI_URL` | `http://localhost:4040` | Point the command to your local server, Grafana Cloud Profiles endpoint, or Grafana data source proxy URL. |
| Basic authentication | `--username`, `--password` | `PROFILECLI_USERNAME`, `PROFILECLI_PASSWORD` | empty | Authenticate with a Cloud Profiles endpoint using stack ID and API token. |
| Bearer token | `--token` | `PROFILECLI_TOKEN` | empty | Authenticate through Grafana data source proxy URLs or token-based environments. |
| Tenant header | `--tenant-id` | `PROFILECLI_TENANT_ID` | empty | Set `X-Scope-OrgID` when you query or upload against multi-tenant deployments. |
| Transport protocol | `--protocol` | Not available | `connect` | Troubleshoot compatibility by switching to `grpc` or `grpc-web` if needed. |

### Authentication examples

Use the method that matches your environment.

#### Basic auth example

Use this pattern when you connect directly to a Cloud Profiles endpoint.

```bash
export PROFILECLI_URL=https://profiles-prod-001.grafana.net
export PROFILECLI_USERNAME=<cloud_stack_id>
export PROFILECLI_PASSWORD=<cloud_access_policy_token>
profilecli query series --query='{service_name="checkout"}'
```

This is the most common setup when you are querying Cloud Profiles directly.

#### Bearer token example

Use this pattern when you connect through a Grafana data source proxy URL.

```bash
export PROFILECLI_URL=https://grafana.example.net/api/datasources/proxy/uid/<uid>
export PROFILECLI_TOKEN=<glsa_or_glc_token>
profilecli query profile --profile-type=process_cpu:cpu:nanoseconds:cpu:nanoseconds
```

This is helpful when you want to use existing Grafana access controls instead of direct Pyroscope credentials.

#### Multi-tenant example

Use this pattern for self-managed, multi-tenant Pyroscope deployments.

```bash
export PROFILECLI_URL=https://pyroscope.example.net
export PROFILECLI_TENANT_ID=team-a
profilecli upload --extra-labels=service_name=payments ./cpu.pprof
```

This is useful when a shared Pyroscope deployment routes data by tenant.

### Environment variable naming

You can use environment variables to avoid passing flags to every command and to reduce accidental credential exposure in shell history.
Environment variables have a `PROFILECLI_` prefix. Here is an example:

```bash
export PROFILECLI_URL=<pyroscope_server_url>
export PROFILECLI_USERNAME=<username>
export PROFILECLI_PASSWORD=<password>
# now you can run profilecli commands without repeating URL or credentials:
profilecli <command>
```

{{< admonition type="caution" >}}
If you're querying data from Cloud Profiles, use the URL of your Cloud Profiles server in `PROFILECLI_URL` (for example, `https://profiles-prod-001.grafana.net`) and **not** the URL of your Grafana Cloud tenant (for example, `<your-tenant>.grafana.net`).
{{< /admonition >}}

## Upload a profile to a Pyroscope server using `profilecli`

Using `profilecli` streamlines the process of uploading profiles to Pyroscope, making it a convenient alternative to manual HTTP requests.

### Why this command helps

Use `profilecli upload` when you have an exported pprof file and want to:

- Reproduce a production issue in a test environment.
- Backfill a profile collected outside your normal instrumentation pipeline.
- Attach labels at upload time to make the data easier to query later.

### Before you begin

- Ensure you have `profilecli` installed on your system by following the [installation](#install-profile-cli) steps above.
- Have a profile file ready for upload. Note that you can only upload pprof files at this time.

### Upload steps

1. Identify the pprof file.

   - Path to your pprof file: `path/to/your/pprof-file.pprof`

1. Optional: Specify any extra labels.

   - You can add additional labels to your uploaded profile using the `--extra-labels` flag.
   - You can provide the name of the application that the profile was captured from via the `service_name` label (defaults to `profilecli-upload`). This will be useful when querying the data via `profilecli` or the UI.
   - You can use the flag multiple times to add several labels.
   - Use `--override-timestamp` if you want the uploaded profile to be treated as "now" instead of its original capture time.

1. Construct and execute the Upload command.

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli upload --extra-labels=<label_name>=<label_value> <pprof_file_path>
     ```

   - Example command:
     ```bash
     export PROFILECLI_URL=https://profiles-prod-001.grafana.net
     export PROFILECLI_USERNAME=my_username
     export PROFILECLI_PASSWORD=my_password

     profilecli upload path/to/your/pprof-file.pprof
     ```

   - Example command with extra labels:
     ```bash
     export PROFILECLI_URL=https://profiles-prod-001.grafana.net
     export PROFILECLI_USERNAME=my_username
     export PROFILECLI_PASSWORD=my_password

     profilecli upload \
         --extra-labels=service_name=my_application_name \
         --extra-labels=cluster=us-east \
         path/to/your/pprof-file.pprof
     ```

   - Example command with timestamp override:
     ```bash
     profilecli upload \
         --override-timestamp \
         --extra-labels=service_name=debug-replay \
         ./local-capture.pprof
     ```

1. Check for successful upload.

   - After running the command, you should see a confirmation message indicating a successful upload. If there are any issues, `profilecli` provides error messages to help you troubleshoot.

## Query a Pyroscope server using `profilecli`

You can use the `profilecli query` command to look up the available profiles on a Pyroscope server and read actual profile data.
This can be useful for debugging purposes or for integrating profiling in CI pipelines (for example to facilitate [profile-guided optimization](https://go.dev/doc/pgo)).

### Look up available profiles on a Pyroscope server

You can use the `profilecli query series` command to look up the available profiles on a Pyroscope server.
By default, it queries the last hour of data, though this can be controlled with the `--from` and `--to` flags.
You can narrow the results down with the `--query` flag. See `profilecli help query series` for more information.

This command is most helpful when you are exploring an unfamiliar environment and need to discover:

- Which services are currently sending profiles.
- Which profile types are available for a service.
- Which label keys and values you can use for follow-up queries.

#### Query series steps

1. Optional: Specify a query, time range, and output format.

   - You can provide a label selector using the `--query` flag, for example: `--query='{service_name="my_application_name"}'`.
   - You can provide a custom time range using the `--from` and `--to` flags, for example, `--from="now-3h" --to="now"`.
   - You can filter which label names appear in the output using the `--label-names` flag, for example, `--label-names=__profile_type__,service_name`.
   - You can control the output format using `--output=table` (default) or `--output=json`. The table view renders one row per series with label names as sorted column headers. The JSON format emits a structured envelope containing `from`, `to`, and a `series` array, which is useful for scripting and pipeline integrations.

1. Construct and execute the Query Series command.

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli query series --query='{<label_name>="<label_value>"}'
     ```

   - Example command:
     ```bash
     export PROFILECLI_URL=https://profiles-prod-001.grafana.net
     export PROFILECLI_USERNAME=my_username
     export PROFILECLI_PASSWORD=my_password

     profilecli query series --query='{service_name="my_application_name"}'
     ```

   - Example command with label filtering and default table output:
     ```bash
     profilecli query series \
         --query='{service_name="my_application_name"}' \
         --label-names=__profile_type__ \
         --label-names=service_name
     ```

   - Example table output (default):
     ```
     +---------------------------------------------+---------------------+
     |                PROFILE TYPE                 |    SERVICE NAME     |
     +---------------------------------------------+---------------------+
     | memory:inuse_objects:count:space:bytes      | my_application_name |
     | process_cpu:cpu:nanoseconds:cpu:nanoseconds | my_application_name |
     +---------------------------------------------+---------------------+
     ```

     Columns are sorted alphabetically by their raw label name. Column headers are derived from label names with underscores replaced by spaces and converted to uppercase (for example, `__profile_type__` becomes `PROFILE TYPE`).

   - Example command using `--output=json`:
     ```bash
     profilecli query series \
         --query='{service_name="my_application_name"}' \
         --output=json
     ```

   - Example JSON output:
     ```json
     {
       "from": "2026-03-12T08:54:07.667114Z",
       "to": "2026-03-12T09:54:07.667114Z",
       "series": [
         {
           "__name__": "memory",
           "__period_type__": "space",
           "__period_unit__": "bytes",
           "__profile_type__": "memory:inuse_objects:count:space:bytes",
           "__service_name__": "my_application_name",
           "__type__": "inuse_objects",
           "__unit__": "count",
           "cluster": "eu-west-1",
           "service_name": "my_application_name"
         }
       ]
     }
     ```

### Read a raw profile from a Pyroscope server

You can use the `profilecli query profile` command to retrieve a merged (aggregated) profile from a Pyroscope server.
The command merges all samples found in the profile store for the specified query and time range.
By default it looks for samples within the last hour, though this can be controlled with the `--from` and `--to` flags. The source data can be narrowed down with the `--query` flag in the same way as with the `series` command.

This command is useful when you want to inspect merged profile data directly, save it for offline analysis, or compare profile windows in scripts and CI jobs.

#### Query profile steps

1. Specify optional flags.

   - You can provide a label selector using the `--query` flag, for example, `--query='{service_name="my_application_name"}'`.
   - You can provide a custom time range using the `--from` and `--to` flags, for example, `--from="now-3h" --to="now"`.
   - You can specify the profile type via the `--profile-type` flag. The available profile types are listed in the output of the `profilecli query series` command.
   - You can set `--output=pprof=./result.pprof` to save the merged profile as a pprof file.
   - You can use `--function-names-only` for faster responses when you don't need full mapping and line details.

2. Construct and execute the `query profile` command.

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli query profile \
         --profile-type=<profile_type> \
         --query='{<label_name>="<label_value>"}' \
         --from="<from>" --to="<to>"
     ```

   - Example command:
     ```bash
     export PROFILECLI_URL=https://profiles-prod-001.grafana.net
     export PROFILECLI_USERNAME=my_username
     export PROFILECLI_PASSWORD=my_password

     profilecli query profile \
         --profile-type=memory:inuse_space:bytes:space:bytes \
         --query='{service_name="my_application_name"}' \
         --from="now-1h" --to="now"
     ```

   - Example command saving pprof output:
     ```bash
     profilecli query profile \
         --profile-type=process_cpu:cpu:nanoseconds:cpu:nanoseconds \
         --query='{service_name="checkout"}' \
         --from="now-30m" --to="now" \
         --output=pprof=./checkout-cpu.pprof
     ```

   - Example output:
     ```bash
     level=info msg="query aggregated profile from profile store" url=http://localhost:4040 from=2023-12-11T13:38:33.115683-04:00 to=2023-12-11T14:38:33.115684-04:00 query={} type=memory:inuse_space:bytes:space:bytes
     PeriodType: space bytes
     Period: 524288
     Time: 2023-12-11 13:59:59.999 -0400 AST
     Duration: 59m5
     Samples:
     inuse_space/bytes[dflt]
       115366240: 107 13 14 15 16 17 1 2 3
     ...
     ```

### Find and inspect exemplars

An exemplar is a pointer from an aggregated view back to a single profile or trace span that contributed to it.
Use `profilecli query exemplars` to list the exemplars in a time range, then pass an ID from the results to `profilecli query profile` to inspect that profile or span on its own.

Use these commands when an aggregated profile shows that something is slow but not which profile or trace span it came from, for example when you want to:

- Inspect the single profile behind a spike in a time series, instead of an average over the whole window.
- Find the most expensive trace spans in a service and see the code that ran during them.
- Export a single span's profile as a pprof file, to compare it against a typical one.

Span-aware instrumentation records which trace span was active as it takes each sample, so one profile contains samples from many spans and querying by span returns that span's samples rather than a whole profile.
Not every profile type produces span exemplars, because the profiler has to be able to attribute each sample to a span as it takes it.
Refer to [Link tracing and profiling with Span Profiles](../../configure-client/trace-span-profiles/) for the profile types each language supports.

{{< admonition type="note" >}}
Exemplars are only supported with the v2 storage layer, and so are the `--profile-id` and `--trace-id` flags.
On a deployment that still runs v1 storage as well, the query time range must fall within the period covered by v2 storage, otherwise the query fails.
{{< /admonition >}}

#### List exemplars

The `profilecli query exemplars profile` command lists individual profiles, each identified by a profile ID, which is a UUID assigned when the profile is ingested.
The `profilecli query exemplars span` command lists trace spans, each identified by a span ID of 16 hexadecimal characters.
Both commands rank the results by value, so the most expensive profiles or spans appear first.
The same span ID can appear in more than one row when its samples fall into different time buckets. Samples that fall into the same bucket are merged into a single row and their values added together, even when they come from different instances.

To list exemplars, you need to:

1. Specify optional flags. 
1. Construct and execute the command. 

**Specify optional flags**

   - You can provide a label selector using the `--query` flag, for example, `--query='{service_name="my_application_name"}'`.
   - You can provide a custom time range using the `--from` and `--to` flags, for example, `--from="now-3h" --to="now"`.
   - You can specify the profile type via the `--profile-type` flag. The default is `process_cpu:cpu:nanoseconds:cpu:nanoseconds`.
   - You can cap how many exemplars the command prints using the `--top-n` flag. The default is `100`. The value also sets how the range is divided, because the command splits it into `--top-n` buckets and keeps the highest-value exemplar in each. For profile exemplars the buckets are time windows, so a low `--top-n` over a wide range can return far fewer exemplars than you asked for. For span exemplars each bucket is a time and value window, so one time window can contribute several spans. If the results look sparse, raise `--top-n` or narrow the time range.
   - You can set how many label columns the table shows using the `--max-label-columns` flag. The default is `3`, and `0` hides labels. The command shows the labels that vary the most between exemplars.
   - You can control the output format using `--output=table` (default) or `--output=json`. The JSON format emits an envelope containing `from`, `to`, `profile_type`, and an `exemplars` array, which is useful for scripting.

**Construct and execute the command**

 Example command for profile exemplars:
     ```bash
     profilecli query exemplars profile \
         --query='{service_name="my_application_name"}' \
         --from="now-5m" --to="now" \
         --top-n=5
     ```

Example table output (default):
     ```
     +--------------------------------------+---------------------------+---------------------+--------------+---------------+----------+
     |              Profile ID              |         Timestamp         | Value (nanoseconds) |   hostname   | pyroscope_spy |  region  |
     +--------------------------------------+---------------------------+---------------------+--------------+---------------+----------+
     | f6591ef9-8f5a-46c3-a6f7-fb28929dd111 | 2026-08-26T18:00:05+02:00 |              29.35s | 55a7b15f975a | gospy         | eu-north |
     | f41c6f0e-06dc-44ff-975b-1698933b3ab0 | 2026-08-26T17:56:36+02:00 |              26.27s | 55a7b15f975a | gospy         | eu-north |
     | f17d25d3-429e-4f17-b2d8-5c0f597b37e2 | 2026-08-26T18:00:35+02:00 |              23.44s | 55a7b15f975a | gospy         | eu-north |
     +--------------------------------------+---------------------------+---------------------+--------------+---------------+----------+
     ```

The `Value` column header names the unit of the profile type you queried, and values are formatted for that unit, so a `nanoseconds` profile type renders durations such as `29.35s` and a `bytes` profile type renders sizes such as `29 MB`. The label columns are chosen automatically.

Example command for span exemplars:
     ```bash
     profilecli query exemplars span \
         --query='{service_name="my_application_name"}' \
         --from="now-5m" --to="now" \
         --top-n=5
     ```

   - Example table output (default):
     ```
     +----------------------------------+------------------+---------------------------+---------------------+----------+
     |             Trace ID             |     Span ID      |         Timestamp         | Value (nanoseconds) |  region  |
     +----------------------------------+------------------+---------------------------+---------------------+----------+
     | 4bf92f3577b34da6a3ce929d0e0e4736 | 90cf4e12878d89ac | 2026-08-26T17:58:05+02:00 |              19.18s | eu-north |
     | 4bf92f3577b34da6a3ce929d0e0e4736 | 5985c8cff1cea41a | 2026-08-26T17:58:35+02:00 |              19.09s | eu-north |
     | 8a3d1f60b27c94e5f0a1b2c3d4e5f607 | 6433a011496b3aae | 2026-08-26T18:00:35+02:00 |              18.91s | eu-north |
     +----------------------------------+------------------+---------------------------+---------------------+----------+
     ```

Span output identifies spans rather than profiles, so it has no `Profile ID` column. The `Trace ID` column appears only when the listed spans carry trace IDs, and several spans of the same request share one trace ID. Profile exemplars never report trace IDs.

#### Drill down into a single exemplar

Pass an ID from the previous step to `profilecli query profile`, using the flag that matches the kind of ID you have.
Include a `service_name` matcher in `--query`, because narrowing the query to a single service makes these lookups considerably faster.
All three flags are repeatable, so you can inspect several profiles, spans, or traces merged together.

| Flag | Accepts | What you get back | Where to get the ID |
| --- | --- | --- | --- |
| `--profile-id` | A profile ID (UUID) | The whole profile | `profilecli query exemplars profile` |
| `--span-selector` | A span ID (16 hexadecimal characters) | Only the samples tagged with that span | `profilecli query exemplars span` |
| `--trace-id` | A trace ID (32 hexadecimal characters) | The samples of every span in that trace | `profilecli query exemplars span`, or a trace you opened in your tracing backend |

These flags select data in different ways, so you can only use one of them per query.
Trace IDs are only available when the ingested samples carry a `trace_id` label. The OpenTelemetry profiles endpoint writes that label automatically from the span link, and recent `otel-profiling-*` integrations send it. Older integrations record a span ID alone, in which case `--trace-id` returns an empty profile.

Example command:
  ```bash
  profilecli query profile \
      --query='{service_name="my_application_name"}' \
      --from="now-5m" --to="now" \
      --span-selector=90cf4e12878d89ac \
      --output=pprof=./slow-span.pprof
  ```

`profilecli` rejects the following combinations:

- `--profile-id` with `--span-selector` or `--trace-id`
- `--span-selector` with `--trace-id`, `--stacktrace-selector`, or `--async`

{{< admonition type="caution" >}}
A query returns an empty profile rather than an error when nothing matches the ID you supplied, for example when a `--span-selector` query runs against blocks that were written without per-sample span IDs.
If a query returns nothing, confirm that the ID came from an exemplar query over the same time range, and that the profiles were ingested with span-aware instrumentation.
{{< /admonition >}}

### Export a profile for Go PGO

You can use the `profilecli query go-pgo` command to retrieve an aggregated profile from a Pyroscope server for use with Go PGO.
Profiles retrieved with `profilecli query profile` include all samples found in the profile store, resulting in a large profile size.
The profile size may cause issues with network transfer and slow down the PGO process.
In contrast, profiles retrieved with `profilecli query go-pgo` include only the information used in Go PGO, making them significantly smaller and more efficient to handle.
By default, it looks for samples within the last hour, though this can be controlled with the `--from` and `--to` flags. The source data can be narrowed down with the `--query` flag in the same way as with the `query` command.

1. Specify optional flags.

    - You can provide a label selector using the `--query` flag, for example, `--query='{service_name="my_application_name"}'`.
    - You can provide a custom time range using the `--from` and `--to` flags, for example, `--from="now-3h" --to="now"`.
    - You can specify the profile type via the `--profile-type` flag. The available profile types are listed in the output of the `profilecli query series` command.
    - You can specify the number of leaf locations to keep via the `--keep-locations` flag. The default value is `5`. The Go compiler does not use the full stack trace. Reducing the number helps to minimize the profile size.
    - You can control whether to use callee aggregation with the `--aggregate-callees` flag. By default, this option is enabled, meaning samples are aggregated based on the leaf location, disregarding the callee line number, which the Go compiler does not utilize. To disable aggregation, use the `--no-aggregate-callees` flag.

2. Construct and execute the command.

    - Example command:
      ```bash
      export PROFILECLI_URL=https://profiles-prod-001.grafana.net
      export PROFILECLI_USERNAME=my_username
      export PROFILECLI_PASSWORD=my_password

      profilecli query go-pgo \
          --query='{service_name="my_service"}' \
          --from="now-1h" --to="now"
      ```

    - Example output:
      ```bash
      level=info msg="querying pprof profile for Go PGO" url=https://localhost:4040 query="{service_name=\"my_service\"}" from=2024-06-20T12:32:20+08:00 to=2024-06-20T15:24:40+08:00 type=process_cpu:cpu:nanoseconds:cpu:nanoseconds output="pprof=default.pgo" keep-locations=5 aggregate-callees=true
      # By default, the profile is saved to the current directory as `default.pgo`
      ```

## Other useful commands

The following commands are also useful in day-to-day operations.

### Find top contributors by label value

Use `profilecli query top` to rank label values, or combinations of label values, by their total profile value in a time window.
This is useful when investigating a spike and you need to identify the services, namespaces, or other dimensions that contribute the most before inspecting a profile in detail.
The command ranks grouped profile values; it does not rank functions or stack frames.

By default, `query top` queries the last hour of `process_cpu:cpu:nanoseconds:cpu:nanoseconds` profiles, groups results by `service_name`, and shows the top 10 groups.
`profilecli query top` is available in Grafana Pyroscope 1.19 and later.

1. Specify optional flags.

   - Use `--query` to filter the profiles to analyze. The default is `{}`.
   - Use `--from` and `--to` to set the time range. The defaults are `now-1h` and `now`.
   - Use `--profile-type` to select the profile type. The default is `process_cpu:cpu:nanoseconds:cpu:nanoseconds`.
   - Use the repeatable `--label-names` flag to select the labels to group by. The default is `service_name`; specifying more than one label ranks each combination of their values.
   - Use `--top-n` to set the number of groups to display. The default is `10`.
   - Use `--output=table` (default) or `--output=json`.

1. Run the command.

   - To rank services by CPU time during the last 30 minutes:
     ```bash
     profilecli query top \
         --profile-type=process_cpu:cpu:nanoseconds:cpu:nanoseconds \
         --query='{namespace="production"}' \
         --from="now-30m" --to="now" \
         --label-names=service_name \
         --top-n=10
     ```

   - To rank combinations of service and namespace:
     ```bash
     profilecli query top \
         --query='{cluster="us-east-1"}' \
         --label-names=service_name \
         --label-names=namespace \
         --top-n=5
     ```

   - Example table output:
     ```
      +------+-----------------+------------+---------------------+
      | Rank | service_name    | namespace  | Total (nanoseconds) |
      +------+-----------------+------------+---------------------+
      |    1 | checkout        | production |               1m12s |
      |    2 | payments        | production |              48.25s |
      |    3 | recommendations | production |              31.88s |
      +------+-----------------+------------+---------------------+
     ```

     The final column names the sample unit from the selected profile type. Table output formats nanoseconds as durations and bytes as sizes. A missing or empty grouping label is shown as `<unknown>`.

   - To use the ranked data in a script, request JSON output:
     ```bash
     profilecli query top \
         --query='{namespace="production"}' \
         --label-names=service_name \
         --output=json
     ```

     The JSON output contains `from`, `to`, `profile_type`, and a `series` array. Each series has a `labels` object and a numeric `total`; totals are raw values in the profile type's sample unit, rather than the formatted values shown in the table.

{{< admonition type="note" >}}
`--top-n` limits the results after `profilecli` receives and ranks all matching groups. A broad time range or a high-cardinality grouping can still produce a large query response. Narrow the label selector or time range when needed.
{{< /admonition >}}

### Detect high-cardinality labels

Use `profilecli query label-values-cardinality` to find label keys with many values.
This is useful when troubleshooting query cost, dashboard slowness, or label design issues.

```bash
profilecli query label-values-cardinality \
  --query='{service_name=~".+"}' \
  --top-n=20
```

### Check endpoint readiness quickly

Use `profilecli ready` in scripts and CI checks to verify endpoint health before running upload or query automation.

```bash
profilecli ready --url=http://localhost:4040
```

### Manage recording rules from the CLI

Use `profilecli recording-rules` commands to list, create, get, and delete recording rules without leaving your terminal.
This is useful for GitOps-style workflows and automated rollout validation.

```bash
profilecli recording-rules list
```

### Validate source mapping coverage

Use `profilecli source-code coverage` to measure how well your `.pyroscope.yaml` mappings translate symbols from a pprof profile to source files.
This is useful when source links in the UI are missing or incomplete.
The command requires GitHub API access. Provide a token with `--github-token` or `PROFILECLI_GITHUB_TOKEN`.

```bash
export PROFILECLI_GITHUB_TOKEN=<github_token>

profilecli source-code coverage \
  --profile=./cpu.pprof \
  --config=./.pyroscope.yaml \
  --output=detailed
```
