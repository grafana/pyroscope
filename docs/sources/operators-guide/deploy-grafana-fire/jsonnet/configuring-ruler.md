---
aliases:
  - /docs/mimir/latest/operators-guide/deploying-grafana-mimir/jsonnet/configuring-ruler/
description: Learn how to configure the Grafana Mimir ruler when using Jsonnet.
menuTitle: Configuring ruler
title: Configuring the Grafana Mimir ruler with Jsonnet
weight: 20
---

# Configuring the Grafana Mimir ruler with Jsonnet

The ruler is an optional component and is therefore not deployed by default when using Jsonnet.
For more information about the ruler, see [Grafana Mimir ruler]({{< relref "../../architecture/components/ruler/index.md" >}}).

To enable it, add the following Jsonnet code to the `_config` section:

```jsonnet
_config+:: {
  ruler_enabled: true
  ruler_client_type: '<type>',
}
```

The `ruler_client_type` option must be one of either `local`, `azure`, `aws`, or `s3`.
For more information about the options available for storing ruler state, see [Grafana Mimir ruler: State]({{< relref "../../architecture/components/ruler/index.md#state" >}}).

To get started, use the `local` client type for initial testing:

```jsonnet
_config+:: {
  ruler_enabled: true
  ruler_client_type: 'local',
  ruler_local_directory: '/path/to/local/directory',
}
```

If you are using object storage, additional configuration options are required:

- Amazon S3 (`s3`)

  - `ruler_storage_bucket_name`
  - `aws_region`

- Google Cloud Storage (`gcs`)

  - `ruler_storage_bucket_name`

- Azure (`azure`)
  - `ruler_storage_bucket_name`
  - `ruler_storage_azure_account_name`
  - `ruler_storage_azure_account_key`

> **Note:** You need to manually provide the storage credentials for `s3` and `gcs` by using additional command line arguments as necessary. For more information, see [Grafana Mimir configuration parameters: ruler_storage]({{< relref "../../configure/reference-configuration-parameters/index.md#ruler_storage" >}}).

## Operational modes

The ruler has two operational modes: _internal_ and _remote_. By default, the Jsonnet deploys the ruler by using the internal operational mode.
For more information about these modes, see [Operational modes]({{< relref "../../architecture/components/ruler/index.md#operational-modes" >}}).

To enable the remote operational mode, add the following code to the Jsonnet:

```jsonnet
_config+:: {
  ruler_remote_evaluation_enabled: true
}
```

> **Note:** To support the _remote_ operational mode, a separate query path is deployed to evaluate rules that consist of three additional Kubernetes deployments:
>
> - `ruler-query-frontend`
> - `ruler-query-scheduler`
> - `ruler-querier`

### Migrate to remote evaluation

To perform a zero downtime migration from internal to remote rule evaluation, follow these steps:

1. Deploy the following changes to enable remote evaluation in migration mode.
   Doing so causes the three new and previously listed Kubernetes deployments to start. However, they will not reconfigure the ruler to use them just yet.

   ```jsonnet
   _config+:: {
     ruler_remote_evaluation_enabled: true
     ruler_remote_evaluation_migration_enabled: true
   }
   ```

1. Check that all of pods for the following deployments have successfully started before moving to the next step:

   - `ruler-query-frontend`
   - `ruler-query-scheduler`
   - `ruler-querier`

1. Reconfigure the ruler pods to perform remote evaluation, by deploying the following changes:

   ```jsonnet
   _config+:: {
     ruler_remote_evaluation_enabled: true
   }
   ```
