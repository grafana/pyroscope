---
title: "Pyroscope v2 deployment modes"
menuTitle: "Deployment modes"
description: "Learn about the deployment options for Pyroscope v2."
weight: 40
keywords:
  - Pyroscope v2
  - deployment
  - Kubernetes
  - microservices
---

# Pyroscope v2 deployment modes

Pyroscope v2 can be deployed in different configurations depending on your scale and operational requirements.

## Microservices mode

In microservices mode, each component runs as a separate process. This is the recommended deployment for production environments at scale.

### Benefits

- **Independent scaling**: Scale each component based on its specific load
- **Fault isolation**: Component failures don't affect other components
- **Resource optimization**: Allocate resources based on component needs
- **Rolling updates**: Update components independently

### Components to deploy

| Component | Instances | Stateful | Notes |
|-----------|-----------|----------|-------|
| Distributor | 2+ | No | Scale based on ingestion rate |
| Segment-writer | 2+ | No | Scale based on ingestion rate |
| Metastore | 3 or 5 | Yes | Odd number for Raft consensus |
| Compaction-worker | 2+ | No | Scale based on compaction backlog |
| Query-frontend | 2+ | No | Scale based on query load |
| Query-backend | 2+ | No | Scale based on query load |

### Object storage requirement

Microservices mode requires object storage (Amazon S3, Google Cloud Storage, Azure Blob Storage, or OpenStack Swift). Local filesystem storage is not supported in this mode.

## Single-node mode

For evaluation, development, or small-scale deployments, Pyroscope v2 can run as a single process with all components enabled.

### Benefits

- **Simple deployment**: Single binary to run
- **Lower resource requirements**: Suitable for smaller workloads
- **Local storage option**: Can use local filesystem for storage

### Limitations

- **No high availability**: Single point of failure
- **Limited scalability**: Cannot scale individual components
- **Not recommended for production**: Use microservices mode for production workloads

## Kubernetes deployment

For Kubernetes deployments, use the Helm chart with the values file located in the `tools/dev/v2` directory.

### Getting started

Clone the repository and deploy using Helm with v2 configuration:

```bash
git clone https://github.com/grafana/pyroscope.git

helm install pyroscope ./pyroscope \
  -f tools/dev/v2/values.yaml
```

### Helm chart considerations

When deploying on Kubernetes:

- Configure persistent volumes for metastore nodes
- Set up object storage credentials
- Configure resource requests and limits for each component
- Set up ingress for distributor and query-frontend

## Storage configuration

### Object storage

Pyroscope v2 supports the following object storage backends:

- **Amazon S3**: Recommended for AWS deployments
- **Google Cloud Storage**: Recommended for GCP deployments
- **Azure Blob Storage**: Recommended for Azure deployments
- **OpenStack Swift**: For OpenStack environments

### Local filesystem

Local filesystem storage is only supported for single-node deployments and is not suitable for production use in microservices mode.

## Resource planning

### Metastore

The metastore is the only component requiring persistent storage:

- **Disk space**: A few gigabytes, even at large scale
- **Memory**: Benefits from keeping the index in memory
- **CPU**: Moderate usage for Raft consensus operations

### Stateless components

All other components are stateless and primarily need:

- **CPU**: For data processing
- **Memory**: For in-flight data and query execution
- **Network**: For object storage access

### Object storage

Plan for object storage costs based on:

- **Write operations**: Segment flushes and compaction uploads
- **Read operations**: Query execution
- **Storage**: Retained profile data
