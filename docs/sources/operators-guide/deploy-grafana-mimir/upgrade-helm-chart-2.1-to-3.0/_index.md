---
aliases:
  - /docs/mimir/latest/operators-guide/deploying-grafana-mimir/upgrade-helm-chart-2.1-to-3.0/
description: "Upgrade the Grafana Mimir Helm chart from version 2.1 to 3.0"
title: "Upgrade the Grafana Mimir Helm chart from version 2.1 to 3.0"
menuTitle: "Upgrade Helm chart 2.1 to 3.0"
weight: 100
---

# Upgrade the Grafana Mimir Helm chart from version 2.1 to 3.0

There are breaking changes between the Grafana Mimir Helm chart versions 2.1 and 3.0.
Several parameters that were available in version 2.1 of the mimir-distributed Helm chart have changed.

**To upgrade from Helm chart 2.1 to 3.0:**

1. Understand the improvements that we made to the Mimir configuration in the Helm chart:

   - The Mimir configuration is now stored in a Kubernetes ConfigMap by default, instead of a Kubernetes Secret.
   - You can override individual properties without copying the entire `mimir.config` block. Specify properties you want to override under the `mimir.structuredConfig`.
   - You can move secrets outside the Mimir configuration via external secrets and environment variables. Environment variables can be used to externalize secrets from the configuration file.

1. Decide whether or not you need to update the Mimir configuration:

   - If you are using external configuration (`useExternalConfig: true`), then you must set `configStorageType: Secret`.

     > **Note:** It is now possible to use a ConfigMap to manage your external configuration instead.
     > If your external configuration contains secrets, then you can externalize them and use a ConfigMap. See _Externalize secrets_.

   - If you are not using external configuration (`useExternalConfig: false`), and your Mimir configuration contains secrets, chose one of two options:

     - Keep the previous location as-is by setting `configStorageType: Secret`.
     - Externalize secrets:

       1. Move secrets from the Mimir configuration to a [Kubernetes Secret](https://kubernetes.io/docs/concepts/configuration/secret/#working-with-secrets).
       2. Mount the Kubernetes Secret via `global.extraEnvFrom`:

          ```yaml
          global:
            extraEnvFrom:
              - secretRef:
                  name: mysecret
          ```

          For more information, see [Secrets - Use case: As container environment variables](https://kubernetes.io/docs/concepts/configuration/secret/#use-case-as-container-environment-variables).

       3. Replace the values in the Mimir configuration with environment variables.

          For example:

          ```yaml
          mimir:
            structuredConfig:
              blocks_storage:
                s3:
                  secret_access_key: ${AWS_SECRET_ACCESS_KEY}
          ```

   - If you are not using an external configuration (`useExternalConfig: false`), and your Mimir configuration does not contain secrets, then the storage location is automatically changed by Helm and you do not need to do anything.

   See [Example migrated values file](#example-of-migrated-values).

1. Update your memcached configuration via your customized Helm chart values, if needed:

   The mimir-distributed Helm chart supports multiple cache types.
   If you have not enabled any memcached caches,
   and you are not overriding the values of `memcached`,
   `memcached-queries`,
   `memcached-metadata`,
   or `memcached-results` sections,
   then you do not need to update the memcached configuration.

   Otherwise, check to see if you need to change any of the following configuration parameters:

   - The `memcached` section was repurposed, and `chunks-cache` was added.
   - The contents of the `memcached` section now contain the following common values that are shared across all memcached instances: `image`, `podSecurityContext`, and `containerSecurityContext`.
   - The following sections were renamed:
     - `memcached-queries` is now `index-cache`
     - `memcached-metadata` is now `metadata-cache`
     - `memcached-results` is now `results-cache`
   - The `memcached*.replicaCount` values were renamed:
     - `memcached.replicaCount` is now `chunks-cache.replicas`
     - `memcached-queries.replicaCount` is now `index-cache.replicas`
     - `memcached-metadata.replicaCount` is now `metadata-cache.replicas`
     - `memcached-results.replicaCount` is now `results-cache.replicas`
   - The `memcached*.architecture` values were removed.
   - The `memcached*.arguments` values were removed.
   - The default arguments are now encoded in the Helm chart templates; the values `*-cache.allocatedMemory`, `*-cache.maxItemMemory` and `*-cache.port` control the arguments `-m`, `-I` and `-u`. To provide additional arguments, use `*-cache.extraArgs`.
   - The `memcached*.metrics` values were consolidated under `memcachedExporter`.

   See also an [example of migration of customized memcached values between versions 2.1 and 3.0](#example-of-migration-of-customized-memcached-values-between-versions-21-and-30).

1. Update your memcached-related Mimir configuration
   via your customized Helm chart value that is named `mimir.config`, if needed:

   The configuration parameters for memcached `addresses` and `max_item_size` have changed in the default `mimir.config` value.
   If you previously copied the value of `mimir.config` into your values file, then take the latest version of the `memcached` configuration in the `mimir.config` from the `values.yaml` file in the Helm chart.

1. (Conditional) If you have enabled `serviceMonitor`, or you are overriding the value of anything under the `serviceMonitor` section, or both, then move the `serviceMonitor` section under `metaMonitoring`.

1. Update the `rbac` section, based on the following changes:

   - If you are not overriding the value of anything under the `rbac` section, then skip this step.
   - The `rbac.pspEnabled` value was removed.
   - To continue using Pod Security Policy (PSP), set `rbac.create` to `true` and `rbac.type` to `psp`.
   - To start using Security Context Constraints (SCC) instead of PSP, set `rbac.create` to `true` and `rbac.type` to `scc`.

1. Update the `mimir.config` value, based on the following information:

   - Compare your overridden value of `mimir.config` with the one in the `values.yaml` file in the chart. If you are not overriding the value of `mimir.config`, then skip this step.

1. Decide whether or not to update the `nginx` configuration:

   - Unless you have overridden the value of `nginx.nginxConfig.file`,
     and you are using the default `mimir.config`, then skip this step.
   - Otherwise, compare the overridden `nginx.nginxConfig.file` value
     to the one in the `values.yaml` file in the Helm chart,
     and incorporate the differences.
     Pay attention to the sections that contain `x_scope_orgid`.
     The value in the `values.yaml` file contains Nginx configuration
     that adds the `X-Scope-OrgId` header to incoming requests that do not already set it.

     > **Note:** This change allows Mimir clients to keep sending requests without needing to specify a tenant ID, even though multi-tenancy is now enabled by default.

## Example of migrated values

The example values file is compatible with version 2.1 of the mimir-distributed Helm chart, and demonstrates a few things:

- All memcached caches are enabled.
- The default pod security policy is disabled.
- ServiceMonitors are enabled.
- Object storage credentials for block storage are specified directly in the `mimir.config` value.
  > **Note:** The unmodified parts of the default `mimir.config` are omitted for brevity, even though in a valid 2.1 values file they need to be included.

```yaml
rbac:
  pspEnabled: false

memcached:
  enabled: true
  replicaCount: 1

memcached-queries:
  enabled: true
  replicaCount: 1

memcached-metadata:
  enabled: true
  replicaCount: 1

memcached-results:
  enabled: true
  replicaCount: 1

serviceMonitor:
  enabled: true

mimir:
  config: |-
    #######
    # default contents omitted for brevity
    #######

    blocks_storage:
      backend: s3
      s3:
        endpoint: s3.amazonaws.com
        bucket_name: my-blocks-bucket
        access_key_id: FAKEACCESSKEY
        secret_access_key: FAKESECRETKEY

    #######
    # default contents omitted for brevity
    #######
```

After applying the migration steps listed in this guide,
you now have a Kubernetes Secret that contains
the S3 credentials, and a values file for version 3.0.
The values file is does not have any omissions.
The parts that were omitted in the 2.1 version are automatically included by the Helm chart in version 3.0.

Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mimir-bucket-secret
data:
  AWS_ACCESS_KEY_ID: FAKEACCESSKEY
  AWS_SECRET_ACCESS_KEY: FAKESECRETKEY
```

Values file:

```yaml
rbac:
  create: false

chunks-cache:
  enabled: true
  replicas: 1

index-cache:
  enabled: true
  replicas: 1

metadata-cache:
  enabled: true
  replicas: 1

results-cache:
  enabled: true
  replicas: 1

metaMonitoring:
  serviceMonitor:
    enabled: true

mimir:
  structuredConfig:
    blocks_storage:
      backend: s3
      s3:
        access_key_id: ${AWS_ACCESS_KEY_ID}
        bucket_name: my-blocks-bucket
        endpoint: s3.amazonaws.com
        secret_access_key: ${AWS_SECRET_ACCESS_KEY}

global:
  extraEnvFrom:
    - secretRef:
        name: mimir-bucket-secret
```

## Example of migration of customized memcached values between versions 2.1 and 3.0

Version 2.1:

```yaml
memcached:
  replicaCount: 12
  arguments:
    - -m 2048
    - -I 128m
    - -u 12345
  image:
    repository: memcached
    tag: 1.6.9-alpine

memcached-queries:
  replicaCount: 3
  architecture: modern
  image:
    repository: memcached
    tag: 1.6.9-alpine
```

Version 3.0:

```yaml
memcached:
  image:
    repository: memcached
    tag: 1.6.9-alpine

chunks-cache:
  allocatedMemory: 2048
  maxItemMemory: 128
  port: 12345
  replicas: 12

index-cache:
  replicas: 3
```
