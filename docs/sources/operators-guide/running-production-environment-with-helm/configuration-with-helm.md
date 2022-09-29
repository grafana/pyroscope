---
title: "Manage the configuration of Grafana Fire with Helm"
menuTitle: "Manage the configuration of Grafana Fire with Helm"
description: "Learn how to customize, secure and update the Grafana Fire configuration using the Helm chart."
weight: 80
---

# Manage the configuration of Grafana Fire with Helm

The `fire-distributed` Helm chart provides interfaces to set Grafana Fire [configuration parameters]({{< relref "../configure/reference-configuration-parameters/" >}}) and customize how Grafana Fire is deployed on a Kubernetes cluster. This document describes the configuration parameters.

## Overview

The Grafana Fire configuration can be managed through the Helm chart or supplied via a user-managed object.

If you want to manage the configuration via the Helm chart, see [Manage the configuration with Helm](#manage-the-configuration-with-helm).

If you want to manage the configuration externally yourself, see [Manage the configuration externally](#manage-the-configuration-externally).

Handling sensitive information, such as credentials, is common between the two methods, see [Injecting credentials](#injecting-credentials).

## Manage the configuration with Helm

There are three ways configuration parameters can be modified:

1. Setting parameters via the `fire.structuredConfig` value (recommended)
1. Copying the whole `fire.config` value and modifying the configuration as text (discouraged, unless you want to prevent upgrades of the chart from automatically updating the configuration)
1. Setting extra CLI flags for components individually (discouraged, except for setting availability zone)

See the [Example](#example-of-configuration-managed-with-helm) for a practical application.

> **Limitation:** it is not possible to delete configuration parameters via `fire.structuredConfig` that were set in `fire.config`. Set the configuration parameter to its default or to some other value instead.

### How the configuration is applied

Grafana Fire components are run with a configuration calculcated by the following process:

1. The configuration YAML in `fire.config` is evaluated as a Helm template. This step ensures that the configuration applies to the Kubernetes cluster where it will be installed. For example, setting up cluster-specific addresses.
1. The values from `fire.structuredConfig` are recursively merged with `fire.config`. The values from `fire.structuredConfig` take precedence over the values in `fire.config`. The result is again evaluated as a Helm template. This step applies user-specific customizations. For example, S3 storage details.
1. The resulting YAML configuration is then sorted alphabetically and stored in a `ConfigMap` (or `Secret` depending on the value of `configStorageType`) and provided to all Grafana Fire components.
1. The configuration file as well as any extra CLI flags are provided to the Fire pods.
1. Each component evaluates the configuration, substituting environment variables as required. Note that extra CLI flags take precedence over the configuration file.

> **Note:** CLI flags are component-specific, thus they will not show up in the generated `ConfigMap` (or `Secret`), making it less obvious what configuration is running. Use only when absolutely necessary.

### Inspect changes to the configuration before upgrade

Follow these steps to inspect what change will be applied to the configuration.

Preparation:

1. Install the [helm diff](https://github.com/databus23/helm-diff) plugin.
1. Set `configStorageType` value to `ConfigMap`.

Inspecting changes with the `helm diff` sub command:

```bash
helm -n fire-test diff upgrade grafana/fire-distributed -f custom.yaml
```

This command shows the differences between the running installation and the installation that would result from executing the `helm upgrade` command. Search for `name: fire-config` in the output to see the difference in configuration settings. See [Example output of helm diff command](#example-output-of-helm-diff-command) for a concrete example.

> **Note:** CLI flags and their difference are found in the `Deployment` and `StatefulSet` objects.

## Manage the configuration externally

Prepare the configuration as text. It cannot include Helm template functions or value evaluations. The configuration may include references to environment variables as explained in [Injecting credentials](#injecting-credentials).

Decide whether you want to use a `ConfigMap` or `Secret` to store the configuration. Handling `ConfigMap` is a little bit simpler, but beware of sensitive information.

### Use external ConfigMap

Prepare a `ConfigMap` object where the configuration is placed under the `fire.yaml` data key.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-fire-config
data:
  fire.yaml: |
    <configuration>
```

Replace `<configuration>` with the configuration as multiline text, be mindful of indentation. The name `my-fire-config` is just an example.

Set the following values in your custom values file (or on the Helm command line):

```yaml
useExternalConfig: true
externalConfigSecretName: my-fire-config
externalConfigVersion: "0"
```

### Use external Secret

Prepare a `Secret` object where the configuration is base64-encoded and placed under the `fire.yaml` data key.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-fire-config
data:
  fire.yaml: <configuration-base64>
```

Replace `<configuration-base64>` with the configuration encoded as base64 format string. The name `my-fire-config` is just an example.

Set the following values in your custom values file (or on the Helm command line):

```yaml
useExternalConfig: true
externalConfigSecretName: my-fire-config
configStorageType: Secret
externalConfigVersion: "0"
```

### Update the configuration

To make components aware of configuration changes, either:

- Update the value in `externalConfigVersion` and run `helm update`
- or restart components affected by the configuration change manually.

## Injecting credentials

You can use the Helm chart value `global.extraEnvFrom` to inject credentials into the runtime environment variables of the Grafana Fire components. The data keys will become environment variables and usable in the Grafana Fire configuration. For example, `AWS_SECRET_ACCESS_KEY` can be referenced as `${AWS_SECRET_ACCESS_KEY}` in the configuration. See the [Example](#example-of-configuration-managed-with-helm) for a practical application.

Grafana Fire does not track changes to the credentials. If the credentials change, Grafana Fire pods should be restarted to use the new value. To trigger a restart, provide a global [pod annotation](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/) in `global.podAnnotation` which will be applied to all Grafana Fire pods. Changing the value of the global annotation will make Kubernetes recreate all pods. For example, changing `global.podAnnotations.bucketSecretVersion` from `"0"` to `"1"` triggers a restart. Note that pod annotations can only be strings.

## Example of configuration managed with Helm

This example shows how to set up the configuration to use an S3 bucket for blocks storage. We assume that the namespace in use is called `fire-test`.

1. Set up the external blocks storage, in this case S3 with buckets named for example `my-blocks-bucket`, `my-ruler-bucket` and in case of Grafana Enterprise Profiles `my-admin-bucket`.

1. Create an external secret with the S3 credentials by writing the following to a `mysecret.yaml` file:

   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: fire-bucket-secret
   data:
     AWS_ACCESS_KEY_ID: FAKEACCESSKEY
     AWS_SECRET_ACCESS_KEY: FAKESECRETKEY
   ```

   Replace `FAKEACCESSKEY` and `FAKESECRETKEY` with the actual value encoded in base64.

1. Apply the secret to your cluster with the `kubectl` command:

   ```bash
   kubectl -n fire-test apply -f mysecret.yaml
   ```

1. Prepare your custom values file called `custom.yaml`:

   ```yaml
   global:
     extraEnvFrom:
       - secretRef:
           name: fire-bucket-secret
     podAnnotations:
       bucketSecretVersion: "0"

   # This turns of the built-in MinIO support
   minio:
     enabled: false

   fire:
     structuredConfig:
       # Uncomment when using Grafana Enterprise Profiles
       # admin_client:
       #   storage:
       #     s3:
       #       bucket_name: my-admin-bucket
       #       access_key_id: ${AWS_ACCESS_KEY_ID}
       #       endpoint: s3.amazonaws.com
       #       secret_access_key: ${AWS_SECRET_ACCESS_KEY}
       alertmanager_storage:
         s3:
           bucket_name: my-ruler-bucket
           access_key_id: ${AWS_ACCESS_KEY_ID}
           endpoint: s3.amazonaws.com
           secret_access_key: ${AWS_SECRET_ACCESS_KEY}
       blocks_storage:
         backend: s3
         s3:
           bucket_name: my-blocks-bucket
           access_key_id: ${AWS_ACCESS_KEY_ID}
           endpoint: s3.amazonaws.com
           secret_access_key: ${AWS_SECRET_ACCESS_KEY}
       ruler_storage:
         s3:
           bucket_name: my-ruler-bucket
           access_key_id: ${AWS_ACCESS_KEY_ID}
           endpoint: s3.amazonaws.com
           secret_access_key: ${AWS_SECRET_ACCESS_KEY}
   ```

1. Check the resulting configuration with the `helm` command before installing:

   ```bash
   helm -n fire-test template fire grafana/fire-distributed -f custom.yaml -s templates/fire-config.yaml
   ```

   You should see the following output:

   ```yaml
   ---
   # Source: fire-distributed/templates/fire-config.yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
   name: fire-config
   labels:
     helm.sh/chart: fire-distributed-3.0.0
     app.kubernetes.io/name: fire
     app.kubernetes.io/instance: fire
     app.kubernetes.io/version: "2.2.0"
     app.kubernetes.io/managed-by: Helm
   namespace: "fire-test"
   data:
   fire.yaml: |

     activity_tracker:
       filepath: /data/profiles-activity.log
     alertmanager:
       data_dir: /data
       enable_api: true
       external_url: /alertmanager
     alertmanager_storage:
       s3:
         access_key_id: ${AWS_ACCESS_KEY_ID}
         bucket_name: my-ruler-bucket
         endpoint: s3.amazonaws.com
         secret_access_key: ${AWS_SECRET_ACCESS_KEY}
     blocks_storage:
       backend: s3
       bucket_store:
         sync_dir: /data/tsdb-sync
       s3:
         access_key_id: ${AWS_ACCESS_KEY_ID}
         bucket_name: my-blocks-bucket
         endpoint: s3.amazonaws.com
         secret_access_key: ${AWS_SECRET_ACCESS_KEY}
       tsdb:
         dir: /data/tsdb
     compactor:
       data_dir: /data
     frontend:
       align_queries_with_step: true
       log_queries_longer_than: 10s
     frontend_worker:
       frontend_address: fire-query-frontend-headless.test.svc:9095
     ingester:
       ring:
         final_sleep: 0s
         num_tokens: 512
         unregister_on_shutdown: false
     ingester_client:
       grpc_client_config:
         max_recv_msg_size: 104857600
         max_send_msg_size: 104857600
     limits: {}
     memberlist:
       abort_if_cluster_join_fails: false
       compression_enabled: false
       join_members:
       - dns+fire-gossip-ring.test.svc.cluster.local:7946
     ruler:
       alertmanager_url: dnssrvnoa+http://_http-profiles._tcp.fire-alertmanager-headless.test.svc.cluster.local/alertmanager
       enable_api: true
       rule_path: /data
     ruler_storage:
       s3:
         access_key_id: ${AWS_ACCESS_KEY_ID}
         bucket_name: my-ruler-bucket
         endpoint: s3.amazonaws.com
         secret_access_key: ${AWS_SECRET_ACCESS_KEY}
     runtime_config:
       file: /var/fire/runtime.yaml
     server:
       grpc_server_max_concurrent_streams: 1000
   ```

1. Install the chart with the `helm` command:

   ```bash
   helm -n fire-test install fire grafana/fire-distributed -f custom.yaml
   ```

## Example output of helm diff command

The example is generated with the following steps:

1. Install Grafana Fire with the `helm` command:

   ```bash
   helm -n test install fire grafana/fire-distributed --version 3.0.0
   ```

1. Create a `custom.yaml` file with the following content:

   ```yaml
   fire:
   structuredConfig:
     alertmanager:
       external_url: https://example.com/alerts
     server:
       log_level: debug
   ```

1. Produce the diff with the `helm` command:

   ```bash
   helm -n test diff upgrade fire grafana/fire-distributed --version 3.0.0  -f custom.yaml
   ```

   The output is an excerpt of the real output to reduce the size:

   ```
   #... cut for size ...

   test, fire-config, ConfigMap (v1) has changed:
     # Source: fire-distributed/templates/fire-config.yaml
     apiVersion: v1
     kind: ConfigMap
     metadata:
       name: fire-config
       labels:
         helm.sh/chart: fire-distributed-3.0.0
         app.kubernetes.io/name: fire
         app.kubernetes.io/instance: fire
         app.kubernetes.io/version: "2.2.0"
         app.kubernetes.io/managed-by: Helm
       namespace: "test"
     data:
       fire.yaml: |
          activity_tracker:
           filepath: /data/profiles-activity.log
         alertmanager:
           data_dir: /data
           enable_api: true
   -       external_url: /alertmanager
   +       external_url: https://example.com/alerts
         alertmanager_storage:
           backend: s3
           s3:
             access_key_id: grafana-fire
             bucket_name: fire-ruler
             endpoint: fire-minio.test.svc:9000
             insecure: true
             secret_access_key: supersecret
         blocks_storage:
           backend: s3
           bucket_store:
             sync_dir: /data/tsdb-sync
           s3:
             access_key_id: grafana-fire
             bucket_name: fire-tsdb
             endpoint: fire-minio.test.svc:9000
             insecure: true
             secret_access_key: supersecret
           tsdb:
             dir: /data/tsdb
         compactor:
           data_dir: /data
         frontend:
           align_queries_with_step: true
           log_queries_longer_than: 10s
         frontend_worker:
           frontend_address: fire-query-frontend-headless.test.svc:9095
         ingester:
           ring:
             final_sleep: 0s
             num_tokens: 512
             unregister_on_shutdown: false
         ingester_client:
           grpc_client_config:
             max_recv_msg_size: 104857600
             max_send_msg_size: 104857600
         limits: {}
         memberlist:
           abort_if_cluster_join_fails: false
           compression_enabled: false
           join_members:
           - fire-gossip-ring
         ruler:
           alertmanager_url: dnssrvnoa+http://_http-profiles._tcp.fire-alertmanager-headless.test.svc.cluster.local/alertmanager
           enable_api: true
           rule_path: /data
         ruler_storage:
           backend: s3
           s3:
             access_key_id: grafana-fire
             bucket_name: fire-ruler
             endpoint: fire-minio.test.svc:9000
             insecure: true
             secret_access_key: supersecret
         runtime_config:
           file: /var/fire/runtime.yaml
         server:
           grpc_server_max_concurrent_streams: 1000
   +       log_level: debug

   #... cut for size ...

   test, fire-distributor, Deployment (apps) has changed:
     # Source: fire-distributed/templates/distributor/distributor-dep.yaml
     apiVersion: apps/v1
     kind: Deployment
     metadata:
       name: fire-distributor
       labels:
         helm.sh/chart: fire-distributed-3.0.0
         app.kubernetes.io/name: fire
         app.kubernetes.io/instance: fire
         app.kubernetes.io/component: distributor
         app.kubernetes.io/part-of: memberlist
         app.kubernetes.io/version: "2.2.0"
         app.kubernetes.io/managed-by: Helm
       annotations:
         {}
       namespace: "test"
     spec:
       replicas: 1
       selector:
         matchLabels:
           app.kubernetes.io/name: fire
           app.kubernetes.io/instance: fire
           app.kubernetes.io/component: distributor
       strategy:
         rollingUpdate:
           maxSurge: 0
           maxUnavailable: 1
         type: RollingUpdate
       template:
         metadata:
           labels:
             helm.sh/chart: fire-distributed-3.0.0
             app.kubernetes.io/name: fire
             app.kubernetes.io/instance: fire
             app.kubernetes.io/version: "2.2.0"
             app.kubernetes.io/managed-by: Helm
             app.kubernetes.io/component: distributor
             app.kubernetes.io/part-of: memberlist
           annotations:
   -         checksum/config: bad33a421a56693ebad68b64ecf407b5e897c3679b1a33b65672dbc4e98e918f
   +         checksum/config: 02f080c347a1fcd6c9e49a38280330378d3afe12efc7151cd679935c96b35b83
           namespace: "test"
         spec:
           serviceAccountName: fire
           securityContext:
             {}
           initContainers:
             []
           containers:
             - name: distributor
               image: "grafana/fire:2.2.0"
               imagePullPolicy: IfNotPresent
               args:
                 - "-target=distributor"
                 - "-config.expand-env=true"
                 - "-config.file=/etc/fire/fire.yaml"
               volumeMounts:
                 - name: config
                   mountPath: /etc/fire
                 - name: runtime-config
                   mountPath: /var/fire
                 - name: storage
                   mountPath: "/data"
                   subPath:
               ports:
                 - name: http-profiles
                   containerPort: 8080
                   protocol: TCP
                 - name: grpc
                   containerPort: 9095
                   protocol: TCP
                 - name: memberlist
                   containerPort: 7946
                   protocol: TCP
               livenessProbe:
                 null
               readinessProbe:
                 httpGet:
                   path: /ready
                   port: http-profiles
                 initialDelaySeconds: 45
               resources:
                 requests:
                   cpu: 100m
                   memory: 512Mi
               securityContext:
                 readOnlyRootFilesystem: true
               env:
               envFrom:
           nodeSelector:
             {}
           affinity:
             podAntiAffinity:
               requiredDuringSchedulingIgnoredDuringExecution:
               - labelSelector:
                   matchExpressions:
                   - key: target
                     operator: In
                     values:
                     - distributor
                 topologyKey: kubernetes.io/hostname
           tolerations:
             []
           terminationGracePeriodSeconds: 60
           volumes:
             - name: config
               configMap:
                 name: fire-config
                 items:
                   - key: "fire.yaml"
                     path: "fire.yaml"
             - name: runtime-config
               configMap:
                 name: fire-runtime
             - name: storage
               emptyDir: {}
    #... cut for size ...
   ```

   Lines starting with "`-`" were removed and lines starting with "`+`" were added. The change to the annotation `checksum/config` means the pods will be restarted when this change is applied.
