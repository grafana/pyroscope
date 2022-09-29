---
aliases:
  - /docs/mimir/latest/operators-guide/deploying-grafana-mimir/jsonnet/deploying/
description: Learn how to deploy Grafana Mimir on Kubernetes with Jsonnet and Tanka.
menuTitle: Deploying with Jsonnet
title: Deploying Grafana Mimir with Jsonnet and Tanka
weight: 10
---

# Deploying Grafana Mimir with Jsonnet and Tanka

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
   - Install Grafana Mimir and Kubernetes Jsonnet libraries
   - Set up an environment

   <!-- prettier-ignore-start -->

   [embedmd]: # "../../../../../operations/mimir/getting-started.sh"

   ```sh
   #!/usr/bin/env bash
   # SPDX-License-Identifier: AGPL-3.0-only

   set -e

   # Initialise the Tanka.
   mkdir jsonnet-example && cd jsonnet-example
   tk init --k8s=1.21

   # Install Mimir jsonnet.
   jb install github.com/grafana/mimir/operations/mimir@main

   # Use the provided example.
   cp vendor/mimir/mimir-manifests.jsonnet.example environments/default/main.jsonnet

   # Generate the YAML manifests.
   export PAGER=cat
   tk show environments/default
   ```

   <!-- prettier-ignore-end -->

1. Generate the Kubernetes YAML manifests and store them in the `./manifests` directory:

   <!-- prettier-ignore-start -->

   ```sh
   # Generate the YAML manifests:
   export PAGER=cat
   tk show environments/default
   tk export manifests environments/default
   ```

   <!-- prettier-ignore-end -->

1. Configure the environment specification file at `environments/default/spec.json`.

   To learn about how to use Tanka and to configure the `spec.json` file, see [Using Jsonnet: Creating a new project](https://tanka.dev/tutorial/jsonnet).

1. Deploy the manifests to a Kubernetes cluster, in one of two ways:

   - **Use the `tk apply` command**.

     Tanka supports commands to show the `diff` and `apply` changes to a Kubernetes cluster:

     ```sh
     # Show the difference between your Jsonnet definition and your Kubernetes cluster:
     tk diff environments/default

     # Apply changes to your Kubernetes cluster:
     tk apply environments/default
     ```

   - **Use the `kubectl apply` command**.

     You generated the Kubernetes manifests and stored them in the `./manifests` directory in the previous step.

     You can run the following command to directly apply these manifests to your Kubernetes cluster:

     ```sh
     # Review the changes that will apply to your Kubernetes cluster:
     kubectl apply --dry-run=client -k manifests/

     # Apply the changes to your Kubernetes cluster:
     kubectl apply -k manifests/
     ```

   > **Note**: The generated Kubernetes manifests create resources in the `default` namespace. To use a different namespace, change the `namespace` configuration option in the `environments/default/main.jsonnet` file, and re-generate the Kubernetes manifests.
