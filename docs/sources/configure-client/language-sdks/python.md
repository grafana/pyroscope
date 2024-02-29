---
title: "Python"
menuTitle: "Python"
description: "Instrumenting Python applications for continuous profiling."
weight: 40
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/python
---

# Python

Our Python Profiler, when integrated with Pyroscope, transforms the way you analyze and optimize Python applications. This combination provides unparalleled real-time insights into your Python codebase, allowing for precise identification of performance issues. It is an essential tool for Python developers focused on enhancing code efficiency and application speed.

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pryoscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add Python profiling to your application

Install the `pyroscope-io` pip package:

```bash
pip install pyroscope-io
```

## Configure the Python client

Add the following code to your application. This code will initialize the Pyroscope profiler and start profiling:

```python
import pyroscope

pyroscope.configure(
  application_name = "my.python.app", # replace this with some name for your application
  server_address   = "http://my-pyroscope-server:4040", # replace this with the address of your Pyroscope server
)
```

Optionally, you can configure several additional parameters:

```python
import pyroscope

pyroscope.configure(
    application_name    = "my.python.app", # replace this with some name for your application
    server_address      = "http://my-pyroscope-server:4040", # replace this with the address of your Pyroscope server
    sample_rate         = 100, # default is 100
    detect_subprocesses = False, # detect subprocesses started by the main process; default is False
    oncpu               = True, # report cpu time only; default is True
    gil_only            = True, # only include traces for threads that are holding on to the Global Interpreter Lock; default is True
    enable_logging      = True, # does enable logging facility; default is False
    tags                = {
        "region": '{os.getenv("REGION")}',
    }
)
```

## Add profiling labels to Python applications

You can add tags to certain parts of your code:

```python
# You can use a wrapper:
with pyroscope.tag_wrapper({ "controller": "slow_controller_i_want_to_profile" }):
    slow_code()
```

## Sending data to Pyroscope OSS or Grafana Cloud Profiles with Python SDK


```python
import pyroscope

pyroscope.configure(
    application_name = "example.python.app",
    server_address = "<URL>",
    basic_auth_username = '<User>',
    basic_auth_password = '<Password>',
    # Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana Cloud.
    # tenant_id = "<TenantID>",
)
```

To configure the Python SDK to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.

## Python profiling examples

Check out the following resources to learn more about Python profiling:
- [Python examples](https://github.com/pyroscope-io/pyroscope/tree/main/examples/language-sdk-instrumentation/python)
- [Python demo](https://demo.pyroscope.io/?query=rideshare-app-python.cpu%7B%7D) showing Python example with tags
