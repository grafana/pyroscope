---
title: "Python"
menuTitle: "Python"
description: "Instrumenting Python applications for continuous profiling."
weight: 40
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/python
---

# Python

The Python profiler, when integrated with Pyroscope, transforms the way you analyze and optimize Python applications.
This combination provides unparalleled real-time insights into your Python codebase, allowing for precise identification of performance issues
It's an essential tool for Python developers focused on enhancing code efficiency and application speed.

{{< admonition type="note" >}}
Refer to [Available profiling types](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/profile-types/) for a list of profile types supported by each language.
{{< /admonition >}}

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pyroscope instance with Grafana Cloud Profiles](/products/cloud/profiles/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

### Profiling on macOS

macOS has a feature called System Integrity Protection (SIP) that prevents even the root user from reading memory from any binary located in system folders.

The easiest way to avoid interference from SIP, is by installing a Python distribution into your home folder. This can be achieved for example by using `pyenv`:

```bash
# Setup pyenv
brew update
brew install pyenv
echo 'export PYENV_ROOT="$HOME/.pyenv"' >> ~/.zshrc
echo '[[ -d $PYENV_ROOT/bin ]] && export PATH="$PYENV_ROOT/bin:$PATH"' >> ~/.zshrc
echo 'eval "$(pyenv init - zsh)"' >> ~/.zshrc
#  Restart your shell
exec "$SHELL"
# Install Python 3.12
pyenv install 3.12
```

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
import os
import pyroscope

pyroscope.configure(
    application_name    = "my.python.app", # replace this with some name for your application
    server_address      = "http://my-pyroscope-server:4040", # replace this with the address of your Pyroscope server
    sample_rate         = 100, # default is 100
    cpu_enabled         = True, # enable CPU profiling; default is True
    oncpu               = True, # report cpu time only; default is True
    gil_only            = True, # only include traces for threads that are holding on to the Global Interpreter Lock; default is True
    mem_enabled         = True, # enable memory profiling; default is False
    mem_max_nframe      = 128, # maximum number of frames in memory allocation stack traces; default is 128
    mem_heap_sample_size = 512 * 1024, # average number of bytes between memory samples; default is 512 KiB
    mem_enable_mem_domain = True, # include the Python memory allocator domain on Python 3.12 and later; default is True
    enable_logging      = True, # does enable logging facility; default is False
    tags                = {
        "region": f'{os.getenv("REGION")}',
    },
)
```

{{< admonition type="caution" >}}
If your application forks processes, initialize the Python client after the fork.
Refer to [Use the Python client with forked processes](#use-the-python-client-with-forked-processes) for details.
{{< /admonition >}}

## Configure memory profiling

Memory profiling is disabled by default.
Set `mem_enabled=True` to collect the following profile types:

* Allocated objects: Estimated number of allocated objects.
* Allocated space: Estimated number of allocated bytes.
* Objects in use: Estimated number of live objects.
* Space in use: Estimated number of live bytes.

The memory profiler samples allocations made after `pyroscope.configure()` starts it.
CPU profiling remains enabled when you enable memory profiling.

To collect memory profiles without starting the CPU sampler, set `cpu_enabled=False`:

```python
import pyroscope

pyroscope.configure(
    application_name="my.python.app",
    server_address="http://my-pyroscope-server:4040",
    cpu_enabled=False,
    mem_enabled=True,
)
```

At least one of `cpu_enabled` or `mem_enabled` must be `True`.
The client rejects configurations that disable both profiling types.

The following options control CPU and memory profiling:

| Option | Default | Description |
| --- | --- | --- |
| `cpu_enabled` | `True` | Enables CPU profiling. Disable it to avoid starting the CPU sampler when collecting memory profiles only. |
| `mem_enabled` | `False` | Enables memory profiling. |
| `mem_max_nframe` | `128` | Sets the maximum number of frames captured for each sampled allocation. Valid values are from `1` through `600`. |
| `mem_heap_sample_size` | `512 * 1024` | Sets the average number of allocated bytes between samples. Smaller values provide more detail but increase profiler overhead. |
| `mem_enable_mem_domain` | `True` | On Python 3.12 and later, tracks allocations from the Python memory allocator domain in addition to the object allocator domain. This option has no effect on earlier Python versions. |

{{< admonition type="note" >}}
Memory profiling isn't available in free-threaded Python builds.
{{< /admonition >}}

## Add profiling labels to Python applications

You can add tags to certain parts of your code:

```python
# You can use a wrapper:
with pyroscope.tag_wrapper({ "controller": "slow_controller_i_want_to_profile" }):
    slow_code()
```

## Use the Python client with forked processes

The Python client starts background threads when you call `pyroscope.configure()`.
The client isn't fork-safe after those threads have started.
If a process calls `fork()` after configuring the client, the child can inherit locks and other synchronization state from threads that no longer exist.
This can cause the child to deadlock or behave unpredictably.

{{< admonition type="caution" >}}
Call `pyroscope.configure()` only after the last fork in each process that you want to profile.
Don't initialize the client in a parent process that later forks.
{{< /admonition >}}

For pre-fork application servers such as Gunicorn, initialize Pyroscope in a post-fork hook.
For example:

```python
# gunicorn.conf.py
import os

import pyroscope

preload_app = True


def post_fork(server, worker):
    pyroscope.configure(
        application_name="my.python.app",
        server_address=os.getenv(
            "PYROSCOPE_SERVER_ADDRESS",
            "http://my-pyroscope-server:4040",
        ),
    )
```

Start Gunicorn with the configuration file:

```bash
gunicorn --config gunicorn.conf.py myapp.wsgi:application
```

When `preload_app` is enabled, Gunicorn imports application modules in the parent process before it forks workers.
Keep `pyroscope.configure()` out of those modules and call it only from `post_fork`.
The same rule applies to other pre-fork servers and direct uses of `os.fork()`: initialize the client separately in each child after the fork.

If you can't avoid forking after profiling has started, stop all code that interacts with the Pyroscope client, call `pyroscope.shutdown()`, perform the fork, and call `pyroscope.configure()` again in each process that you want to profile.
You must synchronize this sequence with every thread that can use the client.
Prefer initializing after the fork because forking a multithreaded process can also be unsafe for libraries other than Pyroscope.

For a runnable configuration, refer to the [Django and Gunicorn example](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/python/rideshare/django).

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

### Locate the URL, user, and password in Grafana Cloud Profiles

[//]: # 'Shared content for URl location in Grafana Cloud Profiles'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/locate-url-pw-user-cloud-profiles.md'

{{< docs/shared source="pyroscope" lookup="locate-url-pw-user-cloud-profiles.md" version="latest" >}}

## Python profiling examples

Check out the following resources to learn more about Python profiling:
- [Python examples](https://github.com/pyroscope-io/pyroscope/tree/main/examples/language-sdk-instrumentation/python) demonstrating how Django, Flask, and FastAPI apps can be profiled with Pyroscope.
- A [Python demo](https://play.grafana.org/a/grafana-pyroscope-app/explore?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=flame-graph&var-serviceName=pyroscope-rideshare-python&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-dataSource=grafanacloud-profiles) on play.grafana.org.
