<!-- generate-sample-config:server:md -->
| Name | Default Value | Usage |
| :- | :- | :- |
| analytics-opt-out | false | "disables analytics" |
| log-level | info | "log level: debug|info|warn|error" |
| badger-log-level | error | "log level: debug|info|warn|error" |
| storage-path | /var/lib/pyroscope | "directory where pyroscope stores profiling data" |
| api-bind-addr | :4040 | "port for the HTTP server used for data ingestion and web UI" |
| base-url |  | "base URL for when the server is behind a reverse proxy with a different path" |
| cache-dimension-size | 1000 | "max number of elements in LRU cache for dimensions" |
| cache-dictionary-size | 1000 | "max number of elements in LRU cache for dictionaries" |
| cache-segment-size | 1000 | "max number of elements in LRU cache for segments" |
| cache-tree-size | 1000 | "max number of elements in LRU cache for trees" |
| badger-no-truncate | false | "indicates whether value log files should be truncated to delete corrupt data, if any" |
| max-nodes-serialization | 2048 | "max number of nodes used when saving profiles to disk" |
| max-nodes-render | 8192 | "max number of nodes used to display data on the frontend" |
| hide-applications |  | "please don't use, this will soon be deprecated" |
| out-of-space-threshold | 512.00 MB | "Threshold value to consider out of space in bytes" |
| sample-rate | 100 | "sample rate for the profiler in Hz. 100 means reading 100 times per second" |
<!-- /generate-sample-config -->

<!-- generate-sample-config:server:yaml -->
```yaml
---
# disables analytics
analytics-opt-out: "false"

# log level: debug|info|warn|error
log-level: "info"

# log level: debug|info|warn|error
badger-log-level: "error"

# directory where pyroscope stores profiling data
storage-path: "/var/lib/pyroscope"

# port for the HTTP server used for data ingestion and web UI
api-bind-addr: ":4040"

# base URL for when the server is behind a reverse proxy with a different path
base-url: ""

# max number of elements in LRU cache for dimensions
cache-dimension-size: "1000"

# max number of elements in LRU cache for dictionaries
cache-dictionary-size: "1000"

# max number of elements in LRU cache for segments
cache-segment-size: "1000"

# max number of elements in LRU cache for trees
cache-tree-size: "1000"

# indicates whether value log files should be truncated to delete corrupt data, if any
badger-no-truncate: "false"

# max number of nodes used when saving profiles to disk
max-nodes-serialization: "2048"

# max number of nodes used to display data on the frontend
max-nodes-render: "8192"

# please don't use, this will soon be deprecated
hide-applications: ""

# Threshold value to consider out of space in bytes
out-of-space-threshold: "512.00 MB"

# sample rate for the profiler in Hz. 100 means reading 100 times per second
sample-rate: "100"

```
<!-- /generate-sample-config -->
