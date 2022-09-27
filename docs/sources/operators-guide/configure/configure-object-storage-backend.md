---
title: "Configure Grafana Mimir object storage backend"
menuTitle: "Configure object storage"
description: "Learn how to configure Grafana Mimir to use different object storage backend implementations."
weight: 65
---

# Configure Grafana Mimir object storage backend

Grafana Mimir can use different object storage services to persist blocks containing the metrics data, as well as recording rules and alertmanager state.
The supported backends are:

- [Amazon S3](https://aws.amazon.com/s3/) (and compatible implementations like [MinIO](https://min.io/))
- [Google Cloud Storage](https://cloud.google.com/storage)
- [Azure Blob Storage](https://azure.microsoft.com/es-es/services/storage/blobs/)
- [Swift (OpenStack Object Storage)](https://wiki.openstack.org/wiki/Swift)

Additionally and for non-production testing purposes, you can use a file-system emulated [`filesystem`]({{< relref "../configure/reference-configuration-parameters/index.md#filesystem_storage_backend" >}}) object storage implementation.

[Ruler and alertmanager support a `local` implementation]({{< relref "../architecture/components/ruler/index.md#local-storage" >}}),
which is similar to `filesystem` in the way that it uses the local file system,
but it is a read-only data source and can be used to provision state into those components.

## Common configuration

To avoid repetition, you can use the [common configuration]({{< relref "about-configurations.md#common-configurations" >}}) and fill the [`common`]({{< relref "../configure/reference-configuration-parameters/index.md#common" >}}) configuration block or by providing the `-common.storage.*` CLI flags.

> **Note:** Blocks storage cannot be located in the same path of the same bucket as the ruler and alertmanager stores. When using the common configuration, make [`blocks_storage`]({{< relref "../configure/reference-configuration-parameters/index.md#blocks_storage" >}}) use either a:

- different bucket, overriding the common bucket name
- storage prefix

Grafana Mimir will fail to start if you configure blocks storage to use the same bucket and storage prefix that the alertmanager or ruler store uses.

A valid configuration for the object storage (taken from the ["Play with Grafana Mimir" tutorial](https://grafana.com/tutorials/play-with-grafana-mimir/)) looks as follows:

```yaml
common:
  storage:
    backend: s3
    s3:
      endpoint: minio:9000
      access_key_id: mimir
      secret_access_key: supersecret
      insecure: true
      bucket_name: mimir

blocks_storage:
  storage_prefix: blocks
```

> **Note**: If you're using a mixture of YAML files and CLI flags, pay attention to their [precedence logic]({{< relref "about-configurations.md#common-configurations" >}}).
