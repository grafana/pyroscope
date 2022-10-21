---
title: "Python"
menuTitle: "Python"
description: "Instrumenting Pyhton applications for continuous profiling"
weight: 50
---

# Python

The Python module [pypprof] adds HTTP-based endpoints similar like Go's [`net/http/pprof`] for collecting profiles from a Python application.

Under the hood, it uses [zprofile] and [mprofile] to collect CPU and heap profiles with minimal overhead.

[`net/http/pprof`]: https://golang.org/pkg/net/http/pprof/
[pypprof]: https://github.com/timpalpant/pypprof
[zprofile]: https://github.com/timpalpant/zprofile
[mprofile]: https://github.com/timpalpant/mprofile

## How to instrument your application

First of all required Python modules need to be installed:

```shell
# Add the modules to the requirements.txt file
cat >> requirements.txt <<EOF
mprofile==0.0.14
protobuf==3.20.3
pypprof==0.0.1
six==1.16.0
zprofile==1.0.12
EOF

# Build the module's wheels locally
pip3 wheel --wheel-dir=/tmp/wheels -r requirements.txt

# Install the modules
pip3 install --no-index --find-links=/tmp/wheels -r requirements.txt
```

Now the initialization code of your application should be invoking the web server exposing the profiling data:

```python
# import continous profiling modules
from pypprof.net_http import start_pprof_server
import mprofile

# start memory profiling
mprofile.start(sample_rate=128 * 1024)

# enable pprof http server
start_pprof_server(host='0.0.0.0', port=8081)
```

To test the handlers you can use the [pprof] tool:

```shell
# Profile the current heap memory usage
pprof -http :6060 "http://127.0.0.1:8081/debug/pprof/heap"

# Profile the cpu for 5 seconds
pprof -http :6060 "http://127.0.0.1:8081/debug/pprof/profile?seconds=5"
```

[pprof]: https://github.com/google/pprof
