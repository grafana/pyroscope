---
aliases:
  - /docs/mimir/latest/operators-guide/securing/encrypting-data-at-rest/
description: Learn how to configure object storage encryption.
menuTitle: Encrypting data at rest
title: Encrypting Grafana Mimir data at rest
weight: 30
---

# Encrypting Grafana Mimir data at rest

Grafana Mimir supports encrypting data at rest in object storage using server-side encryption (SSE).
Configuration of SSE depends on your storage backend.

## Google Cloud Storage

Google Cloud Storage (GCS) encrypts data before writing it to disk. SSE is enabled by default and you cannot turn it off.
For more information about GCS encryption at rest, refer to [Data encryption options](https://cloud.google.com/storage/docs/encryption/).
Grafana Mimir requires no additional configuration to use GCS with SSE.

## AWS S3

Configuring SSE with AWS S3 requires configuration in the Grafana Mimir S3 client.
The S3 client is only used when the storage backend is `s3`.
Grafana Mimir supports the following AWS S3 SSE modes:

- [Server-Side Encryption with Amazon S3-Managed Keys (SSE-S3)](https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingServerSideEncryption.html)
- [Server-Side Encryption with KMS keys Stored in AWS Key Management Service (SSE-KMS)](https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingKMSEncryption.html)

You can configure AWS S3 SSE globally or for specific tenants.

### Configuring AWS S3 SSE globally

Configuring AWS S3 SSE globally requires setting SSE for each of the following storage backends:

- [alertmanager_storage]({{< relref "../configure/reference-configuration-parameters/index.md#alertmanager_storage" >}})
- [blocks_storage]({{< relref "../configure/reference-configuration-parameters/index.md#blocks_storage" >}})
- [ruler_storage]({{< relref "../configure/reference-configuration-parameters/index.md#ruler_storage" >}})

For more information about AWS S3 SSE configuration parameters, refer to [s3_storage_backend]({{< relref "../configure/reference-configuration-parameters/index.md#s3_storage_backend" >}}).

The following code sample shows a snippet of a Grafana Mimir configuration file with every backend storage configured to use AWS S3 SSE with and Amazon S3-managed key.

```yaml
alertmanager_storage:
  backend: "s3"
  s3:
    sse:
      type: "SSE-S3"
blocks_storage:
  backend: "s3"
  s3:
    sse:
      type: "SSE-S3"
ruler_storage:
  backend: "s3"
  s3:
    sse:
      type: "SSE-S3"
```

### Configuring AWS S3 SSE for a specific tenant

You can use the following settings to override AWS S3 SSE for each tenant:

- **`s3_sse_type`**<br />
  S3 server-side encryption type.
  This setting must be applied to enable the SSE configuration override for a given tenant.
- **`s3_sse_kms_key_id`**<br />
  S3 server-side encryption KMS Key ID.
  This setting is ignored if the SSE type override is not set or the type is not `SSE-KMS`.
- **`s3_sse_kms_encryption_context`**<br />
  S3 server-side encryption KMS encryption context.
  If this setting is not applied, and the key ID override is set, the encryption context is not be provided to S3.
  This setting is ignored if the SSE type override is not set or the type is not `SSE-KMS`.

**To configure AWS S3 SSE for a specific tenant**:

1. Ensure Grafana Mimir uses a runtime configuration file by verifying that the flag `-runtime-config.file` is set to a non-null value.
   For more information about supported runtime configuration parameters, refer to [Runtime configuration]({{< relref "../configure/about-runtime-configuration.md" >}}).
1. In the runtime configuration file, apply the `overrides.<TENANT>` SSE settings.

   A partial runtime configuration file that has AWS S3 SSE with Amazon S3-managed keys set for a tenant called "tenant-a" appears as follows:

   ```yaml
   overrides:
     "tenant-a":
       s3_sse_type: "SSE-S3"
   ```

1. Save and deploy the runtime configuration file.
1. After the `-runtime-config.reload-period` has elapsed, components reload the runtime configuration file and use the updated configuration.

## Other storage

Other storage backends might support encryption at rest if it is configured at the storage level.
