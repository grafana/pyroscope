---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/profile-cli/
  - /docs/phlare/latest/profile-cli/
  - /docs/pyroscope/latest/configure-server/profile-cli/
  - ../ingest-and-analyze-profile-data/profile-cli/
description: Getting started with the profile CLI tool.
menuTitle: Profile CLI
title: Profile CLI
weight: 60
---

# Profile CLI

`profilecli` is a command-line utility that enables various productivity flows such as:
- Interacting with a running Pyroscope server to upload profiles, query data, and more
- Inspecting [Parquet](https://parquet.apache.org/docs/) files

> Hint: Use the `help` command (`profilecli help`) to get a full list of capabilities as well as additional help information.

## Install Profile CLI

You can install Profile CLI using a package or by compiling the code.

### Install using a package

On macOS, you can install Profile CLI using [HomeBrew](https://brew.sh):

```bash
brew install pyroscope-io/brew/profilecli
```

For other platforms, you can manually [download the `profilecli` release asset](https://github.com/grafana/pyroscope/releases/latest) for your operating system and architecture and make it executable.

For example, for Linux with the AMD64 architecture:

1. Download and extract the package (archive).

    ```bash
    curl -fL https://github.com/grafana/pyroscope/releases/download/v1.1.5/profilecli_1.1.5_linux_amd64.tar.gz | tar xvz
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

- Go installed (> 1.19).
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

   The command places the `profilecli` executable in `$GOPATH/bin/` (or `$GOBIN/`) and make it available to use.


## Common flags and environment variables

The `profilecli` commands that interact with a Pyroscope server require a server URL and optionally authentication details. These can be provided as command-line flags or environment variables.

1. Server URL

   `default: http://localhost:4040`

   The `--url` flag specifies the server against which the command will run.
   If using Grafana Cloud, an example URL could be `https://profiles-prod-001.grafana.net`.
   For local instances, the URL could look like `http://localhost:4040`.

1. Authentication details.

   `default: <empty>`

   If using Grafana Cloud or authentication is enabled on your Pyroscope server, you will need to provide a username and password using the `--username` and `--password` flags respectively.
   For Grafana Cloud, the username will be the Stack ID and the password the generated API token.

### Environment variable naming

You can use environment variables to avoid passing flags to the command every time you use it, or to protect sensitive information.
Environment variables have a `PROFILECLI_` prefix. Here is an example of providing the server URL and credentials for the `profilecli` tool:

```bash
export PROFILECLI_URL=<pyroscope_server_url>
export PROFILECLI_USERNAME=<username>
export PROFILECLI_PASSWORD=<password>
# now we can run a profilecli command without specifying the url or credentials:
profilecli <command>
```

{{< admonition type="caution" >}}
If you're querying data from Cloud Profiles, be sure to use the url of your Cloud Profiles server in `PROFILECLI_URL` (e.g. `https://profiles-prod-001.grafana.net`) and **not** the url of your Grafana Cloud tenant (e.g. `<your tenant>.grafana.net`).
{{< /admonition >}}

## Uploading a profile to a Pyroscope server using `profilecli`

Using `profilecli` streamlines the process of uploading profiles to Pyroscope, making it a convenient alternative to manual HTTP requests.

### Prerequisites

- Ensure you have `profilecli` installed on your system by following the [installation](#install-profile-cli) steps above.
- Have a profile file ready for upload. Note that you can only upload pprof files at this time.

### Upload steps

1. Identify the pprof file.

   - Path to your pprof file: `path/to/your/pprof-file.pprof`

1. Optional: Specify any extra labels.

   - You can add additional labels to your uploaded profile using the `--extra-labels` flag.
   - You can provide the name of the application that the profile was captured from via the `service_name` label (defaults to `profilecli-upload`). This will be useful when querying the data via `profilecli` or the UI.
   - You can use the flag multiple times to add several labels.

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

1. Check for successful upload.

   - After running the command, you should see a confirmation message indicating a successful upload. If there are any issues, `profilecli` provides error messages to help you troubleshoot.

## Querying a Pyroscope server using `profilecli`

You can use the `profilecli query` command to look up the available profiles on a Pyroscope server and read actual profile data. This can be useful for debugging purposes or for integrating profiling in CI pipelines (for example to facilitate [profile-guided optimization](https://go.dev/doc/pgo)).

### Looking up available profiles on a Pyroscope server

You can use the `profilecli query series` command to look up the available profiles on a Pyroscope server.
By default, it queries the last hour of data, though this can be controlled with the `--from` and `--to` flags.
You can narrow the results down with the `--query` flag. See `profilecli help query series` for more information.

#### Query series steps

1. Optional: Specify a Query and a Time Range.

   - You can provide a label selector using the `--query` flag, for example: `--query='{service_name="my_application_name"}'`.
   - You can provide a custom time range using the `--from` and `--to` flags, for example, `--from="now-3h" --to="now"`.

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

   - Example output:
     ```json
     {
         "__name__":"memory",
         "__period_type__":"space",
         "__period_unit__":"bytes",
         "__profile_type__":"memory:inuse_objects:count:space:bytes",
         "__service_name__":"my_application_name",
         "__type__":"inuse_objects",
         "__unit__":"count",
         "cluster":"eu-west-1",
         "service_name":"my_application_name"
      }
     ```

### Reading a raw profile from a Pyroscope server

You can use the `profilecli query profile` command to retrieve a merged (aggregated) profile from a Pyroscope server.
The command merges all samples found in the profile store for the specified query and time range.
By default it looks for samples within the last hour, though this can be controlled with the `--from` and `--to` flags. The source data can be narrowed down with the `--query` flag in the same way as with the `series` command.

#### Query profile steps

1. Specify optional flags.

   - You can provide a label selector using the `--query` flag, for example, `--query='{service_name="my_application_name"}'`.
   - You can provide a custom time range using the `--from` and `--to` flags, for example, `--from="now-3h" --to="now"`.
   - You can specify the profile type via the `--profile-type` flag. The available profile types are listed in the output of the `profilecli query series` command.

2. Construct and execute the `query profile` command.

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli query profile \
         --profile-type=<profile_type> \
         --query='{<label_name>="<label_value>"' \
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

### Exporting a profile for Go PGO

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
