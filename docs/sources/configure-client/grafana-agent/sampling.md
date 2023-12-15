---
title: "Sampling scrape targets"
menuTitle: "Sampling targets"
description: "Sampling scrape targets with the Grafana Agent"
weight: 30
---

# Sampling scrape targets

It's not uncommon for an application to have many instances deployed. While Pyroscope is designed specifically to handle large amounts of profiling data, you may have other considerations which demand that only a subset of the application's instances should be scraped. For example, the sheer volume of profiling data your application generates may make it unreasonable to profile every instance or you might be targeting cost-reduction. Whatever the case may be, it's possible to sample scrape targets through configuration of the Grafana Agent.

## Prerequisites

Before you begin, make sure you understand how to [configure the Grafana Agent]({{< relref "." >}}) to scrape targets and are familiar with the Grafana Agent [component configuration language](/docs/agent/latest/flow/config-language/components).

## Configuration

The `hashmod` action and the `modulus` argument are used in conjunction to enable sampling behavior. To read further on these concepts, see [rule block documentation](/docs/agent/latest/flow/reference/components/discovery.relabel#rule-block). In short, `hashmod` will perform an MD5 hash on the source labels and `modulus` will perform a modulus operation on the output.

Together, these can be leveraged to shard targets into shards. One or more shards can be selected to be the sampled targets to be scraped. Here is an example:

```river
discovery.relabel "sampled_targets" {
  targets = concat(<targets>)

  // Other rule blocks ...

  rule {
    action        = "hashmod"
    source_labels = [<label>]
    modulus       = 10
    target_label  = "__tmp_hashmod"
  }

  rule {
    action        = "keep"
    source_labels = ["__tmp_hashmod"]
    regex         = "0"
  }
}
```

The sample size can be modified by changing the value of `modulus` and the `regex` field of the `keep` action. For example, consider an application deployed on Kubernetes with 100 pod replicas, all uniquely identified by the label `pod_hash`. The following configuration is set to sample 15% of the pods:

```river
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

This strategy does not guarantee precise sampling. Due to its reliance on an MD5 hash, there is not a perfectly uniform distribution of scrape targets into shards. Larger numbers of scrape targets will yield increasingly accurate sampling.

Keep in mind, if the label being hashed is deterministic, you will see deterministic sharding and thereby deterministic sampling of scrape targets. Similarly, if the the label being hashed is non-deterministic, you will see scrape targets being sampled in a non-deterministic fashion.
