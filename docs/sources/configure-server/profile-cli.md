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
- interacting with a running Pyroscope Server to upload profiles manually, run queries, etc.
- inspecting [Parquet](https://parquet.apache.org/docs/) files.

Hint: Use the `help` command (`profilecli help`) to get a full list of capabilities as well as additional help information.

# Installation

## MacOS

```bash
brew install pyroscope-io/brew/profilecli
```

## Other

Download the `profilecli`` release asset from https://github.com/grafana/pyroscope/releases/latest for your operating system and architecture and make it executable.

For example, for Linux with the AMD64 architecture:

```bash
curl -fL https://github.com/grafana/pyroscope/releases/download/v1.0.0/profilecli_1.0.0_linux_amd64.tar.gz | tar xvz
```

This command will place the `profilecli` executable in the current directory.

## Build from source code

### Prerequesites

- Make sure you have Go installed (> 1.19).
- Make sure either `$GOPATH` or `$GOBIN` is configured and added to your `PATH` environment variable.

### Build and install

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

# Uploading a pprof File to a Pyroscope Server using profilecli

Using `profilecli` streamlines the process of uploading profiles to Pyroscope, making it a convenient alternative to manual HTTP requests.

## Prerequisites

- Ensure you have `profilecli` installed on your system by following the installation steps above.
- Have the pprof file you want to upload ready.

## Upload steps

1. **Identify the pprof file and Pyroscope server details.**

   - Path to your pprof file: `path/to/your/pprof-file.pprof`
   - Pyroscope server URL: If using Grafana Cloud, for example, `https://profiles-prod-001.grafana.net`. For local instances, it could be `http://localhost:4040`.

1. **Determine Authentication Details (if applicable).**

   - If using cloud or authentication is enabled on your Pyroscope server, you will need the following:
     - Username: `<username>`
     - Password: `<password>`

1. **Specify any Extra Labels (Optional).**

   - You can add additional labels to your uploaded profile using the `--extra-labels` flag.
   - The name of the application that the profile was captured from can be provided via the `service_name` label (defaults to `profilecli-upload`).
   - The flag can be used multiple times to add several labels.

1. **Construct and Execute the Upload Command.**

   - Here's a basic command template:
     ```bash
     profilecli upload \
         --url=<pyroscope_server_url> \
         --username=<username> \
         --password=<password> \
         --extra-labels=<label_name>=<label_value> \
         <pprof_file_path>
     ```

   - Modify the placeholders (`<pyroscope_server_url>`, `<username>`, etc.) with your actual values.

   - Example command:
     ```bash
     profilecli upload \
         --url=https://profiles-prod-001.grafana.net \
         --username=my_username \
         --password=my_password \
         path/to/your/pprof-file.pprof
     ```

   - Example command with extra labels:
     ```bash
     profilecli upload \
         --url=https://profiles-prod-001.grafana.net \
         --username=my_username \
         --password=my_password \
         --extra-labels=service_name=my_application_name \
         --extra-labels=cluster=us \
         path/to/your/pprof-file.pprof
     ```

1. **Check for Successful Upload.**

   - After running the command, you should see a confirmation message indicating a successful upload. If there are any issues, `profilecli` will provide error messages to help you troubleshoot.
