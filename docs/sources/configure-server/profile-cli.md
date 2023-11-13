---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/profile-cli/
  - /docs/phlare/latest/configure-server/profile-cli/
description: Getting started with the profile CLI tool.
menuTitle: Profile CLI
title: Profile CLI
weight: 80
---

# Profile CLI

`profilecli` is a command-line utility that enables various productivity flows such as:
- interacting with a running Pyroscope server to upload profiles manually, run queries, etc.
- inspecting [Parquet](https://parquet.apache.org/docs/) files.

> Hint: Use the `help` command (`profilecli help`) to get a full list of capabilities as well as additional help information.

## Installation

### MacOS

```bash
brew install pyroscope-io/brew/profilecli
```

### Other

Download the `profilecli`` release asset from https://github.com/grafana/pyroscope/releases/latest for your operating system and architecture and make it executable.

For example, for Linux with the AMD64 architecture:

```bash
curl -fL https://github.com/grafana/pyroscope/releases/download/v1.1.5/profilecli_1.1.5_linux_amd64.tar.gz | tar xvz
```

This command will place the `profilecli` executable in the current directory.

### Build from source code

#### Prerequesites

- Make sure you have Go installed (> 1.19).
- Make sure either `$GOPATH` or `$GOBIN` is configured and added to your `PATH` environment variable.

#### Build and install

1. **Clone the repository**

   ```bash
   git clone git@github.com:grafana/pyroscope.git
   ```

1. **Run the Go install command to build and install the package**

   ```bash
   cd pyroscope
   go install ./cmd/profilecli
   ```

   The command will place the `profilecli` executable in `$GOPATH/bin/` (or `$GOBIN/`) and make it available to use.

## Common flags, environment variables.

`profilecli` commands that interact with a Pyroscope server require a server URL and optionally authentication details. These can be provided as command line flags or environment variables.

1. **Server URL**

   The `--url` flag specifies the server against which the command will run. If using Grafana Cloud, an example URL could be `https://profiles-prod-001.grafana.net`. For local instances, the URL could look like `http://localhost:4040` (this the default URL used if the `--url` flag is omitted).

1. **Authentication details**

   If using Grafana Cloud or authentication is enabled on your Pyroscope server, you will need to provide a username and password using the `--username` and `--password` flags respectively. For Grafana Cloud, the username will be the Stack ID and the password the generated API token.

### Environment variable naming

Environment variables are in uppercase and have the `PROFILECLI_` prefix. Here is an example of providing the server URL and credentials for the `profilecli` tool:

```bash
export PROFILECLI_USERNAME=<username>
export PROFILECLI_PASSWORD=<password>
export PROFILECLI_URL=<pyroscope_server_url>
# now we can run a profilecli command without specifying the url or credentials:
profilecli <command>
```

## Uploading a pprof file to a Pyroscope server using `profilecli`

Using `profilecli` streamlines the process of uploading profiles to Pyroscope, making it a convenient alternative to manual HTTP requests.

### Prerequisites

- Ensure you have `profilecli` installed on your system by following the installation steps above.
- Have the pprof file you want to upload ready.

### Upload steps

1. **Identify the pprof file.**

   - Path to your pprof file: `path/to/your/pprof-file.pprof`

1. **Specify any extra labels (optional).**

   - You can add additional labels to your uploaded profile using the `--extra-labels` flag.
   - You can provide the name of the application that the profile was captured from via the `service_name` label (defaults to `profilecli-upload`). This will be useful when querying the data via `profilecli` or the UI.
   - You can use the flag multiple times to add several labels.

1. **Construct and execute the Upload command.**

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli upload --extra-labels=<label_name>=<label_value> <pprof_file_path>
     ```

   - Modify the placeholders (`<pyroscope_server_url>`, `<username>`, etc.) with your actual values.

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
         --extra-labels=cluster=us \
         path/to/your/pprof-file.pprof
     ```

1. **Check for successful upload.**

   - After running the command, you should see a confirmation message indicating a successful upload. If there are any issues, `profilecli` will provide error messages to help you troubleshoot.

## Querying a Pyroscope server using `profilecli`

You can use the `profilecli query` command to look up the available profiles on a Pyroscope server and read actual profile data. This can be useful for debugging purposes or for integrating profiling in CI pipelines (for example to facilitate [Profile-guided Optimization](https://go.dev/doc/pgo)).

### Looking up available profiles on a Pyroscope server

You can use the `profilecli query series` command to look up the available profiles on a Pyroscope server. By default it queries the last hour of data, though this can be controlled with the `--from` and `--to` flags. You can narrow the results down with the `--query` flag. See `profilecli help query series` for more information.

#### Query Series Steps

1. **Specify a Query and a Time Range (optional).**

   - You can provide a label selector using the `--query` flag (e.g., `--query='{service_name="my_application_name"}'`)
   - You can provide a custom time range using the `--from` and `--to` flags (e.g., `--from="now-3h" --to="now"`)

1. **Construct and execute the Query Series command.**

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli query series --query=<label_name>=<label_value>
     ```

   - Modify the placeholders (`<pyroscope_server_url>`, `<username>`, etc.) with your actual values.

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

You can use the `profilecli query merge` command to retrieve a merged (aggregated) profile from a Pyroscope server. The command merges all samples found in the profile store for the specified query time range. By default it looks for samples within the last hour, though this can be controlled with the `--from` and `--to` flags. The source data can be narrowed down with the `--query` flag, in the same way as with the `series` command.

#### Query Merge Steps

1. **Specify optional flags.**

   - You can provide a label selector using the `--query` flag (e.g., `--query='{service_name="my_application_name"}'`)
   - You can provide a custom time range using the `--from` and `--to` flags (e.g., `--from="now-3h" --to="now"`)
   - You can specify the profile type via the `--profile-type` flag. The available profile types are listed in the output of the `profilecli query series` command.

1. **Construct and execute the Query Merge command.**

   - Here's a basic command template:
     ```bash
     export PROFILECLI_URL=<pyroscope_server_url>
     export PROFILECLI_USERNAME=<username>
     export PROFILECLI_PASSWORD=<password>

     profilecli query merge \
         --query=<label_name>=<label_value>
         --profile-type=<profile_type>
     ```

   - Modify the placeholders (`<pyroscope_server_url>`, `<username>`, etc.) with your actual values.

   - Example command:
     ```bash
     export PROFILECLI_URL=https://profiles-prod-001.grafana.net
     export PROFILECLI_USERNAME=my_username
     export PROFILECLI_PASSWORD=my_password

     profilecli query merge \
         --query='{service_name="my_application_name"}' \
         --profile-type=memory:inuse_space:bytes:space:bytes
     ```
