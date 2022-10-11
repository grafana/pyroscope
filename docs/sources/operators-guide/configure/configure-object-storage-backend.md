---
title: "Configure Grafana Phlare object storage backend"
menuTitle: "Configure object storage"
description: "Learn how to configure Grafana Phlare to use different object storage backend implementations."
weight: 30
---

# Configure Grafana Phlare object storage backend

Grafana Phlare can use different object storage services to persist blocks containing the profiles data.
Blocks are flushed by ingesters [on disk]({{<relref "./configure-disk-storage.md">}}) first then are uploaded to object store.
> The storage is still in development and querying from object store is not yet implemented.

The supported backends are:

- [Amazon S3](https://aws.amazon.com/s3/) (and compatible implementations like [MinIO](https://min.io/))
- [Google Cloud Storage](https://cloud.google.com/storage)
- [Azure Blob Storage](https://azure.microsoft.com/es-es/services/storage/blobs/)
- [Swift (OpenStack Object Storage)](https://wiki.openstack.org/wiki/Swift)

Additionally and for non-production testing purposes, you can use a file-system emulated [`filesystem`](https://thanos.io/tip/thanos/storage.md/#filesystem) object storage implementation.

Object storage configuration is currently only supported via the configuration files under the `storage.bucketConfig` configuration, see below for example.

> Since Grafana Phlare uses the same object storage configuration as [Thanos](https://thanos.io/) you can also refer to their [configuration section](https://thanos.io/tip/thanos/storage.md)
> for more details.

## Amazon S3

```yaml
storage:
  bucketConfig: |
    type: S3
    config:
      bucket: ""
      endpoint: ""
      region: ""
      aws_sdk_auth: false
      access_key: ""
      insecure: false
      signature_version2: false
      secret_key: ""
      put_user_metadata: {}
      http_config:
        idle_conn_timeout: 1m30s
        response_header_timeout: 2m
        insecure_skip_verify: false
        tls_handshake_timeout: 10s
        expect_continue_timeout: 1s
        max_idle_conns: 100
        max_idle_conns_per_host: 100
        max_conns_per_host: 0
        tls_config:
          ca_file: ""
          cert_file: ""
          key_file: ""
          server_name: ""
          insecure_skip_verify: false
        disable_compression: false
      trace:
        enable: false
      list_objects_version: ""
      bucket_lookup_type: auto
      part_size: 67108864
      sse_config:
        type: ""
        kms_key_id: ""
        kms_encryption_context: {}
        encryption_key: ""
      sts_endpoint: ""
    prefix: ""
```

At a minimum, you will need to provide a value for the `bucket`, `endpoint`, `access_key`, and `secret_key` keys.

### Google Cloud Storage

```yaml
storage:
  bucketConfig: |
    type: GCS
    config:
      bucket: ""
      service_account: ""
    prefix: ""
```

Use the `GOOGLE_APPLICATION_CREDENTIALS` environnement variable to locate your [application credentials](https://cloud.google.com/docs/authentication/production).

### Azure Blob Storage

```yaml
storage:
  bucketConfig: |
    type: AZURE
    config:
      storage_account: ""
      storage_account_key: ""
      container: ""
      endpoint: ""
      max_retries: 0
      msi_resource: ""
      user_assigned_id: ""
      pipeline_config:
        max_tries: 0
        try_timeout: 0s
        retry_delay: 0s
        max_retry_delay: 0s
      reader_config:
        max_retry_requests: 0
      http_config:
        idle_conn_timeout: 0s
        response_header_timeout: 0s
        insecure_skip_verify: false
        tls_handshake_timeout: 0s
        expect_continue_timeout: 0s
        max_idle_conns: 0
        max_idle_conns_per_host: 0
        max_conns_per_host: 0
        tls_config:
          ca_file: ""
          cert_file: ""
          key_file: ""
          server_name: ""
          insecure_skip_verify: false
        disable_compression: false
    prefix: ""
```

If `msi_resource` is used, authentication is done via system-assigned managed identity. The value for Azure should be `https://<storage-account-name>.blob.core.windows.net`.

If `user_assigned_id` is used, authentication is done via user-assigned managed identity. When using `user_assigned_id` the `msi_resource` defaults to `https://<storage_account>.<endpoint>`

The generic `max_retries` will be used as value for the `pipeline_config`’s `max_tries` and `reader_config`’s `max_retry_requests`. For more control, `max_retries` could be ignored (0) and one could set specific retry values.

### Swift (OpenStack Object Storage)

```yaml
storage:
  bucketConfig: |
    type: SWIFT
    config:
      auth_version: 0
      auth_url: ""
      username: ""
      user_domain_name: ""
      user_domain_id: ""
      user_id: ""
      password: ""
      domain_id: ""
      domain_name: ""
      project_id: ""
      project_name: ""
      project_domain_id: ""
      project_domain_name: ""
      region_name: ""
      container_name: ""
      large_object_chunk_size: 1073741824
      large_object_segments_container_name: ""
      retries: 3
      connect_timeout: 10s
      timeout: 5m
      use_dynamic_large_objects: false
    prefix: ""
```

>If the `name` of a user, project or tenant is used one must also specify its domain by ID or name. Various examples for OpenStack authentication can be found in the [official documentation](https://developer.openstack.org/api-ref/identity/v3/index.html?expanded=password-authentication-with-scoped-authorization-detail#password-authentication-with-unscoped-authorization).
