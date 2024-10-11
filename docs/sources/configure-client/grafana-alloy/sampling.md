---
title: "Sampling scrape targets"
menuTitle: "Sampling targets"
description: "Sampling scrape targets with Grafana Alloy"
weight: 30
---

# Sampling scrape targets

Applications often have many instances deployed.
While Pyroscope is designed to handle large amounts of profiling data, you may want only a subset of the application's instances to be scraped.

For example, the volume of profiling data your application generates may make it unreasonable to profile every instance, or you might be targeting cost-reduction.

Through configuration of Grafana Alloy collector, Pyroscope can sample scrape targets.

## Before you begin

Make sure you understand how to configure the collector to scrape targets and are familiar with the component configuration language.
Alloy configuration files use the Alloy [configuration syntax](https://grafana.com/docs/alloy/latest/concepts/configuration-syntax/).

## Configuration

The `hashmod` action and the `modulus` argument are used in conjunction to enable sampling behavior by sharding one or more labels.
To read further on these concepts, refer to [rule block documentation](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.relabel/#rule-block).
In short, `hashmod` performs an MD5 hash on the source labels and `modulus` performs a modulus operation on the output.

The sample size can be modified by changing the value of `modulus` in the `hashmod` action and the `regex` argument in the `keep` action.
The `modulus` value defines the number of shards, while the `regex` value selects a subset of the shards.

![Workflow for sampling scrape targets](../sample.svg)

{{< admonition type="note" >}}
Choose your source label(s) for the `hashmod` action carefully. They must uniquely define each scrape target or `hashmod` won't be able to shard the targets uniformly.
{{< /admonition >}}

For example, consider an application deployed on Kubernetes with 100 pod replicas, all uniquely identified by the label `pod_hash`.
The following configuration is set to sample 15% of the pods:

```alloy
discovery.kubernetes "profile_pods" {
  role = "pod"
}

discovery.relabel "profile_pods" {
  targets = concat(discovery.kubernetes.profile_pods.targets)

  // Other rule blocks ...

  rule {
    action        = "hashmod"
    source_labels = ["pod_hash"]
    modulus       = 100
    target_label  = "__tmp_hashmod"
  }

  rule {
    action        = "keep"
    source_labels = ["__tmp_hashmod"]
    regex         = "^([0-9]|1[0-4])$"
  }

  // Other rule blocks ...
}
```

## Considerations

This strategy doesn't guarantee precise sampling.
Due to its reliance on an MD5 hash, there isn't a perfectly uniform distribution of scrape targets into shards.
Larger numbers of scrape targets yield increasingly accurate sampling.

Keep in mind, if the label hashed is deterministic, you see deterministic sharding and thereby deterministic sampling of scrape targets.
Similarly, if the label hashed is non-deterministic, you see scrape targets sampled in a non-deterministic fashion.
