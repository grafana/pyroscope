---
title: "Grafana Mimir listblocks"
menuTitle: "Listblocks"
description: "Listblocks show the block details of a tenant."
weight: 10
---

# Grafana Mimir listblocks

The listblocks tool lists blocks and show the block details of a tenant.
Listblocks requires at least configuration to access the bucket and tenant.

Listblocks doesn't use the bucket index; instead, it downloads the `meta.json` file of every block in the tenant.
This means that listblocks has an up-to-date view of the blocks in the bucket.

```
$ ./listblocks -backend=gcs -gcs.bucket-name=bucket-with-blocks -user=10428
Block ID                     Min Time               Max Time               Duration
01E0HMK47RGAAKZJBMG8B8QXGP   2020-02-07T07:49:46Z   2020-02-08T00:00:00Z   16h10m13.89s
01E0M0VK2KEDZC5AK1PX8K00EX   2020-02-08T00:00:00Z   2020-02-09T00:00:00Z   24h0m0s
01E0PK9B84XJ9KQ0DHZDQECNH6   2020-02-09T00:00:00Z   2020-02-10T00:00:00Z   24h0m0s
01E0S8VAKJ0H41N41GBKQN4G1N   2020-02-10T00:00:00Z   2020-02-11T00:00:00Z   24h0m0s
01E0VTN88859KW1KTDVBS14E7A   2020-02-11T00:00:00Z   2020-02-12T00:00:00Z   24h0m0s
01E0YCZKFG2ME5GZ60AYCQ39V4   2020-02-12T00:00:00Z   2020-02-13T00:00:00Z   24h0m0s
01E111CX17BXFZD97AKSYKX0A5   2020-02-13T00:00:00Z   2020-02-14T00:00:00Z   24h0m0s
01E13JCZK9A5SJMAY6QSSEB0XX   2020-02-14T00:00:00Z   2020-02-15T00:00:00Z   24h0m0s
01E164EJFPT8ZCY6QWEKNJ0VYX   2020-02-15T00:00:00Z   2020-02-16T00:00:00Z   24h0m0s
...
```

Listblocks has many options you can use to modify the output. The following list contains the most important listblocks options:

```
  -max-time value
    	If set, only blocks with MaxTime <= this value is printed
  -min-time value
    	If set, only blocks with MinTime >= this value is printed
  -show-block-size
    	Show size of block based on details in meta.json, if available
  -show-compaction-level
    	Show compaction level
  -show-deleted
    	Show deleted blocks
  -show-labels
    	Show block labels
  -show-parents
    	Show parent blocks
  -show-sources
    	Show compaction sources
  -show-ulid-time
    	Show time from ULID
```

## Example

```
$ ./listblocks -backend=gcs -gcs.bucket-name=bucket-with-blocks -user=10428 -min-time=2022-02-01T00:00:00Z -max-time=2022-02-04T00:00:00Z -show-labels -show-block-size
Block ID                     Min Time               Max Time               Duration   Size     Labels (excl. __org_id__)
01FTWJ3V2TP7N4D7FCSSBJXQ9Z   2022-02-01T00:00:00Z   2022-02-02T00:00:00Z   24h0m0s    69 GiB   {__compactor_shard_id__="1_of_4"}
01FTWJZ3FD4QX4T1FMJJNP7XR1   2022-02-01T00:00:00Z   2022-02-02T00:00:00Z   24h0m0s    69 GiB   {__compactor_shard_id__="2_of_4"}
01FTWMN7AQBPMXWBHVC61ENPT7   2022-02-01T00:00:00Z   2022-02-02T00:00:00Z   24h0m0s    69 GiB   {__compactor_shard_id__="3_of_4"}
01FTWQ5Y87AWVKXH44T2N23BHW   2022-02-01T00:00:00Z   2022-02-02T00:00:00Z   24h0m0s    69 GiB   {__compactor_shard_id__="4_of_4"}
01FTZ4QWE2PNK69ZJGTK2NCWFB   2022-02-02T00:00:00Z   2022-02-03T00:00:00Z   24h0m0s    73 GiB   {__compactor_shard_id__="1_of_4"}
01FTZ55XAZCVHWP9K5AAR5BVHF   2022-02-02T00:00:00Z   2022-02-03T00:00:00Z   24h0m0s    73 GiB   {__compactor_shard_id__="2_of_4"}
01FTZ7AQBCSBB8T6P2Q5QZ416W   2022-02-02T00:00:00Z   2022-02-03T00:00:00Z   24h0m0s    73 GiB   {__compactor_shard_id__="3_of_4"}
01FTYW42TNTZ44QMM9YTFDE6Y4   2022-02-02T00:00:00Z   2022-02-03T00:00:00Z   24h0m0s    73 GiB   {__compactor_shard_id__="4_of_4"}
01FV1S5GQDAFTQ4M9CTN1CD1E4   2022-02-03T00:00:00Z   2022-02-04T00:00:00Z   24h0m0s    77 GiB   {__compactor_shard_id__="1_of_4"}
01FV1JKPH2VFXA4K6XNETC8FBR   2022-02-03T00:00:00Z   2022-02-04T00:00:00Z   24h0m0s    77 GiB   {__compactor_shard_id__="2_of_4"}
01FV1VQQTAJVA287ZY8DC435HD   2022-02-03T00:00:00Z   2022-02-04T00:00:00Z   24h0m0s    77 GiB   {__compactor_shard_id__="3_of_4"}
01FV1FRX39NC1J64D6H6W9VVZ9   2022-02-03T00:00:00Z   2022-02-04T00:00:00Z   24h0m0s    77 GiB   {__compactor_shard_id__="4_of_4"}
```
