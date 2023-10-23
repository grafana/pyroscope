# Split and Merge Compactor

## How the grouper works

Given the list of all blocks in the storage:

- Group blocks by resolution + external labels (excluding shard ID). For each group:
  - Group blocks by compactable time ranges (eg. 2h, 6h, 12h, 24h)
    - Given the time range, if the time range is the "smallest one" (eg. 2h) AND there are non-sharded blocks, then we should split the block
      - Example: TR=2h
        - Case 1: all the blocks for the 2h time range have the "shard ID" label in the meta.json.
          In this case, we shouldn't run the split stage anymore.
        - Case 2: there is at least 1 block for the 2h time range NOT having "shard ID" in the meta.json.
          In this case, we should run the split stage on 2h blocks without the "shard ID".
      - Horizontal sharding
        - Each compactor will take decisions without a central coordination
        - Each compactor will run the planning for ALL blocks
        - Each compactor will only execute "jobs" belonging to its shard
        - Splitting job (each job consists of a set of blocks which will be merged together calling CompactWithSplitting())
          - Which blocks should belong to a specific job?
            - We don't want all blocks to be compacted together, otherwise doesn't scale
            - We use the configured number of output shards to determine how many concurrent jobs we want to run to split/shard blocks
            - For each block to split, we add it to the job with index `hash(blockID) % numShards`
            - Output: up to `numShards` jobs (each job contain the set of blocks to be merged together when running CompactWithSplitting())
          - How can we know if the compactor instance should process a job or not?
            - A job is owned if `hash(tenant + stage + time range + shard ID)` belongs to the compactor tokens
      - If the previous check has not produced any job AT ALL it means all the blocks for the "smallest time range" are already split
        (or there are no blocks at all), so we can proceed with the merging stage:
        - Group blocks by "shard ID" (taking in account the case a block doesn't have the shard ID)
        - Create a job for each group that contains 2+ blocks (because they need to be merged together)
        - Add the job to the list of "valid jobs to execute" only if for the job shard ID there are no other
          jobs already in the list of "valid jobs" overlapping its time range
- Execute the jobs that belong to the compactor instance
  - Loop through jobs and filter out the jobs hashes not belonging to the compactor tokens
