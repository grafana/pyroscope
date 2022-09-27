---
title: "Grafana Mimir runbooks"
menuTitle: "Runbooks"
description: "Grafana Mimir runbooks."
weight: 110
keywords:
  - Mimir runbooks
---

# Grafana Mimir runbooks

This document contains runbooks, or at least a checklist of what to look for, for alerts in the mimir-mixin and logs from Mimir. This document assumes that you are running a Mimir cluster:

1. Using this mixin config
2. Using GCS as object store (but similar procedures apply to other backends)

## Alerts

### MimirIngesterRestarts

First, check if the alert is for a single ingester or multiple. Even if the alert is only for one ingester, it's best to follow up by checking `kubectl get pods --namespace=<prod/staging/etc.>` every few minutes, or looking at the query `rate(kube_pod_container_status_restarts_total{container="ingester"}[30m]) > 0` just until you're sure there isn't a larger issue causing multiple restarts.

Next, check `kubectl get events`, with and without the addition of the `--namespace` flag, to look for node restarts or other related issues. Grep or something similar to filter the output can be useful here. The most common cause of this alert is a single cloud providers node restarting and causing the ingester on that node to be rescheduled somewhere else.

In events you're looking for things like:

```
57m Normal NodeControllerEviction Pod Marking for deletion Pod ingester-01 from Node cloud-provider-node-01
37m Normal SuccessfulDelete ReplicaSet (combined from similar events): Deleted pod: ingester-01
32m         Normal    NodeNotReady              Node   Node cloud-provider-node-01 status is now: NodeNotReady
28m         Normal    DeletingAllPods           Node   Node cloud-provider-node-01 event: Deleting all Pods from Node cloud-provider-node-01.
```

If nothing obvious from the above, check for increased load:

- If there is an increase in the number of active series and the memory provisioned is not enough, scale up the ingesters horizontally to have the same number of series as before per ingester.
- If we had an outage and once Mimir is back up, the incoming traffic increases. (or) The clients have their Prometheus remote-write lagging and starts to send samples at a higher rate (again, an increase in traffic but in terms of number of samples). Scale up the ingester horizontally in this case too.

### MimirIngesterReachingSeriesLimit

This alert fires when the `max_series` per ingester instance limit is enabled and the actual number of in-memory series in an ingester is reaching the limit. Once the limit is reached, writes to the ingester will fail (5xx) for new series, while appending samples to existing ones will continue to succeed.

In case of **emergency**:

- If the actual number of series is very close to or already hit the limit, then you can increase the limit via runtime config to gain some time
- Increasing the limit will increase the ingesters' memory utilization. Please monitor the ingesters' memory utilization via the `Mimir / Writes Resources` dashboard

How the limit is **configured**:

- The limit can be configured either on CLI (`-ingester.instance-limits.max-series`) or in the runtime config:
  ```
  ingester_limits:
    max_series: <int>
  ```
- The mixin configures the limit in the runtime config and can be fine-tuned via:
  ```
  _config+:: {
    ingester_instance_limits+:: {
      max_series: <int>
    }
  }
  ```
- When configured in the runtime config, changes are applied live without requiring an ingester restart
- The configured limit can be queried via `cortex_ingester_instance_limits{limit="max_series"}`

How to **fix** it:

1. **Temporarily increase the limit**<br />
   If the actual number of series is very close to or already hit the limit, or if you foresee the ingester will hit the limit before dropping the stale series as an effect of the scale up, you should also temporarily increase the limit.
2. **Check if shuffle-sharding shard size is correct**<br />

- When shuffle-sharding is enabled, we target up to 100K series / tenant / ingester assuming tenants on average use 50% of their max series limit.
- Run the following **instant query** to find tenants that might cause higher pressure on ingesters. The query excludes tenants which are already sharded across all ingesters:

  ```
  topk by (pod) (5, # top 5 tenants per ingester
      sum by (user, pod) ( # get in-memory series for each tenant on each pod
          cortex_ingester_memory_series_created_total{namespace="<namespace>"} - cortex_ingester_memory_series_removed_total{namespace="<namespace>"}
      )
      and on(user) # intersection with tenants that are exceeding 50% of their series limit (added acorss ingesters & accounting for replication)
      (
          sum by (user) ( # total in-memory series for the tenant across ingesters
              cortex_ingester_memory_series_created_total{namespace="<namespace>"} - cortex_ingester_memory_series_removed_total{namespace="<namespace>"}
          )
          > 200000 # show only big tenants - with more than 200k series across ingesters
          >
          (
              max by(user) (cortex_limits_overrides{namespace="<namespace>", limit_name="max_global_series_per_user"}) # global limit
              *
              scalar(max(cortex_distributor_replication_factor{namespace="<namespace>"})) # with replication
              *
              0.5 # 50%
          )
      )
      and on (pod) ( # intersection with top 3 ingesters by in-memory series
          topk(3,
              sum by (pod) (cortex_ingester_memory_series{namespace="<namespace>"})
          )
      )
      and on(user) ( # intersection with the tenants which don't have series on all ingesters
          count by (user) (cortex_ingester_memory_series_created_total{namespace="<namespace>"}) # count ingesters where each tenant has series
          !=
          scalar(count(count by (pod) (cortex_ingester_memory_series{namespace="<namespace>"}))) # count total ingesters: first `count` counts series by ingester (we ignore the counted number), second `count` counts rows in series per ingester, second count gives the number of ingesters
      )
  )
  ```

- Check the current shard size of each tenant in the output and, if they're not already sharded across all ingesters, you may consider to double their shard size
- Be warned that the when increasing the shard size for a tenant, the number of in-memory series will temporarily increase. Make sure to monitor:
  - The per-ingester number of series, to make sure that any are not close to reaching the limit. You might need to temporarily raise the ingester `max_series`.
  - The per-tenant number of series. Due to reshuffling, series will be counted multiple times (in the new and old ingesters), and therefore a tenant may risk having samples rejected because they hit the `per_user` series limit. You might need to temporarily raise the limit.
- The in-memory series in the ingesters will be effectively reduced at the TSDB head compaction happening at least 1h after you increased the shard size for the affected tenants

3. **Scale up ingesters**<br />
   Scaling up ingesters will lower the number of series per ingester. However, the effect of this change will take up to 4h, because after the scale up we need to wait until all stale series are dropped from memory as the effect of TSDB head compaction, which could take up to 4h (with the default config, TSDB keeps in-memory series up to 3h old and it gets compacted every 2h).

### MimirIngesterReachingTenantsLimit

This alert fires when the `max_tenants` per ingester instance limit is enabled and the actual number of tenants in an ingester is reaching the limit. Once the limit is reached, writes to the ingester will fail (5xx) for new tenants, while they will continue to succeed for previously existing ones.

In case of **emergency**:

- If the actual number of tenants is very close to or already hit the limit, then you can increase the limit via runtime config to gain some time
- Increasing the limit will increase the ingesters' memory utilization. Please monitor the ingesters' memory utilization via the `Mimir / Writes Resources` dashboard

How the limit is **configured**:

- The limit can be configured either on CLI (`-ingester.instance-limits.max-tenants`) or in the runtime config:
  ```
  ingester_limits:
    max_tenants: <int>
  ```
- The mixin configures the limit in the runtime config and can be fine-tuned via:
  ```
  _config+:: {
    ingester_instance_limits+:: {
      max_tenants: <int>
    }
  }
  ```
- When configured in the runtime config, changes are applied live without requiring an ingester restart
- The configured limit can be queried via `cortex_ingester_instance_limits{limit="max_tenants"}`

How to **fix** it:

1. Ensure shuffle-sharding is enabled in the Mimir cluster
1. Assuming shuffle-sharding is enabled, scaling up ingesters will lower the number of tenants per ingester. However, the effect of this change will be visible only after `-blocks-storage.tsdb.close-idle-tsdb-timeout` period so you may have to temporarily increase the limit

### MimirDistributorReachingInflightPushRequestLimit

This alert fires when the `cortex_distributor_inflight_push_requests` per distributor instance limit is enabled and the actual number of in-flight push requests is approaching the set limit. Once the limit is reached, push requests to the distributor will fail (5xx) for new requests, while existing in-flight push requests will continue to succeed.

In case of **emergency**:

- If the actual number of in-flight push requests is very close to or already at the set limit, then you can increase the limit via CLI flag or config to gain some time
- Increasing the limit will increase the number of in-flight push requests which will increase distributors' memory utilization. Please monitor the distributors' memory utilization via the `Mimir / Writes Resources` dashboard

How the limit is **configured**:

- The limit can be configured either by the CLI flag (`-distributor.instance-limits.max-inflight-push-requests`) or in the config:
  ```
  distributor:
    instance_limits:
      max_inflight_push_requests: <int>
  ```
- These changes are applied with a distributor restart.
- The configured limit can be queried via `cortex_distributor_instance_limits{limit="max_inflight_push_requests"})`

How to **fix** it:

1. **Temporarily increase the limit**<br />
   If the actual number of in-flight push requests is very close to or already hit the limit.
2. **Scale up distributors**<br />
   Scaling up distributors will lower the number of in-flight push requests per distributor.

### MimirRequestLatency

This alert fires when a specific Mimir route is experiencing an high latency.

The alert message includes both the Mimir service and route experiencing the high latency. Establish if the alert is about the read or write path based on that (see [Mimir routes by path](#mimir-routes-by-path)).

#### Write Latency

How to **investigate**:

- Check the `Mimir / Writes` dashboard
  - Looking at the dashboard you should see in which Mimir service the high latency originates
  - The panels in the dashboard are vertically sorted by the network path (eg. gateway -> distributor -> ingester)
- Deduce where in the stack the latency is being introduced
  - **`gateway`**
    - Latency may be caused by the time taken for the gateway to receive the entire request from the client. There are a multitude of reasons this can occur, so communication with the user may be necessary. For example:
      - Network issues such as packet loss between the client and gateway.
      - Poor performance of intermediate network hops such as load balancers or HTTP proxies.
      - Client process having insufficient CPU resources.
    - The gateway may need to be scaled up. Use the `Mimir / Scaling` dashboard to check for CPU usage vs requests.
    - There could be a problem with authentication (eg. slow to run auth layer)
  - **`distributor`**
    - Typically, distributor p99 latency is in the range 50-100ms. If the distributor latency is higher than this, you may need to scale up the distributors.
  - **`ingester`**
    - Typically, ingester p99 latency is in the range 5-50ms. If the ingester latency is higher than this, you should investigate the root cause before scaling up ingesters.
    - Check out the following alerts and fix them if firing:
      - `MimirProvisioningTooManyActiveSeries`
      - `MimirProvisioningTooManyWrites`

#### Read Latency

Query performance is a known issue. A query may be slow because of high cardinality, large time range and/or because not leveraging on cache (eg. querying series data not cached yet). When investigating this alert, you should check if it's caused by few slow queries or there's an operational / config issue to be fixed.

How to **investigate**:

- Check the `Mimir / Reads` dashboard
  - Looking at the dashboard you should see in which Mimir service the high latency originates
  - The panels in the dashboard are vertically sorted by the network path (eg. gateway -> query-frontend -> query->scheduler -> querier -> store-gateway)
- Check the `Mimir / Slow Queries` dashboard to find out if it's caused by few slow queries
- Deduce where in the stack the latency is being introduced
  - **`gateway`**
    - The gateway may need to be scaled up. Use the `Mimir / Scaling` dashboard to check for CPU usage vs requests.
    - There could be a problem with authentication (eg. slow to run auth layer)
  - **`query-frontend`**
    - The query-frontend may need to be scaled up. If the Mimir cluster is running with the query-scheduler, the query-frontend can be scaled up with no side effects, otherwise the maximum number of query-frontend replicas should be the configured `-querier.max-concurrent`.
  - **`querier`**
    - Look at slow queries traces to find out where it's slow.
    - Typically, slowness either comes from running PromQL engine (`innerEval`) or fetching chunks from ingesters and/or store-gateways.
    - If slowness comes from running PromQL engine, typically there's not much we can do. Scaling up queriers may help only if querier nodes are overloaded.
    - If slowness comes from fetching chunks from ingesters and/or store-gateways you should investigate deeper on the root cause. Common causes:
      - High CPU utilization in ingesters
        - Scale up ingesters
      - Low cache hit ratio in the store-gateways
        - Check `Memcached Overview` dashboard
        - If memcached eviction rate is high, then you should scale up memcached replicas. Check the recommendations by `Mimir / Scaling` dashboard and make reasonable adjustments as necessary.
        - If memcached eviction rate is zero or very low, then it may be caused by "first time" queries
      - Cache query timeouts
        - Check store-gateway logs and look for warnings about timed out Memcached queries (example query: `{namespace="example-mimir-cluster", name=~"store-gateway.*"} |= "level=warn" |= "memcached" |= "timeout"`)
        - If there are indeed a lot of timed out Memcached queries, consider whether the store-gateway Memcached timeout setting (`-blocks-storage.bucket-store.chunks-cache.memcached.timeout`) is sufficient
    - By consulting the "Queue length" panel of the `Mimir / Queries` dashboard, determine if queries are waiting in queue due to busy queriers (an indication of this would be queue length > 0 for some time)
      - If queries are waiting in queue
        - Consider scaling up number of queriers if they're not auto-scaled; if auto-scaled, check auto-scaling parameters
      - If queries are not waiting in queue
        - Consider [enabling query sharding]({{< relref "../architecture/query-sharding/index.md#how-to-enable-query-sharding" >}}) if not already enabled, to increase query parallelism
        - If query sharding already enabled, consider increasing total number of query shards (`query_sharding_total_shards`) for tenants submitting slow queries, so their queries can be further parallelized

#### Alertmanager

How to **investigate**:

- Check the `Mimir / Alertmanager` dashboard
  - Looking at the dashboard you should see which part of the stack is affected
- Deduce where in the stack the latency is being introduced
  - **Configuration API (gateway) + Alertmanager UI**
    - Latency may be caused by the time taken for the gateway to receive the entire request from the client. There are a multitude of reasons this can occur, so communication with the user may be necessary. For example:
      - Network issues such as packet loss between the client and gateway.
      - Poor performance of intermediate network hops such as load balancers or HTTP proxies.
      - Client process having insufficient CPU resources.
    - The gateway may need to be scaled up. Use the `Mimir / Scaling` dashboard to check for CPU usage vs requests.
    - There could be a problem with authentication (eg. slow to run auth layer)
  - **Alertmanager distributor**
    - Typically, Alertmanager distributor p99 latency is in the range 50-100ms. If the distributor latency is higher than this, you may need to scale up the number of alertmanager replicas.

### MimirRequestErrors

This alert fires when the rate of 5xx errors of a specific route is > 1% for some time.

This alert typically acts as a last resort to detect issues / outages. SLO alerts are expected to trigger earlier: if an **SLO alert** has triggered as well for the same read/write path, then you can ignore this alert and focus on the SLO one (but the investigation procedure is typically the same).

How to **investigate**:

- Check for which route the alert fired (see [Mimir routes by path](#mimir-routes-by-path))
  - Write path: open the `Mimir / Writes` dashboard
  - Read path: open the `Mimir / Reads` dashboard
- Looking at the dashboard you should see in which Mimir service the error originates
  - The panels in the dashboard are vertically sorted by the network path (eg. on the write path: gateway -> distributor -> ingester)
- If the failing service is going OOM (`OOMKilled`): scale up or increase the memory
- If the failing service is crashing / panicking: look for the stack trace in the logs and investigate from there
  - If crashing service is query-frontend, querier or store-gateway, and you have "activity tracker" feature enabled, look for `found unfinished activities from previous run` message and subsequent `activity` messages in the log file to see which queries caused the crash.
- When using Memberlist as KV store for hash rings, ensure that Memberlist is working correctly. See instructions for [`MimirGossipMembersMismatch`](#MimirGossipMembersMismatch) alert.

#### Alertmanager

How to **investigate**:

- Looking at `Mimir / Alertmanager` dashboard you should see in which part of the stack the error originates
- If some replicas are going OOM (`OOMKilled`): scale up or increase the memory
- If the failing service is crashing / panicking: look for the stack trace in the logs and investigate from there

### MimirIngesterUnhealthy

This alert goes off when one or more ingesters are marked as unhealthy. Check the ring web page to see which ones are marked as unhealthy. You could then check the logs to see if there are any related to involved ingesters, f.ex: `kubectl logs -f ingester-01 --namespace=prod`. A simple way to resolve this may be to click the "Forget" button on the ring page, especially if the pod doesn't exist anymore. It might not exist anymore because it was on a node that got shut down, so you could check to see if there are any logs related to the node that pod is/was on, f.ex.: `kubectl get events --namespace=prod | grep cloud-provider-node`.

### MimirMemoryMapAreasTooHigh

This alert fires when a Mimir process has a number of memory map areas close to the limit. The limit is a per-process limit imposed by the kernel and this issue is typically caused by a large number of mmap-ed failes.

How to **fix** it:

- Increase the limit on your system: `sysctl -w vm.max_map_count=<NEW LIMIT>`
- If it's caused by a store-gateway, consider enabling `-blocks-storage.bucket-store.index-header-lazy-loading-enabled=true` to lazy mmap index-headers at query time

More information:

- [Kernel doc](https://www.kernel.org/doc/Documentation/sysctl/vm.txt)
- [Side effects when increasing `vm.max_map_count`](https://www.suse.com/support/kb/doc/?id=000016692)

### MimirRulerFailedRingCheck

This alert occurs when a ruler is unable to validate whether or not it should claim ownership over the evaluation of a rule group. The most likely cause is that one of the rule ring entries is unhealthy. If this is the case proceed to the ring admin http page and forget the unhealth ruler. The other possible cause would be an error returned the ring client. If this is the case look into debugging the ring based on the in-use backend implementation.

When using Memberlist as KV store for hash rings, ensure that Memberlist is working correctly. See instructions for [`MimirGossipMembersMismatch`](#MimirGossipMembersMismatch) alert.

### MimirRulerTooManyFailedPushes

This alert fires when rulers cannot push new samples (result of rule evaluation) to ingesters.

In general, pushing samples can fail due to problems with Mimir operations (eg. too many ingesters have crashed, and ruler cannot write samples to them), or due to problems with resulting data (eg. user hitting limit for number of series, out of order samples, etc.).
This alert fires only for first kind of problems, and not for problems caused by limits or invalid rules.

How to **fix** it:

- Investigate the ruler logs to find out the reason why ruler cannot write samples. Note that ruler logs all push errors, including "user errors", but those are not causing the alert to fire. Focus on problems with ingesters.
- When using Memberlist as KV store for hash rings, ensure that Memberlist is working correctly. See instructions for [`MimirGossipMembersMismatch`](#MimirGossipMembersMismatch) alert.

### MimirRulerTooManyFailedQueries

This alert fires when rulers fail to evaluate rule queries.

Each rule evaluation may fail due to many reasons, eg. due to invalid PromQL expression, or query hits limits on number of chunks. These are "user errors", and this alert ignores them.

There is a category of errors that is more important: errors due to failure to read data from store-gateways or ingesters. These errors would result in 500 when run from querier. This alert fires if there is too many of such failures.

How to **fix** it:

- Investigate the ruler logs to find out the reason why ruler cannot evaluate queries. Note that ruler logs rule evaluation errors even for "user errors", but those are not causing the alert to fire. Focus on problems with ingesters or store-gateways.
- In case remote operational mode is enabled the problem could be at any of the ruler query path components (ruler-query-frontend, ruler-query-scheduler and ruler-querier). Check the `Mimir / Remote ruler reads` and `Mimir / Remote ruler reads resources` dashboards to find out in which Mimir service the error is being originated.
- When using Memberlist as KV store for hash rings, ensure that Memberlist is working correctly. See instructions for [`MimirGossipMembersMismatch`](#MimirGossipMembersMismatch) alert.

### MimirRulerMissedEvaluations

This alert fires when there is a rule group that is taking longer to evaluate than its evaluation interval.

How it **works**:

- The Mimir ruler will evaluate a rule group according to the evaluation interval on the rule group.
- If an evaluation is not finished by the time the next evaluation should happen, the next evaluation is missed.

How to **fix** it:

- Increase the evaluation interval of the rule group. You can use the rate of missed evaluation to estimate how long the rule group evaluation actually takes.
- Try splitting up the rule group into multiple rule groups. Rule groups are evaluated in parallel, so the same rules may still fit in the same resolution.

### MimirIngesterHasNotShippedBlocks

This alert fires when a Mimir ingester is not uploading any block to the long-term storage. An ingester is expected to upload a block to the storage every block range period (defaults to 2h) and if a longer time elapse since the last successful upload it means something is not working correctly.

How to **investigate**:

- Ensure the ingester is receiving write-path traffic (samples to ingest)
- Look for any upload error in the ingester logs (ie. networking or authentication issues)

_If the alert `MimirIngesterTSDBHeadCompactionFailed` fired as well, then give priority to it because that could be the cause._

#### Ingester hit the disk capacity

If the ingester hit the disk capacity, any attempt to append samples will fail. You should:

1. Increase the disk size and restart the ingester. If the ingester is running in Kubernetes with a Persistent Volume, please refers to [Resizing Persistent Volumes using Kubernetes](#resizing-persistent-volumes-using-kubernetes).
2. Investigate why the disk capacity has been hit

- Was the disk just too small?
- Was there an issue compacting TSDB head and the WAL is increasing indefinitely?

### MimirIngesterHasNotShippedBlocksSinceStart

Same as [`MimirIngesterHasNotShippedBlocks`](#MimirIngesterHasNotShippedBlocks).

### MimirIngesterHasUnshippedBlocks

This alert fires when a Mimir ingester has compacted some blocks but such blocks haven't been successfully uploaded to the storage yet.

How to **investigate**:

- Look for details in the ingester logs

### MimirIngesterTSDBHeadCompactionFailed

This alert fires when a Mimir ingester is failing to compact the TSDB head into a block.

A TSDB instance is opened for each tenant writing at least 1 series to the ingester and its head contains the in-memory series not flushed to a block yet. Once the TSDB head is compactable, the ingester will try to compact it every 1 minute. If the TSDB head compaction repeatedly fails, it means it's failing to compact a block from the in-memory series for at least 1 tenant, and it's a critical condition that should be immediately investigated.

The cause triggering this alert could **lead to**:

- Ingesters run out of memory
- Ingesters run out of disk space
- Queries return partial results after `-querier.query-ingesters-within` time since the beginning of the incident

How to **investigate**:

- Look for details in the ingester logs

### MimirIngesterTSDBHeadTruncationFailed

This alert fires when a Mimir ingester fails to truncate the TSDB head.

The TSDB head is the in-memory store used to keep series and samples not compacted into a block yet. If head truncation fails for a long time, the ingester disk might get full as it won't continue to the WAL truncation stage and the subsequent ingester restart may take a long time or even go into an OOMKilled crash loop because of the huge WAL to replay. For this reason, it's important to investigate and address the issue as soon as it happen.

How to **investigate**:

- Look for details in the ingester logs

### MimirIngesterTSDBCheckpointCreationFailed

This alert fires when a Mimir ingester fails to create a TSDB checkpoint.

How to **investigate**:

- Look for details in the ingester logs
- If the checkpoint fails because of a `corruption in segment`, you can restart the ingester because at next startup TSDB will try to "repair" it. After restart, if the issue is repaired and the ingester is running, you should also get paged by `MimirIngesterTSDBWALCorrupted` to signal you the WAL was corrupted and manual investigation is required.

### MimirIngesterTSDBCheckpointDeletionFailed

This alert fires when a Mimir ingester fails to delete a TSDB checkpoint.

Generally, this is not an urgent issue, but manual investigation is required to find the root cause of the issue and fix it.

How to **investigate**:

- Look for details in the ingester logs

### MimirIngesterTSDBWALTruncationFailed

This alert fires when a Mimir ingester fails to truncate the TSDB WAL.

How to **investigate**:

- Look for details in the ingester logs

### MimirIngesterTSDBWALCorrupted

This alert fires when a Mimir ingester finds a corrupted TSDB WAL (stored on disk) while replaying it at ingester startup or when creation of a checkpoint comes across a WAL corruption.

If this alert fires during an **ingester startup**, the WAL should have been auto-repaired, but manual investigation is required. The WAL repair mechanism causes data loss because all WAL records after the corrupted segment are discarded, and so their samples are lost while replaying the WAL. If this happens only on 1 ingester then Mimir doesn't suffer any data loss because of the replication factor, but if it happens on multiple ingesters some data loss is possible.

To investigate how the ingester dealt with the WAL corruption, it's recommended you search the logs, e.g. with the following Grafana Loki query:

```
{cluster="<cluster>",namespace="<namespace>", pod="<pod>"} |= "corrupt"
```

The aforementioned query should typically produce entries starting with the ingester discovering the WAL corruption ("Encountered WAL read error, attempting repair"), and should hopefully show that the ingester repaired the WAL.

WAL corruption can occur after pods are rescheduled following a fault with the underlying node, causing the node to be marked `NotReady` (e.g. an unplanned power outage, storage and/or network fault). Check for recent events related to the ingester pod in question:

```
kubectl get events --field-selector involvedObject.name=ingester-X
```

If this alert fires during a **checkpoint creation**, you should have also been paged with `MimirIngesterTSDBCheckpointCreationFailed`, and you can follow the steps under that alert.

### MimirIngesterTSDBWALWritesFailed

This alert fires when a Mimir ingester is failing to log records to the TSDB WAL on disk.

How to **investigate**:

- Look for details in the ingester logs

### MimirQuerierHasNotScanTheBucket

This alert fires when a Mimir querier is not successfully scanning blocks in the storage (bucket). A querier is expected to periodically iterate the bucket to find new and deleted blocks (defaults to every 5m) and if it's not successfully synching the bucket since a long time, it may end up querying only a subset of blocks, thus leading to potentially partial results.

How to **investigate**:

- Look for any scan error in the querier logs (ie. networking or rate limiting issues)

### MimirQuerierHighRefetchRate

This alert fires when there's an high number of queries for which series have been refetched from a different store-gateway because of missing blocks. This could happen for a short time whenever a store-gateway ring resharding occurs (e.g. during/after an outage or while rolling out store-gateway) but store-gateways should reconcile in a short time. This alert fires if the issue persist for an unexpected long time and thus it should be investigated.

How to **investigate**:

- Ensure there are no errors related to blocks scan or sync in the queriers and store-gateways
- Check store-gateway logs to see if all store-gateway have successfully completed a blocks sync

### MimirStoreGatewayHasNotSyncTheBucket

This alert fires when a Mimir store-gateway is not successfully scanning blocks in the storage (bucket). A store-gateway is expected to periodically iterate the bucket to find new and deleted blocks (defaults to every 5m) and if it's not successfully synching the bucket for a long time, it may end up querying only a subset of blocks, thus leading to potentially partial results.

How to **investigate**:

- Look for any scan error in the store-gateway logs (ie. networking or rate limiting issues)

### MimirStoreGatewayNoSyncedTenants

This alert fires when a store-gateway doesn't own any tenant. Effectively it is sitting idle because no blocks are sharded to it.

How it **works**:

- Store-gateways join a hash ring to shard tenants and blocks across all store-gateway replicas.
- A tenant can be sharded across multiple store-gateways. How many exactly is determined by `-store-gateway.tenant-shard-size` or the `store_gateway_tenant_shard_size` limit.
- When the tenant shard size is less than the replicas of store-gateways, some store-gateways may not get any tenants' blocks sharded to them.
- This is more likely to happen in Mimir clusters with fewer number of tenants.

How to **fix** it:

There are three options:

- Reduce the replicas of store-gateways so that they match the highest number of shards per tenant or
- Increase the shard size of one or more tenants to match the number of replicas or
- Set the shard size of one or more tenant to `0`; this will shard this tenant's blocks across all store-gateways.

### MimirCompactorHasNotSuccessfullyCleanedUpBlocks

This alert fires when a Mimir compactor is not successfully deleting blocks marked for deletion for a long time.

How to **investigate**:

- Ensure the compactor is not crashing during compaction (ie. `OOMKilled`)
- Look for any error in the compactor logs (ie. bucket Delete API errors)

### MimirCompactorHasNotSuccessfullyCleanedUpBlocksSinceStart

Same as [`MimirCompactorHasNotSuccessfullyCleanedUpBlocks`](#MimirCompactorHasNotSuccessfullyCleanedUpBlocks).

### MimirCompactorHasNotUploadedBlocks

This alert fires when a Mimir compactor is not uploading any compacted blocks to the storage since a long time.

How to **investigate**:

- If the alert `MimirCompactorHasNotSuccessfullyRunCompaction` has fired as well, then investigate that issue first
- If the alert `MimirIngesterHasNotShippedBlocks` or `MimirIngesterHasNotShippedBlocksSinceStart` have fired as well, then investigate that issue first
- Ensure ingesters are successfully shipping blocks to the storage
- Look for any error in the compactor logs

### MimirCompactorHasNotSuccessfullyRunCompaction

This alert fires if the compactor is not able to successfully compact all discovered compactable blocks (across all tenants).

When this alert fires, the compactor may still have successfully compacted some blocks but, for some reason, other blocks compaction is consistently failing. A common case is when the compactor is trying to compact a corrupted block for a single tenant: in this case the compaction of blocks for other tenants is still working, but compaction for the affected tenant is blocked by the corrupted block.

How to **investigate**:

- Look for any error in the compactor logs
  - Corruption: [`not healthy index found`](#compactor-is-failing-because-of-not-healthy-index-found)
  - Invalid result block:
    - **How to detect**: Search compactor logs for `invalid result block`.
    - **What it means**: The compactor successfully validated the source blocks. But the validation of the result block after the compaction did not succeed. The result block was not uploaded and the compaction job will be retried.
    - Out-of-order chunks
      - **How to detect**: Search compactor logs for `invalid result block` and `out-of-order chunks`.
      - This is caused by a bug in the ingester - see [mimir#1537](https://github.com/grafana/mimir/issues/1537). Ingesters upload blocks where the MinT and MaxT of some chunks don't match the first and last samples in the chunk. When the faulty chunks' MinT and MaxT overlap with other chunks, the compactor merges the chunks. Because one chunk's MinT and MaxT are incorrect the merge may be performed incorrectly, leading to OoO samples.
      - **How to mitigate**: Mark the faulty blocks to avoid compacting them in the future:
        - Find all affected compaction groups in the compactor logs. You will find them as `invalid result block /data/compact/<compaction_group>/<result_block>`.
        - For each failed compaction job
          - Pick one result block (doesn't matter which)
          - Find source blocks for the compaction job: search for `msg="compact blocks"` and a mention of the result block ID.
          - Mark the source blocks for no compaction (in this example the object storage backend is GCS):
            ```
            ./tools/markblocks/markblocks -backend gcs -gcs.bucket-name <bucket> -mark no-compact -tenant <tenant-id> -details "Leading to out-of-order chunks when compacting with other blocks" <block-1> <block-2>...
            ```

### MimirCompactorSkippedBlocksWithOutOfOrderChunks

This alert fires when compactor tries to compact a block, but finds that given block has out-of-order chunks. This indicates a bug in Prometheus TSDB library and should be investigated.

#### Compactor is failing because of `not healthy index found`

The compactor may fail to compact blocks due a corrupted block index found in one of the source blocks:

```
level=error ts=2020-07-12T17:35:05.516823471Z caller=compactor.go:339 component=compactor msg="failed to compact user blocks" user=REDACTED-TENANT err="compaction: group 0@6672437747845546250: block with not healthy index found /data/compact/0@6672437747845546250/REDACTED-BLOCK; Compaction level 1; Labels: map[__org_id__:REDACTED]: 1/1183085 series have an average of 1.000 out-of-order chunks: 0.000 of these are exact duplicates (in terms of data and time range)"
```

When this happen you should:

1. Rename the block prefixing it with `corrupted-` so that it will be skipped by the compactor and queriers. Keep in mind that doing so the block will become invisible to the queriers too, so its series/samples will not be queried. If the corruption affects only 1 block whose compaction `level` is 1 (the information is stored inside its `meta.json`) then Mimir guarantees no data loss because all the data is replicated across other blocks. In all other cases, there may be some data loss once you rename the block and stop querying it.
2. Ensure the compactor has recovered
3. Investigate offline the root cause (eg. download the corrupted block and debug it locally)

To rename a block stored on GCS you can use the `gsutil` CLI command:

```
gsutil mv gs://BUCKET/TENANT/BLOCK gs://BUCKET/TENANT/corrupted-BLOCK
```

Where:

- `BUCKET` is the gcs bucket name the compactor is using. The cluster's bucket name is specified as the `blocks_storage_bucket_name` in the cluster configuration
- `TENANT` is the tenant id reported in the example error message above as `REDACTED-TENANT`
- `BLOCK` is the last part of the file path reported as `REDACTED-BLOCK` in the example error message above

### MimirBucketIndexNotUpdated

This alert fires when the bucket index, for a given tenant, is not updated since a long time. The bucket index is expected to be periodically updated by the compactor and is used by queriers and store-gateways to get an almost-updated view over the bucket store.

How to **investigate**:

- Ensure the compactor is successfully running
- Look for any error in the compactor logs

### MimirTenantHasPartialBlocks

This alert fires when Mimir finds partial blocks for a given tenant. A partial block is a block missing the `meta.json` and this may usually happen in two circumstances:

1. A block upload has been interrupted and not cleaned up or retried
2. A block deletion has been interrupted and `deletion-mark.json` has been deleted before `meta.json`

How to **investigate**:

1. Look for partial blocks in the logs. Example Loki query: `{cluster="<cluster>",namespace="<namespace>",container="compactor"} |= "skipped partial block"`
1. Pick a block and note its ID (`block` field in log entry) and tenant ID (`org_id` in log entry)
1. Find the bucket used by the Mimir cluster, such as checking the configured `blocks_storage_bucket_name` if you are using Jsonnet.
1. Find out which Mimir component operated on the block last (e.g. uploaded by ingester/compactor, or deleted by compactor)
   1. Determine when the partial block was uploaded: `gsutil ls -l gs://${BUCKET}/${TENANT_ID}/${BLOCK_ID}`. Alternatively you can use `ulidtime` command from Mimir tools directory `ulidtime ${BLOCK_ID}` to find block creation time.
   1. Search in the logs around that time to find the log entry from when the compactor created the block ("compacted blocks" for log message)
   1. From the compactor log entry you found, pick the job ID from the `groupKey` field, f.ex. `0@9748515562602778029-merge--1645711200000-1645718400000`
   1. Then search the logs for the job ID and look for an entry with the message "compaction job failed", this will show that the compactor failed uploading the block
   1. If you found a failed compaction job, as outlined in the previous step, try searching for a corresponding log message (for the same job ID) "compaction job succeeded". This will mean that the compaction job was retried successfully. Note: this should produce a different block ID from the failed upload.
1. Investigate if it was a partial upload or partial delete
   1. If it was a partial delete or an upload failed by a compactor you can safely mark the block for deletion, and compactor will delete the block. You can use `markblocks` command from Mimir tools directory: `markblocks -mark deletion -allow-partial -tenant <tenant> <blockID>` with correct backend (eg. GCS: `-backend gcs -gcs.bucket-name <bucket-name>`) configuration.
   1. If it was a failed upload by an ingester, but not later retried (ingesters are expected to retry uploads until succeed), further investigate
1. Prevent the issue from reoccurring by enabling automatic partial block cleanup. This can be enabled with the `-compactor.partial-block-deletion-delay` flag. It takes a duration as an argument. If a partial block persists past the specified duration, the compactor will automatically delete it. One can monitor automatic cleanup of partial blocks via the `cortex_compactor_blocks_marked_for_deletion_total{reason="partial"}` counter.

### MimirQueriesIncorrect

_TODO: this runbook has not been written yet._

### MimirInconsistentRuntimeConfig

This alert fires if multiple replicas of the same Mimir service are using a different runtime config for a longer period of time.

The Mimir runtime config is a config file which gets live reloaded by Mimir at runtime. In order for Mimir to work properly, the loaded config is expected to be the exact same across multiple replicas of the same Mimir service (eg. distributors, ingesters, ...). When the config changes, there may be short periods of time during which some replicas have loaded the new config and others are still running on the previous one, but it shouldn't last for more than few minutes.

How to **investigate**:

- Check how many different config file versions (hashes) are reported
  ```
  count by (sha256) (cortex_runtime_config_hash{namespace="<namespace>"})
  ```
- Check which replicas are running a different version
  ```
  cortex_runtime_config_hash{namespace="<namespace>",sha256="<unexpected>"}
  ```
- Check if the runtime config has been updated on the affected replicas' filesystem. Check `-runtime-config.file` command line argument to find the location of the file.
- Check the affected replicas logs and look for any error loading the runtime config

### MimirBadRuntimeConfig

This alert fires if Mimir is unable to reload the runtime config.

This typically means an invalid runtime config was deployed. Mimir keeps running with the previous (valid) version of the runtime config; running Mimir replicas and the system availability shouldn't be affected, but new replicas won't be able to startup until the runtime config is fixed.

How to **investigate**:

- Check the latest runtime config update (it's likely to be broken)
- Check Mimir logs to get more details about what's wrong with the config

### MimirFrontendQueriesStuck

This alert fires if Mimir is running without query-scheduler and queries are piling up in the query-frontend queue.

The procedure to investigate it is the same as the one for [`MimirSchedulerQueriesStuck`](#MimirSchedulerQueriesStuck): please see the other runbook for more details.

### MimirSchedulerQueriesStuck

This alert fires if queries are piling up in the query-scheduler.

How it **works**:

- A query-frontend API endpoint is called to execute a query
- The query-frontend enqueues the request to the query-scheduler
- The query-scheduler is responsible for dispatching enqueued queries to idle querier workers
- The querier runs the query, sends the response back directly to the query-frontend and notifies the query-scheduler that it can process another query

How to **investigate**:

- Are queriers in a crash loop (eg. OOMKilled)?
  - `OOMKilled`: temporarily increase queriers memory request/limit
  - `panic`: look for the stack trace in the logs and investigate from there
  - if queriers run with activity tracker enabled, they may log `unfinished activities` message on startup with queries that possibly caused the crash.
- Is QPS increased?
  - Scale up queriers to satisfy the increased workload
- Is query latency increased?
  - An increased latency reduces the number of queries we can run / sec: once all workers are busy, new queries will pile up in the queue
  - Temporarily scale up queriers to try to stop the bleed
  - Check if a specific tenant is running heavy queries
    - Run `sum by (user) (cortex_query_scheduler_queue_length{namespace="<namespace>"}) > 0` to find tenants with enqueued queries
    - If remote ruler evaluation is enabled, make sure you understand which one of the read paths (user or ruler queries?) is being affected - check the alert message.
    - Check the `Mimir / Slow Queries` dashboard to find slow queries
  - On multi-tenant Mimir cluster with **shuffle-sharing for queriers disabled**, you may consider to enable it for that specific tenant to reduce its blast radius. To enable queriers shuffle-sharding for a single tenant you need to set the `max_queriers_per_tenant` limit override for the specific tenant (the value should be set to the number of queriers assigned to the tenant).
  - On multi-tenant Mimir cluster with **shuffle-sharding for queriers enabled**, you may consider to temporarily increase the shard size for affected tenants: be aware that this could affect other tenants too, reducing resources available to run other tenant queries. Alternatively, you may choose to do nothing and let Mimir return errors for that given user once the per-tenant queue is full.
  - On multi-tenant Mimir clusters with **query-sharding enabled** and **more than a few tenants** being affected: The workload exceeds the available downstream capacity. Scaling of queriers and potentially store-gateways should be considered.
  - On multi-tenant Mimir clusters with **query-sharding enabled** and **only a single tenant** being affected:
    - Verify if the particular queries are hitting edge cases, where query-sharding is not benefical, by getting traces from the `Mimir / Slow Queries` dashboard and then look where time is spent. If time is spent in the query-frontend running PromQL engine, then it means query-sharding is not beneficial for this tenant. Consider disabling query-sharding or reduce the shard count using the `query_sharding_total_shards` override.
    - Otherwise and only if the queries by the tenant are within reason representing normal usage, consider scaling of queriers and potentially store-gateways.

### MimirMemcachedRequestErrors

This alert fires if Mimir memcached client is experiencing an high error rate for a specific cache and operation.

How to **investigate**:

- The alert reports which cache is experiencing issue
  - `metadata-cache`: object store metadata cache
  - `index-cache`: TSDB index cache
  - `chunks-cache`: TSDB chunks cache
- Check which specific error is occurring
  - Run the following query to find out the reason (replace `<namespace>` with the actual Mimir cluster namespace)
    ```
    sum by(name, operation, reason) (rate(thanos_memcached_operation_failures_total{namespace="<namespace>"}[1m])) > 0
    ```
- Based on the **`reason`**:
  - `timeout`
    - Scale up the memcached replicas
  - `server-error`
    - Check both Mimir and memcached logs to find more details
  - `network-error`
    - Check Mimir logs to find more details
  - `malformed-key`
    - The key is too long or contains invalid characters
    - Check Mimir logs to find the offending key
    - Fixing this will require changes to the application code
  - `other`
    - Check both Mimir and memcached logs to find more details

### MimirProvisioningTooManyActiveSeries

This alert fires if the average number of in-memory series per ingester is above our target (1.5M).

How to **fix** it:

- Scale up ingesters
  - To find out the Mimir clusters where ingesters should be scaled up and how many minimum replicas are expected:
    ```
    ceil(sum by(cluster, namespace) (cortex_ingester_memory_series) / 1.5e6) >
    count by(cluster, namespace) (cortex_ingester_memory_series)
    ```
- After the scale up, the in-memory series are expected to be reduced at the next TSDB head compaction (occurring every 2h)

### MimirProvisioningTooManyWrites

This alert fires if the average number of samples ingested / sec in ingesters is above our target.

How to **fix** it:

- Scale up ingesters
  - To compute the desired number of ingesters to satisfy the average samples rate you can run the following query, replacing `<namespace>` with the namespace to analyse and `<target>` with the target number of samples/sec per ingester (check out the alert threshold to see the current target):
    ```
    sum(rate(cortex_ingester_ingested_samples_total{namespace="<namespace>"}[$__rate_interval])) / (<target> * 0.9)
    ```

### MimirAllocatingTooMuchMemory

This alert fires when ingester memory utilization is getting too close to the limit.

How it **works**:

- Mimir ingesters are stateful services
- Having 2+ ingesters `OOMKilled` might cause a cluster outage
- Ingester memory baseline usage is primarily influenced by memory allocated by the process (mostly Go heap) and mmap-ed files (used by TSDB)
- Ingester memory short spikes are primarily influenced by queries and TSDB head compaction into new blocks (occurring every 2h)
- A pod gets `OOMKilled` once its working set memory reaches the configured limit, so it's important to prevent ingesters' memory utilization (working set memory) from getting close to the limit (we need to keep at least 30% room for spikes due to queries)

How to **fix** it:

- Check if the issue occurs only for few ingesters. If so:
  - Restart affected ingesters 1 by 1 (proceed with the next one once the previous pod has restarted and it's Ready)
    ```
    kubectl -n <namespace> delete pod ingester-XXX
    ```
  - Restarting an ingester typically reduces the memory allocated by mmap-ed files. After the restart, ingester may allocate this memory again over time, but it may give more time while working on a longer term solution
- Check the `Mimir / Writes Resources` dashboard to see if the number of series per ingester is above the target (1.5M). If so:
  - Scale up ingesters; you can use e.g. the `Mimir / Scaling` dashboard for reference, in order to determine the needed amount of ingesters (also keep in mind that each ingester should handle ~1.5 million series, and the series will be duplicated across three instances)
  - Memory is expected to be reclaimed at the next TSDB head compaction (occurring every 2h)

### MimirGossipMembersMismatch

This alert fires when any instance does not register all other instances as members of the memberlist cluster.

How it **works**:

- This alert applies when memberlist is used as KV store for hash rings.
- All Mimir instances using the ring, regardless of type, join a single memberlist cluster.
- Each instance (ie. memberlist cluster member) should see all memberlist cluster members.
- Therefore the following should be equal for every instance:
  - The reported number of cluster members (`memberlist_client_cluster_members_count`)
  - The total number of currently responsive instances that use memberlist KV store for hash ring.

How to **investigate**:

- The instance which has the incomplete view of the cluster (too few members) is specified in the alert.
- If the count is zero:
  - It is possible that the joining the cluster has yet to succeed.
  - The following log message indicates that the _initial_ initial join did not succeed: `failed to join memberlist cluster`
  - The following log message indicates that subsequent re-join attempts are failing: `re-joining memberlist cluster failed`
  - If it is the case that the initial join failed, take action according to the reason given.
- Verify communication with other members by checking memberlist traffic is being sent and received by the instance using the following metrics:
  - `memberlist_tcp_transport_packets_received_total`
  - `memberlist_tcp_transport_packets_sent_total`
- If traffic is present, then verify there are no errors sending or receiving packets using the following metrics:
  - `memberlist_tcp_transport_packets_sent_errors_total`
  - `memberlist_tcp_transport_packets_received_errors_total`
  - These errors (and others) can be found by searching for messages prefixed with `TCPTransport:`.
- Logs coming directly from memberlist are also logged by Mimir; they may indicate where to investigate further. These can be identified as such due to being tagged with `caller=memberlist_logger.go:<line>`.

### EtcdAllocatingTooMuchMemory

This can be triggered if there are too many HA dedupe keys in etcd. We saw this when one of our clusters hit 20K tenants that were using HA dedupe config. Raise the etcd limits via:

```
  etcd+: {
    spec+: {
      pod+: {
        resources+: {
          limits: {
            memory: '2Gi',
          },
        },
      },
    },
  },
```

### MimirAlertmanagerSyncConfigsFailing

How it **works**:

This alert is fired when the multi-tenant alertmanager cannot load alertmanager configs from the remote object store for at least 30 minutes.

Loading the alertmanager configs can happen in the following situations:

1. When the multi tenant alertmanager is started
2. Each time it polls for config changes in the alertmanager
3. When there is a ring change

The metric for this alert is cortex_alertmanager_sync_configs_failed_total and is incremented each time one of the above fails.

When there is a ring change or the interval has elapsed, a failure to load configs from the store is logged as a warning.

How to **investigate**:

Look at the error message that is logged and attempt to understand what is causing the failure. I.e. it could be a networking issue, incorrect configuration for the store, etc.

### MimirAlertmanagerRingCheckFailing

How it **works**:

This alert is fired when the multi-tenant alertmanager has been unable to check if one or more tenants should be owned on this shard for at least 10 minutes.

When the alertmanager loads its configuration on start up, when it polls for config changes or when there is a ring change it must check the ring to see if the tenant is still owned on this shard. To prevent one error from causing the loading of all configurations to fail we assume that on error the tenant is NOT owned for this shard. If checking the ring continues to fail then some tenants might not be assigned an alertmanager and might not be able to receive notifications for their alerts.

The metric for this alert is `cortex_alertmanager_ring_check_errors_total`.

How to **investigate**:

- Look at the error message that is logged and attempt to understand what is causing the failure. In most cases the error will be encountered when attempting to read from the ring, which can fail if there is an issue with in-use backend implementation.
- When using Memberlist as KV store for hash rings, ensure that Memberlist is working correctly. See instructions for [`MimirGossipMembersMismatch`](#MimirGossipMembersMismatch) alert.

### MimirAlertmanagerPartialStateMergeFailing

How it **works**:

This alert is fired when the multi-tenant alertmanager attempts to merge a partial state for something that it either does not know about or the partial state cannot be merged with the existing local state. State merges are gRPC messages that are gossiped between a shard and the corresponding alertmanager instance in other shards.

The metric for this alert is cortex_alertmanager_partial_state_merges_failed_total.

How to **investigate**:

The error is not currently logged on the receiver side. If this alert is firing, it is likely that `MimirAlertmanagerReplicationFailing` is firing also, so instead follow the investigation steps for that alert, with the assumption that the issue is not RPC/communication related.

### MimirAlertmanagerReplicationFailing

How it **works**:

This alert is fired when the multi-tenant alertmanager attempts to replicate a state update for a tenant (i.e. a silence or a notification) to another alertmanager instance but failed. This could be due to an RPC/communication error or the other alertmanager being unable to merge the state with its own local state.

The metric for this alert is cortex_alertmanager_state_replication_failed_total.

How to **investigate**:

When state replication fails it gets logged as an error in the alertmanager that attempted the state replication. Check the error message in the log to understand the cause of the error (i.e. was it due to an RPC/communication error or was there an error in the receiving alertmanager).

### MimirAlertmanagerPersistStateFailing

How it **works**:

This alert is fired when the multi-tenant alertmanager cannot persist its state to the remote object store. This operation is attempted periodically (every 15m by default).

Each alertmanager writes its state (silences, notification log) to the remote object storage and the cortex_alertmanager_state_persist_failed_total metric is incremented each time this fails. The alert fires if this fails for an hour or more.

How to **investigate**:

Each failure to persist state to the remote object storage is logged. Find the reason in the Alertmanager container logs with the text “failed to persist state”. Possibles reasons:

- The most probable cause is that remote write failed. Try to investigate why based on the message (network issue, storage issue). If the error indicates the issue might be transient, then you can wait until the next periodic attempt and see if it succeeds.
- It is also possible that encoding the state failed. This does not depend on external factors as it is just pulling state from the Alertmanager internal state. It may indicate a bug in the encoding method.

### MimirAlertmanagerInitialSyncFailed

How it **works**:

When a tenant replica becomes owned it is assigned to an alertmanager instance. The alertmanager instance attempts to read the state from other alertmanager instances. If no other alertmanager instances could replicate the full state then it attempts to read the full state from the remote object store. This alert fires when both of these operations fail.

Note that the case where there is no state for this user in remote object storage, is not treated as a failure. This is expected when a new tenant becomes active for the first time.

How to **investigate**:

When an alertmanager cannot read the state for a tenant from storage it gets logged as the following error: "failed to read state from storage; continuing anyway". The possible causes of this error could be:

- The state could not be merged because it might be invalid and could not be decoded. This could indicate data corruption and therefore a bug in the reading or writing of the state, and would need further investigation.
- The state could not be read from storage. This could be due to a networking issue such as a timeout or an authentication and authorization issue with the remote object store.

### MimirAlertmanagerAllocatingTooMuchMemory

This alert fires when alertmanager memory utilization is getting too close to the limit.

How it **works**:

- Mimir alertmanager is an stateful service
- Having 2+ alertmanagers `OOMKilled` might cause service interruption as it needs quorum for API responses. Notification (from alertmanager to third-party) can succeed without quorum.
- Alertmanager memory baseline usage is primarily influenced by memory allocated by the process (mostly Go heap) for alerts and silences.
- A pod gets `OOMKilled` once its working set memory reaches the configured limit, so it's important to prevent alertmanager's memory utilization (working set memory) from going over to the limit. The memory usage is typically sustained and does not suffer from spikes, hence thresholds are set very close to the limit.

How to **fix** it:

- Scale up alertmanager replicas; you can use e.g. the `Mimir / Scaling` dashboard for reference, in order to determine the needed amount of alertmanagers.

### MimirRolloutStuck

This alert fires when a Mimir service rollout is stuck, which means the number of updated replicas doesn't match the expected one and looks there's no progress in the rollout. The alert monitors services deployed as Kubernetes `StatefulSet` and `Deployment`.

How to **investigate**:

- Run `kubectl -n <namespace> get pods -l name=<statefulset|deployment>` to get a list of running pods
- Ensure there's no pod in a failing state (eg. `Error`, `OOMKilled`, `CrashLoopBackOff`)
- Ensure there's no pod `NotReady` (the number of ready containers should match the total number of containers, eg. `1/1` or `2/2`)
- Run `kubectl -n <namespace> describe statefulset <name>` or `kubectl -n <namespace> describe deployment <name>` and look at "Pod Status" and "Events" to get more information

### MimirKVStoreFailure

This alert fires if a Mimir instance is failing to run any operation on a KV store (eg. consul or etcd).
When using Memberlist as KV store for hash rings, all read and update operations work on a local copy of the hash ring, and will never fail and raise this alert.

How it **works**:

- Consul is typically used to store the hash ring state.
- Etcd is typically used to store by the HA tracker (distributor) to deduplicate samples.
- If an instance is failing operations on the **hash ring**, either the instance can't update the heartbeat in the ring or is failing to receive ring updates.
- If an instance is failing operations on the **HA tracker** backend, either the instance can't update the authoritative replica or is failing to receive updates.

How to **investigate**:

- Ensure Consul/Etcd is up and running.
- Investigate the logs of the affected instance to find the specific error occurring when talking to Consul/Etcd.

### MimirReachingTCPConnectionsLimit

This alert fires if a Mimir instance is configured with `-server.http-conn-limit` or `-server.grpc-conn-limit` and is reaching the limit.

How it **works**:

- A Mimir service could be configured with a limit of the max number of TCP connections accepted simultaneously on the HTTP and/or gRPC port.
- If the limit is reached:
  - New connections acceptance will put on hold or rejected. Exact behaviour depends on backlog parameter to `listen()` call and kernel settings.
  - The **health check endpoint may fail** (eg. timeout).
- The limit is typically set way higher than expected usage, so if limit is reached (or close to be) then it means there's a critical issue.

How to **investigate**:

- Limit reached in `gateway`:
  - Check if it's caused by an **high latency on write path**:
    - Check the distributors and ingesters latency in the `Mimir / Writes` dashboard
    - An high latency on write path could lead our customers Prometheus / Agent to increase the number of shards nearly at the same time, leading to a significantly higher number of concurrent requests to the load balancer and thus gateway
  - Check if it's caused by a **single tenant**:
    - We don't have a metric tracking the active TCP connections or QPS per tenant
    - As a proxy metric, you can check if the ingestion rate has significantly increased for any tenant (it's not a very accurate proxy metric for number of TCP connections so take it with a grain of salt):
    ```
    topk(10, sum by(user) (rate(cortex_distributor_samples_in_total{namespace="<namespace>"}[$__rate_interval])))
    ```
    - In case you need to quickly reject write path traffic from a single tenant, you can override its `ingestion_rate` and `ingestion_rate_burst` setting lower values (so that some/most of their traffic will be rejected)

### MimirQuerierAutoscalerNotActive

This alert fires when the Mimir querier Kubernetes Horizontal Pod Autoscaler's (HPA) `ScalingActive` condition is `false`. When this happens, it's not able to calculate desired scale and generally indicates problems with fetching metrics.

How it **works**:

- HPA is configured to autoscale Mimir queriers based on custom metrics fetched from Prometheus via the KEDA custom metrics API server
- HPA periodically queries updated metrics and updates the number of desired replicas based on that
- Please refer to the [HPA documentation](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) for more information about it

How to **investigate**:

- Check HPA conditions and events to get more details about the failure
  ```
  kubectl describe hpa -n <namespace> keda-hpa-querier
  ```
- Ensure KEDA custom metrics API server is up and running
  ```
  # Assuming KEDA is running in a dedicated namespace "keda":
  kubectl get pods -n keda
  ```
- Check KEDA custom metrics API server logs
  ```
  # Assuming KEDA is running in a dedicated namespace "keda":
  kubectl logs -n keda deployment/keda-operator-metrics-apiserver
  ```

### MimirContinuousTestNotRunningOnWrites

This alert fires when `mimir-continuous-test` is deployed in the Mimir cluster, and continuous testing is not effectively running because writes are failing.

How it **works**:

- `mimir-continuous-test` is an optional testing tool that can be deployed in the Mimir cluster
- The tool runs some tests against the Mimir cluster itself at regular intervals
- This alert fires if the tool is unable to properly run the tests, and not if the tool assertions don't match the expected results

How to **investigate**:

- Check continuous test logs to find out more details about the failure:
  ```
  kubectl logs -n <namespace> deployment/continuous-test
  ```

### MimirContinuousTestNotRunningOnReads

This alert is like [`MimirContinuousTestNotRunningOnWrites`](#MimirContinuousTestNotRunningOnWrites) but it fires when queries are failing.

### MimirContinuousTestFailed

This alert fires when `mimir-continuous-test` is deployed in the Mimir cluster, and continuous testing tool's assertions don't match the expected results.
When this alert fires there could be a bug in Mimir that should be investigated as soon as possible.

How it **works**:

- `mimir-continuous-test` is an optional testing tool that can be deployed in the Mimir cluster
- The tool runs some tests against the Mimir cluster itself at regular intervals
- This alert fires if the tool assertions don't match the expected results

How to **investigate**:

- Check continuous test logs to find out more details about the failed assertions:
  ```
  kubectl logs -n <namespace> deployment/continuous-test
  ```
- Check if query result comparison is failing
  - Is query failing both when results cache is enabled and when it's disabled?
- This alert should always be actionable. There are two possible outcomes:
  1. The alert fired because of a bug in Mimir: fix it.
  1. The alert fired because of a bug or edge case in the continuous test tool, causing a false positive: fix it.

### RolloutOperatorNotReconciling

This alert fires if the [`rollout-operator`](https://github.com/grafana/rollout-operator) is not successfully reconciling in a namespace.

How it **works**:

- The rollout-operator coordinates the rollout of pods between different StatefulSets within a specific namespace and is used to manage multi-zone deployments
- The rollout-operator is deployed in namespaces where some Mimir components (e.g. ingesters) are deployed in multi-zone
- The rollout-operator reconciles as soon as there's any change in observed Kubernetes resources or every 5m at most

How to **investigate**:

- Check rollout-operator logs to find more details about the error, e.g. with the following Grafana Loki query:
  ```
  {name="rollout-operator",namespace="<namespace>"}
  ```

## Errors catalog

Mimir has some codified error IDs that you might see in HTTP responses or logs.
These error IDs allow you to read related details in the documentation that follows.

### err-mimir-missing-metric-name

This non-critical error occurs when Mimir receives a write request that contains a series without a metric name.
Each series must have a metric name. Rarely it does not, in which case there might be a bug in the sender client.

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-metric-name-invalid

This non-critical error occurs when Mimir receives a write request that contains a series with an invalid metric name.
A metric name can only contain characters as defined by Prometheus’ [Metric names and labels](https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels).

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-max-label-names-per-series

This non-critical error occurs when Mimir receives a write request that contains a series with a number of labels that exceed the configured limit.
The limit protects the system’s stability from potential abuse or mistakes. To configure the limit on a per-tenant basis, use the `-validation.max-label-names-per-series` option.

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-label-invalid

This non-critical error occurs when Mimir receives a write request that contains a series with an invalid label name.
A label name name can only contain characters as defined by Prometheus’ [Metric names and labels](https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels).

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-label-name-too-long

This non-critical error occurs when Mimir receives a write request that contains a series with a label name whose length exceeds the configured limit.
The limit protects the system’s stability from potential abuse or mistakes. To configure the limit on a per-tenant basis, use the `-validation.max-length-label-name` option.

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-label-value-too-long

This non-critical error occurs when Mimir receives a write request that contains a series with a label value whose length exceeds the configured limit.
The limit protects the system’s stability from potential abuse or mistakes. To configure the limit on a per-tenant basis, use the `-validation.max-length-label-value` option.

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-duplicate-label-names

This non-critical error occurs when Mimir receives a write request that contains a series with the same label name two or more times.
A series that contains a duplicated label name is invalid and gets skipped during the ingestion.

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-labels-not-sorted

This error occurs when Mimir receives a write request that contains a series whose label names are not sorted alphabetically.
However, Mimir internally sorts labels for series that it receives, so this error should not occur in practice.
If you experience this error, [open an issue in the Mimir repository](https://github.com/grafana/mimir/issues).

> **Note**: Invalid series are skipped during the ingestion, and valid series within the same request are ingested.

### err-mimir-too-far-in-future

This non-critical error occurs when Mimir receives a write request that contains a sample whose timestamp is in the future compared to the current "real world" time.
Mimir accepts timestamps that are slightly in the future, due to skewed clocks for example. It rejects timestamps that are too far in the future, based on the definition that you can set via the `-validation.create-grace-period` option.
On a per-tenant basis, you can fine tune the tolerance by configuring the `-validation.max-length-label-value` option.

> **Note**: Series with invalid samples are skipped during the ingestion, and series within the same request are ingested.

### err-mimir-exemplar-labels-missing

This non-critical error occurs when Mimir receives a write request that contains an exemplar without a label that identifies the related metric.
An exemplar must have at least one valid label pair, otherwise it cannot be associated with any metric.

> **Note**: Invalid exemplars are skipped during the ingestion, and valid exemplars within the same request are ingested.

### err-mimir-exemplar-labels-too-long

This non-critical error occurs when Mimir receives a write request that contains an exemplar where the combined set size of its labels exceeds the limit.
The limit is used to protect the system’s stability from potential abuse or mistakes, and it cannot be configured.

> **Note**: Invalid exemplars are skipped during the ingestion, and valid exemplars within the same request are ingested.

### err-mimir-exemplar-timestamp-invalid

This non-critical error occurs when Mimir receives a write request that contains an exemplar without a timestamp.
An exemplar must have a valid timestamp, otherwise it cannot be correlated to any point in time.

> **Note**: Invalid exemplars are skipped during the ingestion, and valid exemplars within the same request are ingested.

### err-mimir-metadata-missing-metric-name

This non-critical error occurs when Mimir receives a write request that contains a metric metadata without a metric name.
Each metric metadata must have a metric name. Rarely it does not, in which case there might be a bug in the sender client.

> **Note**: Invalid metrics metadata are skipped during the ingestion, and valid metadata within the same request are ingested.

### err-mimir-metric-name-too-long

This non-critical error occurs when Mimir receives a write request that contains a metric metadata with a metric name whose length exceeds the configured limit.
The limit protects the system’s stability from potential abuse or mistakes. To configure the limit on a per-tenant basis, use the `-validation.max-metadata-length` option.

> **Note**: Invalid metrics metadata are skipped during the ingestion, and valid metadata within the same request are ingested.

### err-mimir-unit-too-long

This non-critical error occurs when Mimir receives a write request that contains a metric metadata with unit name whose length exceeds the configured limit.
The limit protects the system’s stability from potential abuse or mistakes. To configure the limit on a per-tenant basis, use the `-validation.max-metadata-length` option.

> **Note**: Invalid metrics metadata are skipped during the ingestion, and valid metadata within the same request are ingested.

### err-mimir-distributor-max-ingestion-rate

This critical error occurs when the rate of received samples, exemplars and metadata per second is exceeded in a distributor.

The distributor implements a rate limit on the samples per second that can be ingested, and it's used to protect a distributor from overloading in case of high traffic.
This per-instance limit is applied to all samples, exemplars, and all of the metadata that it receives.
Also, the limit spans all of the tenants within each distributor.

How to **fix** it:

- Scale up the distributors.
- Increase the limit by using the `-distributor.instance-limits.max-ingestion-rate` option.

### err-mimir-distributor-max-inflight-push-requests

This error occurs when a distributor rejects a write request because the maximum in-flight requests limit has been reached.

How it **works**:

- The distributor has a per-instance limit on the number of in-flight write (push) requests.
- The limit applies to all in-flight write requests, across all tenants, and it protects the distributor from becoming overloaded in case of high traffic.
- To configure the limit, set the `-distributor.instance-limits.max-inflight-push-requests` option.

How to **fix** it:

- Increase the limit by setting the `-distributor.instance-limits.max-inflight-push-requests` option.
- Check the write requests latency through the `Mimir / Writes` dashboard and come back to investigate the root cause of high latency (the higher the latency, the higher the number of in-flight write requests).
- Consider scaling out the distributors.

### err-mimir-distributor-max-inflight-push-requests-bytes

This error occurs when a distributor rejects a write request because the total size in bytes of all in-flight requests limit has been reached.

How it **works**:

- The distributor has a per-instance limit on the total size in bytes of all in-flight write (push) requests.
- The limit applies to all in-flight write requests, across all tenants, and it protects the distributor from going out of memory in case of high traffic or high latency on the write path.
- To configure the limit, set the `-distributor.instance-limits.max-inflight-push-requests-bytes` option.

How to **fix** it:

- Increase the limit by setting the `-distributor.instance-limits.max-inflight-push-requests-bytes` option.
- Check the write requests latency through the `Mimir / Writes` dashboard and come back to investigate the root cause of the increased size of requests or the increased latency (the higher the latency, the higher the number of in-flight write requests, the higher their combined size).
- Consider scaling out the distributors.

### err-mimir-ingester-max-ingestion-rate

This critical error occurs when the rate of received samples per second is exceeded in an ingester.

The ingester implements a rate limit on the samples per second that can be ingested, and it's used to protect an ingester from overloading in case of high traffic.
This per-instance limit is applied to all samples that it receives.
Also, the limit spans all of the tenants within each ingester.

How to **fix** it:

- Scale up the ingesters.
- Increase the limit by using the `-ingester.instance-limits.max-ingestion-rate` option (or `max_ingestion_rate` in the runtime config).

### err-mimir-ingester-max-tenants

This critical error occurs when the ingester receives a write request for a new tenant (a tenant for which no series have been stored yet) but the ingester cannot accept it because the maximum number of allowed tenants per ingester has been reached.

How to **fix** it:

- Increase the limit by using the `-ingester.instance-limits.max-tenants` option (or `max_tenants` in the runtime config).
- Consider configuring ingesters shuffle sharding to reduce the number of tenants per ingester.

### err-mimir-ingester-max-series

This critical error occurs when an ingester rejects a write request because it reached the maximum number of in-memory series.

How it **works**:

- The ingester keeps most recent series data in-memory.
- The ingester has a per-instance limit on the number of in-memory series, used to protect the ingester from overloading in case of high traffic.
- When the limit on the number of in-memory series is reached, new series are rejected, while samples can still be appended to existing ones.
- To configure the limit, set the `-ingester.instance-limits.max-series` option (or `max_series` in the runtime config).

How to **fix** it:

- See [`MimirIngesterReachingSeriesLimit`](#MimirIngesterReachingSeriesLimit) runbook.

### err-mimir-ingester-max-inflight-push-requests

This error occurs when an ingester rejects a write request because the maximum in-flight requests limit has been reached.

How it **works**:

- The ingester has a per-instance limit on the number of in-flight write (push) requests.
- The limit applies to all in-flight write requests, across all tenants, and it protects the ingester from becoming overloaded in case of high traffic.
- To configure the limit, set the `-ingester.instance-limits.max-inflight-push-requests` option (or `max_inflight_push_requests` in the runtime config).

How to **fix** it:

- Increase the limit by setting the `-ingester.instance-limits.max-inflight-push-requests` option (or `max_inflight_push_requests` in the runtime config).
- Check the write requests latency through the `Mimir / Writes` dashboard and come back to investigate the root cause of high latency (the higher the latency, the higher the number of in-flight write requests).
- Consider scaling out the ingesters.

### err-mimir-max-series-per-user

This error occurs when the number of in-memory series for a given tenant exceeds the configured limit.

The limit is used to protect ingesters from overloading in case a tenant writes a high number of series, as well as to protect the whole system’s stability from potential abuse or mistakes.
To configure the limit on a per-tenant basis, use the `-ingester.max-global-series-per-user` option (or `max_global_series_per_user` in the runtime configuration).

How to **fix** it:

- Ensure the actual number of series written by the affected tenant is legit.
- Consider increasing the per-tenant limit by using the `-ingester.max-global-series-per-user` option (or `max_global_series_per_user` in the runtime configuration).

### err-mimir-max-series-per-metric

This error occurs when the number of in-memory series for a given tenant and metric name exceeds the configured limit.

The limit is primarily used to protect a tenant from potential mistakes on their metrics instrumentation.
For example, if an instrumented application exposes a metric with a label value including very dynamic data (e.g. a timestamp) the ingestion of that metric would quickly lead to hit the per-tenant series limit, causing other metrics to be rejected too.
This limit introduces a cap on the maximum number of series each metric name can have, rejecting exceeding series only for that metric name, before the per-tenant series limit is reached.
To configure the limit on a per-tenant basis, use the `-ingester.max-global-series-per-metric` option (or `max_global_series_per_metric` in the runtime configuration).

How to **fix** it:

- Check the details in the error message to find out which is the affected metric name.
- Investigate if the high number of series exposed for the affected metric name is legit.
- Consider reducing the cardinality of the affected metric, by tuning or removing some of its labels.
- Consider increasing the per-tenant limit by using the `-ingester.max-global-series-per-metric` option.
- Consider excluding specific metric names from this limit's check by using the `-ingester.ignore-series-limit-for-metric-names` option (or `max_global_series_per_metric` in the runtime configuration).

### err-mimir-max-metadata-per-user

This non-critical error occurs when the number of in-memory metrics with metadata for a given tenant exceeds the configured limit.

Metric metadata is a set of information attached to a metric name, like its unit (e.g. counter) and description.
Metric metadata can be included by the sender in the write request, and it's returned when querying the `/api/v1/metadata` API endpoint.
Metric metadata is stored in the ingesters memory, so the higher the number of metrics metadata stored, the higher the memory utilization.

Mimir has a per-tenant limit of the number of metric names that have metadata attached.
This limit is used to protect the whole system’s stability from potential abuse or mistakes.
To configure the limit on a per-tenant basis, use the `-ingester.max-global-series-per-user` option (or `max_global_metadata_per_user` in the runtime configuration).

How to **fix** it:

- Check the current number of metric names for the affected tenant, running the instant query `count(count by(__name__) ({__name__=~".+"}))`. Alternatively, you can get the cardinality of `__name__` label calling the API endpoint `/api/v1/cardinality/label_names`.
- Consider increasing the per-tenant limit setting to a value greater than the number of unique metric names returned by the previous query.

### err-mimir-max-metadata-per-metric

This non-critical error occurs when the number of different metadata for a given metric name exceeds the configured limit.

Metric metadata is a set of information attached to a metric name, like its unit (e.g. counter) and description.
Typically, for a given metric name there's only one set of metadata (e.g. the same metric name exposed by different application has the same counter and description).
However, there could be some edge cases where the same metric name has a different meaning between applications or the same meaning but a slightly different description.
In these edge cases, different applications would expose different metadata for the same metric name.

This limit is used to protect the whole system’s stability from potential abuse or mistakes, in case the number of metadata variants for a given metric name grows indefinitely.
To configure the limit on a per-tenant basis, use the `-ingester.max-global-series-per-metric` option (or `max_global_metadata_per_metric` in the runtime configuration).

How to **fix** it:

- Check the metadata for the affected metric name, querying the `/api/v1/metadata?metric=<name>` API endpoint (replace `<name>` with the metric name).
- If the different metadata is unexpected, consider fixing the discrepancy in the instrumented applications.
- If the different metadata is expected, consider increasing the per-tenant limit by using the `-ingester.max-global-series-per-metric` option (or `max_global_metadata_per_metric` in the runtime configuration).

### err-mimir-max-chunks-per-query

This error occurs when a query execution exceeds the limit on the number of series chunks fetched.

This limit is used to protect the system’s stability from potential abuse or mistakes, when running a query fetching a huge amount of data.
To configure the limit on a per-tenant basis, use the `-querier.max-fetched-chunks-per-query` option (or `max_fetched_chunks_per_query` in the runtime configuration).

How to **fix** it:

- Consider reducing the time range and/or cardinality of the query. To reduce the cardinality of the query, you can add more label matchers to the query, restricting the set of matching series.
- Consider increasing the per-tenant limit by using the `-querier.max-fetched-chunks-per-query` option (or `max_fetched_chunks_per_query` in the runtime configuration).

### err-mimir-max-series-per-query

This error occurs when a query execution exceeds the limit on the maximum number of series.

This limit is used to protect the system’s stability from potential abuse or mistakes, when running a query fetching a huge amount of data.
To configure the limit on a per-tenant basis, use the `-querier.max-fetched-series-per-query` option (or `max_fetched_series_per_query` in the runtime configuration).

How to **fix** it:

- Consider reducing the time range and/or cardinality of the query. To reduce the cardinality of the query, you can add more label matchers to the query, restricting the set of matching series.
- Consider increasing the per-tenant limit by using the `-querier.max-fetched-series-per-query` option (or `max_fetched_series_per_query` in the runtime configuration).

### err-mimir-max-chunks-bytes-per-query

This error occurs when a query execution exceeds the limit on aggregated size (in bytes) of fetched chunks.

This limit is used to protect the system’s stability from potential abuse or mistakes, when running a query fetching a huge amount of data.
To configure the limit on a per-tenant basis, use the `-querier.max-fetched-chunk-bytes-per-query` option (or `max_fetched_chunk_bytes_per_query` in the runtime configuration).

How to **fix** it:

- Consider reducing the time range and/or cardinality of the query. To reduce the cardinality of the query, you can add more label matchers to the query, restricting the set of matching series.
- Consider increasing the per-tenant limit by using the `-querier.max-fetched-chunk-bytes-per-query` option (or `max_fetched_chunk_bytes_per_query` in the runtime configuration).

### err-mimir-max-query-length

This error occurs when the time range of a query exceeds the configured maximum length.

Both PromQL instant and range queries can fetch metrics data over a period of time.
A [range query](https://prometheus.io/docs/prometheus/latest/querying/api/#range-queries) requires a `start` and `end` timestamp, so the difference of `end` minus `start` is the time range length of the query.
An [instant query](https://prometheus.io/docs/prometheus/latest/querying/api/#instant-queries) requires a `time` parameter and the query is executed fetching samples at that point in time.
However, even an instant query can fetch metrics data over a period of time by using the [range vector selectors](https://prometheus.io/docs/prometheus/latest/querying/basics/#range-vector-selectors).
For example, the instant query `sum(rate(http_requests_total{job="prometheus"}[1h]))` fetches metrics over a 1 hour period.
This time period is what Grafana Mimir calls the _query time range length_ (or _query length_).

Mimir has a limit on the query length.
This limit is applied to partial queries, after they've split (according to time) by the query-frontend. This limit protects the system’s stability from potential abuse or mistakes.
To configure the limit on a per-tenant basis, use the `-store.max-query-length` option (or `max_query_length` in the runtime configuration).

### err-mimir-tenant-max-request-rate

This error occurs when the rate of write requests per second is exceeded for this tenant.

How it **works**:

- There is a per-tenant rate limit on the write requests per second, and it's applied across all distributors for this tenant.
- The limit is implemented using [token buckets](https://en.wikipedia.org/wiki/Token_bucket).

How to **fix** it:

- Increase the per-tenant limit by using the `-distributor.request-rate-limit` (requests per second) and `-distributor.request-burst-size` (number of requests) options (or `request_rate` and `request_burst_size` in the runtime configuration). The configurable burst represents how many requests can temporarily exceed the limit, in case of short traffic peaks. The configured burst size must be greater or equal than the configured limit.

### err-mimir-tenant-max-ingestion-rate

This error occurs when the rate of received samples, exemplars and metadata per second is exceeded for this tenant.

How it **works**:

- There is a per-tenant rate limit on the samples, exemplars and metadata that can be ingested per second, and it's applied across all distributors for this tenant.
- The limit is implemented using [token buckets](https://en.wikipedia.org/wiki/Token_bucket).

How to **fix** it:

- Increase the per-tenant limit by using the `-distributor.ingestion-rate-limit` (samples per second) and `-distributor.ingestion-burst-size` (number of samples) options (or `ingestion_rate` and `ingestion_burst_size` in the runtime configuration). The configurable burst represents how many samples, exemplars and metadata can temporarily exceed the limit, in case of short traffic peaks. The configured burst size must be greater or equal than the configured limit.

### err-mimir-tenant-too-many-ha-clusters

This error occurs when a distributor rejects a write request because the number of [high-availability (HA) clusters]({{< relref "../configure/configuring-high-availability-deduplication.md" >}}) has hit the configured limit for this tenant.

How it **works**:

- The distributor implements an upper limit on the number of clusters that the HA tracker will keep track of for a single tenant.
- It is triggered when the write request would add a new cluster while the number the tenant currently has is already equal to the limit.
- To configure the limit, set the `-distributor.ha-tracker.max-clusters` option (or `ha_max_clusters` in the runtime configuration).

How to **fix** it:

- Increase the per-tenant limit by using the `-distributor.ha-tracker.max-clusters` option (or `ha_max_clusters` in the runtime configuration).

### err-mimir-sample-timestamp-too-old

This error occurs when the ingester rejects a sample because its timestamp is too old as compared to the most recent timestamp received for the same tenant across all its time series.

How it **works**:

- If the incoming timestamp is more than 1 hour older than the most recent timestamp ingested for the tenant, the sample will be rejected.

> **Note**: If the out-of-order sample ingestion is enabled, then this error is similar to `err-mimir-sample-out-of-order` below with a difference that the sample is older than the out-of-order time window as it relates to the latest sample for that particular time series or the TSDB.

### err-mimir-sample-out-of-order

This error occurs when the ingester rejects a sample because another sample with a more recent timestamp has already been ingested.

How it **works**:

- Currently, samples are not allowed to be ingested out of order for a given series.

Common **causes**:

- Your code has a single target that exposes the same time series multiple times, or multiple targets with identical labels.
- System time of your Prometheus instance has been shifted backwards. If this was a mistake, fix the system time back to normal. Otherwise, wait until the system time catches up to the time it was changed.
- You are running multiple Prometheus instances pushing the same metrics and [your high-availability tracker is not properly configured for deduplication]({{< relref "../configure/configuring-high-availability-deduplication.md" >}}).
- Prometheus relabelling has been configured and it causes series to clash after the relabelling. Check the error message for information about which series has received a sample out of order.
- A Prometheus instance was restarted, and it pushed all data from its Write-Ahead Log to remote write upon restart, some of which has already been pushed and ingested. This is normal and can be ignored.

> **Note**: You can learn more about out of order samples in Prometheus, in the blog post [Debugging out of order samples](https://www.robustperception.io/debugging-out-of-order-samples/).

### err-mimir-sample-duplicate-timestamp

This error occurs when the ingester rejects a sample because it is a duplicate of a previously received sample with the same timestamp but different value in the same time series.

Common **causes**:

- Multiple endpoints are exporting the same metrics, or multiple Prometheus instances are scraping different metrics with identical labels.
- Prometheus relabelling has been configured and it causes series to clash after the relabelling. Check the error message for information about which series has received a duplicate sample.

### err-mimir-exemplar-series-missing

This error occurs when the ingester rejects an exemplar because its related series has not been ingested yet.

How it **works**:

- The series must already exist before exemplars can be appended, as we do not create new series upon ingesting exemplars. The series will be created when a sample from it is ingested.

### err-mimir-store-consistency-check-failed

This error occurs when the querier is unable to fetch some of the expected blocks after multiple retries and connections to different store-gateways. The query fails because some blocks are missing in the queried store-gateways.

How it **works**:

- Mimir has been designed to guarantee query results correctness and never return partial query results. Either a query succeeds returning fully consistent results or it fails.
- Queriers, and rulers running with the "internal" evaluation mode, run a consistency check to ensure all expected blocks have been queried from the long-term storage via the store-gateways.
- If any expected block has not been queried via the store-gateways, then the query fails with this error.
- See [Anatomy of a query request]({{< relref "../architecture/components/querier.md#anatomy-of-a-query-request" >}}) to learn more.

How to **fix** it:

- Ensure all store-gateways are healthy.
- Ensure all store-gateways are successfully synching owned blocks (see [`MimirStoreGatewayHasNotSyncTheBucket`](#MimirStoreGatewayHasNotSyncTheBucket)).

### err-mimir-bucket-index-too-old

This error occurs when a query fails because the bucket index is too old.

How it **works**:

- Compactors periodically write a per-tenant file, called the "bucket index", to the object storage. The bucket index contains all known blocks for the given tenant and is updated every `-compactor.cleanup-interval`.
- When a query is executed, queriers and rulers running with the "internal" evaluation mode look up the bucket index to find which blocks should be queried through the store-gateways.
- To ensure all required blocks are queried, queriers and rulers determine how old a bucket index is based on the time that it was last updated by the compactor.
- If the age is older than the maximum stale period that is configured via `-blocks-storage.bucket-store.bucket-index.max-stale-period`, the query fails.
- This circuit breaker ensures that the queriers and rulers do not return any partial query results due to a stale view of the long-term storage.

How to **fix** it:

- Ensure the compactor is running successfully (e.g. not crashing, not going out of memory).
- Ensure each compactor replica has successfully updated bucket index of each owned tenant within the double of `-compactor.cleanup-interval` (query below assumes the cleanup interval is set to 15 minutes):
  `time() - cortex_compactor_block_cleanup_last_successful_run_timestamp_seconds > 2 * (15 * 60)`

### err-mimir-distributor-max-write-message-size

This error occurs when a distributor rejects a write request because its message size is larger than the allowed limit.

How it **works**:

- The distributor implements an upper limit on the message size of incoming write requests.
- To configure the limit, set the `-distributor.max-recv-msg-size` option.

How to **fix** it:

- Increase the allowed limit by using the `-distributor.max-recv-msg-size` option.

## Mimir routes by path

**Write path**:

- `/distributor.Distributor/Push`
- `/cortex.Ingester/Push`
- `api_v1_push`
- `api_v1_push_influx_write`

**Read path**:

- `/schedulerpb.SchedulerForFrontend/FrontendLoop`
- `/cortex.Ingester/QueryStream`
- `/cortex.Ingester/QueryExemplars`
- `/gatewaypb.StoreGateway/Series`
- `api_prom_api_v1_label_name_values`
- `api_prom_api_v1_labels`
- `api_prom_api_v1_metadata`
- `api_prom_api_v1_query`
- `api_prom_api_v1_query_exemplars`
- `api_prom_api_v1_query_range`
- `api_prom_api_v1_rules`
- `api_prom_api_v1_series`

**Ruler / rules path**:

- `api_v1_rules`
- `api_v1_rules_namespace`
- `prometheus_api_v1_rules`
- `prometheus_rules_namespace`
- `prometheus_rules`

## Mimir blocks storage - What to do when things to wrong

## Recovering from a potential data loss incident

The ingested series data that could be lost during an incident can be stored in two places:

1. Ingesters (before blocks are shipped to the bucket)
2. Bucket

There could be several root causes leading to a potential data loss. In this document we're going to share generic procedures that could be used as a guideline during an incident.

### Halt the compactor

The Mimir cluster continues to successfully operate even if the compactor is not running, except that over a long period (12+ hours) this will lead to query performance degradation. The compactor could potentially be the cause of data loss because:

- It marks blocks for deletion (soft deletion). _This doesn't lead to any immediate deletion, but blocks marked for deletion will be hard deleted once a delay expires._
- It permanently deletes blocks marked for deletion after `-compactor.deletion-delay` (hard deletion)
- It could generate corrupted compacted blocks (eg. due to a bug or if a source block is corrupted and the automatic checks can't detect it)

**If you suspect the compactor could be the cause of data loss, halt it** (delete the statefulset or scale down the replicas to 0). It can be restarted anytime later.

When the compactor is **halted**:

- No new blocks will be compacted
- No blocks will be deleted (soft and hard deletion)

### Recover source blocks from ingesters

Ingesters keep, on their persistent disk, the blocks compacted from TSDB head until the `-experimental.tsdb.retention-period` retention expires. The **default retention is 4 days**, in order to give cluster operators enough time to react in case of a data loss incident.

The blocks retained in the ingesters can be used in case the compactor generates corrupted blocks and the source blocks, shipped from ingesters, have already been hard deleted from the bucket.

How to manually blocks from ingesters to the bucket:

1. Ensure [`gsutil`](https://cloud.google.com/storage/docs/gsutil) is installed in the Mimir pod. If not, [install it](#install-gsutil-in-the-mimir-pod)
2. Run `cd /data/tsdb && /path/to/gsutil -m rsync -n -r -x 'thanos.shipper.json|chunks_head|wal' . gs://<bucket>/recovered/`
   - `-n` enabled the **dry run** (remove it once you've verified the output matches your expectations)
   - `-m` enables parallel mode
   - `-r` enables recursive rsync
   - `-x <pattern>` excludes specific patterns from sync (no WAL or shipper metadata file should be uploaded to the bucket)
   - Don't use `-d` (dangerous) because it will delete from the bucket any block which is not in the local filesystem

### Freeze ingesters persistent disk

The blocks and WAL stored in the ingester persistent disk are the last fence of defence in case of an incident involving blocks not shipped to the bucket or corrupted blocks in the bucket. If the data integrity in the ingester's disk is at risk (eg. close to hit the TSDB retention period or close to reach max disk utilisation), you should freeze it taking a **disk snapshot**.

To take a **GCP persistent disk snapshot**:

1. Identify the Kubernetes PVC volume name (`kubectl get pvc -n <namespace>`) of the volumes to snapshot
2. For each volume, [create a snapshot](https://console.cloud.google.com/compute/snapshotsAdd) from the GCP console ([documentation](https://cloud.google.com/compute/docs/disks/create-snapshots))

### Halt the ingesters

Halting the ingesters should be the **very last resort** because of the side effects. To halt the ingesters, while preserving their disk and without disrupting the cluster write path, you need to:

1. Create a second pool of ingesters

- Uses the functions `newIngesterStatefulSet()`, `newIngesterPdb()`

2. Wait until the second pool is up and running
3. Halt existing ingesters (scale down to 0 or delete their statefulset)

However the **queries will return partial data**, due to all the ingested samples which have not been compacted to blocks yet.

## Manual procedures

### Resizing Persistent Volumes using Kubernetes

This is the short version of an extensive documentation on [how to resize Kubernetes Persistent Volumes](https://kubernetes.io/blog/2018/07/12/resizing-persistent-volumes-using-kubernetes/).

**Pre-requisites**:

- Running Kubernetes v1.11 or above
- The PV storage class has `allowVolumeExpansion: true`
- The PV is backed by a supported block storage volume (eg. GCP-PD, AWS-EBS, ...)

**How to increase the volume**:

1. Edit the PVC (persistent volume claim) `spec` for the volume to resize and **increase** `resources` > `requests` > `storage`
2. Restart the pod attached to the PVC for which the storage request has been increased

### How to create clone volume (Google Cloud specific)

In some scenarios, it may be useful to preserve current volume status for inspection, but keep using the volume.
[Google Persistent Disk supports "Clone"](https://cloud.google.com/compute/docs/disks/add-persistent-disk#source-disk) operation that can be used to do that.
Newly cloned disk is independent from its original, and can be used for further investigation by attaching it to a new Machine / Pod.

When using Kubernetes, here is YAML file that creates PV (`clone-ingester-7-pv`) pointing to the new disk clone (`clone-pvc-80cc0efa-4996-11ea-ba79-42010a96008c` in this example),
PVC (`clone-ingester-7-pvc`) pointing to PV, and finally Pod (`clone-ingester-7-dataaccess`) using the PVC to access the disk.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: clone-ingester-7-pv
spec:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 150Gi
  gcePersistentDisk:
    fsType: ext4
    pdName: clone-pvc-80cc0efa-4996-11ea-ba79-42010a96008c
  persistentVolumeReclaimPolicy: Retain
  storageClassName: fast
  volumeMode: Filesystem
---
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: clone-ingester-7-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 150Gi
  storageClassName: fast
  volumeName: clone-ingester-7-pv
  volumeMode: Filesystem
---
apiVersion: v1
kind: Pod
metadata:
  name: clone-ingester-7-dataaccess
spec:
  containers:
    - name: alpine
      image: alpine:latest
      command: ["sleep", "infinity"]
      volumeMounts:
        - name: mypvc
          mountPath: /data
      resources:
        requests:
          cpu: 500m
          memory: 1024Mi
  volumes:
    - name: mypvc
      persistentVolumeClaim:
        claimName: clone-ingester-7-pvc
```

After this preparation, one can use `kubectl exec -t -i clone-ingester-7-dataaccess /bin/sh` to inspect the disk mounted under `/data`.

### Install `gsutil` in the Mimir pod

1. Install python
   ```
   apk add python3 py3-pip
   ln -s /usr/bin/python3 /usr/bin/python
   pip install google-compute-engine
   ```
2. Download `gsutil`
   ```
   wget https://storage.googleapis.com/pub/gsutil.tar.gz
   tar -zxvf gsutil.tar.gz
   ./gsutil/gsutil --help
   ```
3. Configure credentials

   ```
   gsutil config -e

   # Private key path: /var/secrets/google/credentials.json
   # Project ID: your google project ID
   ```

### Deleting a StatefulSet with persistent volumes

When you delete a Kubernetes StatefulSet whose pods have persistent volume claims (PVC), the PVCs are not automatically deleted. This means that if the StatefulSet is recreated, the pods for which there was already a PVC will get the volume mounted previously.

A PVC can be manually deleted by an operator. When a PVC claim is deleted, what happens to the volume depends on its [Reclaim Policy](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#reclaiming):

- `Retain`: the volume will not be deleted until the PV resource will be manually deleted from Kubernetes
- `Delete`: the volume will be automatically deleted

### Recover accidentally deleted blocks (Google Cloud specific)

_This runbook assumes you've enabled versioning in your GCS bucket and the retention of deleted blocks didn't expire yet._

These are just example actions but should give you a fair idea on how you could go about doing this. Read the [GCS doc](https://cloud.google.com/storage/docs/using-versioned-objects#gsutil_1) before you proceed.

Step 1: Use `gsutil ls -l -a $BUCKET` to list all blocks, including the deleted ones. Now identify the deleted blocks and save the ones to restore in a file named `deleted-block-list` (one block per line).

Step 2: Once you have the `deleted-block-list`, you can now list all the objects you need to restore, because only objects can be restored and not prefixes:

```
while read block; do
gsutil ls -a -r $block | grep "#" | grep -v deletion-mark.json | grep -v index.cache.json
done < deleted-list > full-deleted-file-list
```

The above script will ignore the `deletion-mark.json` and `index.cache.json` which shouldn't be restored.

Step 3: Run the following script to restore the objects:

```
while read file; do
gsutil cp $file ${file%#*}
done < full-deleted-list
```

## Log lines

### Log line containing 'sample with repeated timestamp but different value'

This means a sample with the same timestamp as the latest one was received with a different value. The number of occurrences is recorded in the `cortex_discarded_samples_total` metric with the label `reason="new-value-for-timestamp"`.

Possible reasons for this are:

- Incorrect relabelling rules can cause a label to be dropped from a series so that multiple series have the same labels. If these series were collected from the same target they will have the same timestamp.
- The exporter being scraped sets the same timestamp on every scrape. Note that exporters should generally not set timestamps.
