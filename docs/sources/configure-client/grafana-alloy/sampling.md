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

This approach reduces the number of application instances Alloy profiles.
For Pyroscope v2 deployments, you can sample profiles after they reach the distributor instead. Refer to [Configure write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/configure-sampling/).

## Before you begin

Make sure you understand how to configure the collector to scrape targets and are familiar with the component configuration language.
Alloy configuration files use the Alloy [configuration syntax](https://grafana.com/docs/alloy/latest/concepts/configuration-syntax/).

## Configuration

The `hashmod` action and the `modulus` argument are used in conjunction to enable sampling behavior by sharding one or more labels.
To read further on these concepts, refer to [rule block documentation](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.relabel/#rule-block).
In short, `hashmod` performs an MD5 hash on the source labels and `modulus` performs a modulus operation on the output.

The sample size can be modified by changing the value of `modulus` in the `hashmod` action and the `regex` argument in the `keep` action.
The `modulus` value defines the number of shards, while the `regex` value selects a subset of the shards.

{{< mermaid >}}
flowchart LR
    A["Kubernetes pod discovery"] --> B["Hash pod UID modulo 100"]
    B --> C["Keep shards 0 through 14"]
    C --> D["Scrape the filtered targets"]
{{< /mermaid >}}

{{< admonition type="note" >}}
Choose your source label(s) for the `hashmod` action carefully. They must identify the unit you want to sample.
For pod-level sampling, use a label that is unique to each pod and shared by all of that pod's scrape targets.
{{< /admonition >}}

For example, consider an application deployed on Kubernetes with 100 pod replicas.
The following configuration uses the `__meta_kubernetes_pod_uid` label provided by [`discovery.kubernetes`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.kubernetes/#pod-role) to sample approximately 15% of the pods.
Apply your other relabeling rules to select the application's pods and scrape endpoints before these sampling rules.
Keep the pod UID label until the sampling rules have run.

```alloy
discovery.kubernetes "profile_pods" {
	role = "pod"
}

discovery.relabel "profile_pods" {
	targets = discovery.kubernetes.profile_pods.targets

	// Other rule blocks ...

	rule {
		action        = "hashmod"
		source_labels = ["__meta_kubernetes_pod_uid"]
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

Configure `pyroscope.scrape` to use `discovery.relabel.profile_pods.output` as its `targets` input, rather than the unfiltered `discovery.kubernetes.profile_pods.targets`.
For a complete scrape and write configuration, refer to [Set up Go profiling in pull mode](../go_pull/).

## Considerations

This strategy doesn't guarantee precise sampling.
Due to its reliance on an MD5 hash, there isn't a perfectly uniform distribution of scrape targets into shards.
Larger numbers of scrape targets yield increasingly accurate sampling.

Keep in mind, if the label hashed is deterministic, you see deterministic sharding and thereby deterministic sampling of scrape targets.
Similarly, if the label hashed is non-deterministic, you see scrape targets sampled in a non-deterministic fashion.

A pod's UID is stable for its lifetime, so all scrape targets from that pod are kept or dropped together.
When Kubernetes replaces a pod, the new pod has a different UID and may fall into a different sampling shard.
