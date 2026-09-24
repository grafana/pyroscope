---
description: Learn how to deploy Pyroscope on Kubernetes with Jsonnet and Tanka.
keywords:
  - Pyroscope deployment
  - Kubernetes
  - Jsonnet
  - Tanka
menuTitle: Deploy with Jsonnet and Tanka
title: Deploy Pyroscope with Jsonnet and Tanka
weight: 60
aliases:
  - /docs/phlare/latest/operators-guide/deploying-grafana-phlare/jsonnet/
  - /docs/phlare/latest/deploy-kubernetes/tanka-jsonnet/
---

# Deploy Pyroscope with Jsonnet and Tanka

Grafana Labs publishes a [Jsonnet](https://jsonnet.org/) library that you can use to deploy Pyroscope.
The Jsonnet files are located in the [Pyroscope repository](https://github.com/grafana/pyroscope/tree/main/operations/pyroscope/jsonnet) and render the [Helm chart](helm/) as Kubernetes manifests.

The chart defaults to the [v2 storage architecture](../reference-pyroscope-v2-architecture/about-pyroscope-v2-architecture/) and installs [Grafana Alloy](https://grafana.com/docs/alloy/latest/) to discover and scrape profile endpoints from annotated pods.

## Install tools and deploy the first cluster

You can use [Tanka](https://tanka.dev/) and [jsonnet-bundler](https://github.com/jsonnet-bundler/jsonnet-bundler) to generate Kubernetes YAML manifests from the jsonnet files.

1. Install `tanka` and `jb`:

   Follow the steps at [https://tanka.dev/install](https://tanka.dev/install). If you have `go` installed locally you can also use:

   ```console
   # make sure to be outside of GOPATH or a go.mod project
   go install github.com/grafana/tanka/cmd/tk@latest
   go install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@latest
   ```

1. Set up a Jsonnet project, based on the example that follows:

   - Initialize Tanka
   - Install Pyroscope and Kubernetes Jsonnet libraries
   - Set up an environment

   ```console
   # Initialize a Tanka directory
   mkdir jsonnet-example && cd jsonnet-example
   tk init --k8s=1.21

   # Install Pyroscope jsonnet
   jb install github.com/grafana/pyroscope/operations/pyroscope@main

   # Install required tanka-util
   jb install github.com/grafana/jsonnet-libs/tanka-util@master

   # Setup your current cluster as the server for the default environment
   tk env set environments/default --server-from-context=$(kubectl config current-context)
   ```

1. Decide if you want to run Pyroscope as a single binary or in microservices mode.

   - Option A: For single-binary mode, the file `environments/default/main.jsonnet` should look like this:

     ```jsonnet
     local pyroscope = import 'pyroscope/jsonnet/pyroscope/pyroscope.libsonnet';
     local tk = import 'tk';

     pyroscope.new(overrides={
       namespace: tk.env.spec.namespace,
     })
     ```

   - Option B: For microservices mode, the file `environments/default/main.jsonnet` should look like this. This enables the bundled MinIO object store for local development and testing:

     ```jsonnet
     local pyroscope = import 'pyroscope/jsonnet/pyroscope/pyroscope.libsonnet';
     local tk = import 'tk';

     pyroscope.new(overrides={
       namespace: tk.env.spec.namespace,
       values+: {
         architecture+: {
           microservices+: {
             enabled: true,
           },
         },
         minio+: {
           enabled: true,
         },
       },
     })
     ```
1. Generate the Kubernetes YAML manifests and store them in the `./manifests` directory:

   ```console
   # Take a look at the generated YAML manifests.
   tk show environments/default

   # Export the YAML manifests to the folder `./manifests`:
   tk export ./manifests environments/default
   ```

1. Deploy the manifests to a Kubernetes cluster, in one of two ways:

   - **Use the `tk apply` command**.

     Tanka supports commands to show the `diff` and `apply` changes to a Kubernetes cluster:

     ```console
     # Show the difference between your Jsonnet definition and your Kubernetes cluster:
     tk diff environments/default

     # Apply changes to your Kubernetes cluster:
     tk apply environments/default
     ```

   - **Use the `kubectl apply` command**.

     You generated the Kubernetes manifests and stored them in the `./manifests` directory in the previous step.

     You can run the following command to directly apply these manifests to your Kubernetes cluster:

     ```console
     # Review the changes that will apply to your Kubernetes cluster:
     kubectl apply --dry-run=client -k manifests/

     # Apply the changes to your Kubernetes cluster:
     kubectl apply -k manifests/
     ```

   {{% admonition type="note" %}}
   The generated Kubernetes manifests create resources in the `default` namespace. To use a different namespace, change the `namespace` configuration option in the `environments/default/main.jsonnet` file, and re-generate the Kubernetes manifests.
   {{% /admonition %}}

## Next steps

- To query profiles in Grafana after deployment, refer to [Deploy Pyroscope using the Helm chart](helm/#query-profiles-in-grafana). Use `http://pyroscope-query-frontend.<NAMESPACE>.svc.cluster.local.:4040/` as the data source URL in microservices mode.
- If you installed an older v1 microservices deployment and need to move to v2 storage, refer to [Migrate from v1 to v2 storage using Helm](../reference-pyroscope-v2-architecture/migrate-from-v1/).
