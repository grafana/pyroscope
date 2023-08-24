---
title: "Python"
menuTitle: "Python"
description: "Instrumenting Python applications for continuous profiling"
weight: 40
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/python
---

# Python

## How to add Python profiling to your application

Install the `pyroscope-io` pip package:

```bash
pip install pyroscope-io
```

## Pyroscope Python pip package configuration

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
  log_level           = "info", # default is info, possible values: trace, debug, info, warn, error and critical
  tags           = {
    "region":   '{os.getenv("REGION")}',
  }
)
```

## How to add profiling labels to Python applications

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
  # tenant_id only needed if multi-tenancy enabled,
	# tenant_id = "<TenantID>",
)
```

To configure the Python SDK to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.

## Python profiling examples

Check out the following resources to learn more about Python profiling:
- [Python examples](https://github.com/pyroscope-io/pyroscope/tree/main/examples/python)
- [Python demo](https://demo.pyroscope.io/?query=rideshare-app-python.cpu%7B%7D) showing Python example with tags
