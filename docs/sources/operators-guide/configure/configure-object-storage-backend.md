---
title: "Configure Grafana Phlare object storage backend"
menuTitle: "Configure object storage"
description: "Learn how to configure Grafana Phlare to use different object storage backend implementations."
weight: 30
---

# Configure Grafana Phlare object storage backend

Grafana Phlare can use different object storage services to persist blocks containing the profiles data.
Blocks are flushed by ingesters [on disk]({{<relref "./configure-disk-storage.md">}}) first then are uploaded to object store.

> The long term storage is still in development and querying from object store is not yet implemented.

The supported backends are:

- [Amazon S3](https://aws.amazon.com/s3/) (and compatible implementations like [MinIO](https://min.io/))
- [Google Cloud Storage](https://cloud.google.com/storage)
- [Azure Blob Storage](https://azure.microsoft.com/es-es/services/storage/blobs/)
- [Swift (OpenStack Object Storage)](https://wiki.openstack.org/wiki/Swift)

> Under the hood Grafana Phlare uses [Thanos' object store client] library, so their stated limitations apply.

[Thanos' object store client]: https://github.com/thanos-io/objstore#supported-providers-clients

## Amazon S3

To use an AWS S3 or S3-compatible bucket for long term storage, you can find Grafana Phlare's configuration parameters [in the reference config][aws_ref]. Apart from those it is also possible to supply configuration using [the well-known environment variables] of the AWS SDK.

At a minimum, you will need to provide a values for the `bucket_name`, `endpoint`, `access_key_id`, and `secret_access_key` keys.

[aws_ref]: {{< relref "./reference-configuration-parameters/#s3_storage_backend" >}}
[aws_enf]: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html

### Example using an AWS Bucket

This how one would configure a bucket in the AWS region `eu-west-2`:

```yaml
storage:
 backend: s3
 s3:
   bucket_name: grafana-phlare-data
   region: eu-west-2
   endpoint: s3.eu-west-2.amazonaws.com
   access_key_id: MY_ACCESS_KEY
   secret_access_key: MY_SECRET_KEY
```

### Example using a S3 compatible Bucket

This how one would configure a bucket on a locally running instance of [MinIO]:

```yaml
storage:
 backend: s3
 s3:
   bucket_name: grafana-phlare-data
   endpoint: localhost:9000
   insecure: true
   access_key_id: grafana-phlare-data
   secret_access_key: grafana-phlare-data
```

[MinIO]: https://min.io/docs/minio/container/index.html

## Google Cloud Storage

To use a Google Cloud Storage (GCS) bucket for long term storage, you can find Grafana Phlare's configuration parameters [in the reference config][gcs_ref].

[gcs_ref]: {{< relref "./reference-configuration-parameters/#gcs_storage_backend" >}}

At a minimum, you will need to provide a values for the `bucket_name` and a service account. To supply the service account there are two ways:

* Use the `GOOGLE_APPLICATION_CREDENTIALS` environment variable to locate your [application credentials](https://cloud.google.com/docs/authentication/production).
* Provide the content the service account key within the `service_account` parameter.

### Example using a Google Cloud Storage bucket

This how one would configure a GCS bucket using the `service_account` parameter:

```yaml
storage:
  backend: gcs
  gcs:
    bucket_name: grafana-phlare-data
    service_account: |
        {
          "type": "service_account",
          "project_id": "PROJECT_ID",
          "private_key_id": "KEY_ID",
          "private_key": "-----BEGIN PRIVATE KEY-----\nPRIVATE_KEY\n-----END PRIVATE KEY-----\n",
          "client_email": "SERVICE_ACCOUNT_EMAIL",
          "client_id": "CLIENT_ID",
          "auth_uri": "https://accounts.google.com/o/oauth2/auth",
          "token_uri": "https://accounts.google.com/o/oauth2/token",
          "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
          "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/SERVICE_ACCOUNT_EMAIL"
        }
```

## Azure Blob Storage

To use a Google Cloud Storage (GCS) bucket for long term storage, you can find Grafana Phlare's configuration parameters [in the reference config][azure_ref].

[azure_ref]: {{< relref "./reference-configuration-parameters/#azure_storage_backend" >}}

If `user_assigned_id` is used, authentication is done via user-assigned managed identity.

[//TODO]: <> (Provide example with and without user-assigned managed identity)

## Swift (OpenStack Object Storage)

To use a Swift (OpenStack Object Storage) bucket for long term storage, you can find Grafana Phlare's configuration parameters [in the reference config][swift_ref].

[swift_ref]: {{< relref "./reference-configuration-parameters/#swift_storage_backend" >}}

>If the `name` of a user, project or tenant is used one must also specify its domain by ID or name. Various examples for OpenStack authentication can be found in the [official documentation](https://developer.openstack.org/api-ref/identity/v3/index.html?expanded=password-authentication-with-scoped-authorization-detail#password-authentication-with-unscoped-authorization).

[//TODO]: <> (Provide example)
