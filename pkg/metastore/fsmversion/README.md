# FSM versioning

The FSM version is a replicated integer that tells every metastore replica how to apply raft log entries. It lets a change to apply-time behavior ship in a binary but stay dormant until every replica runs that binary, so replicas never apply the same entry differently during a rollout.

## How it works

- Each binary supports FSM versions up to `fsmversion.Latest` and reports it in `NodeInfo.supported_fsm_version`.
- The active version is stored in the `fsm_version` bbolt bucket. It is part of the FSM state, so snapshots carry it and log replay reproduces it at the same index.
- The raft leader runs the activator every `-metastore.fsm-version.check-interval`:
  1. It asks every discovered metastore instance for its `NodeInfo`. Every server in the raft configuration, voters and non-voters alike, must answer; otherwise nothing happens.
  2. The target is the lowest supported version, capped by `-metastore.fsm-version.max-version`.
  3. If `-metastore.fsm-version.activation-delay` is set, the same target must be observed for that long.
  4. It checks that the raft configuration and term did not change during the check, and proposes `RAFT_COMMAND_SET_FSM_VERSION` with the current term.
- Applying `SET_FSM_VERSION`:
  - A proposal from a previous term is ignored.
  - A version at or below the active one is ignored. The version never goes down.
  - A version above what the binary supports fails the command, which stops the replica.
- A state or snapshot whose version the binary does not support is refused before it replaces the current state. A replica with an older binary does not start, and does not accept snapshots from the leader.
- An unknown command type stops the replica instead of being skipped.

## Changing how an entry is applied

1. Add a level to `version.go` and point `Latest` at it. Never renumber or reuse a level once it is merged: development environments run `main`.
2. Branch at apply time on the replicated version, never on local configuration:

   ```go
   if fsmversion.IsActive(tx, fsmversion.MyChange) {
       // new behavior
   } else {
       // old behavior
   }
   ```

   Keep the old branch. The log and snapshots can hold entries proposed before the activation.
3. New command types, or new fields that change how an entry is applied, must only be proposed once the leader observes `State.Active() >= MyChange`. Older replicas silently ignore unknown fields.
4. Remove the old branch only once upgrades from binaries that predate the level are no longer supported.

## Rollouts and rollbacks

- During a rollout nothing changes until every replica reports support for the new version.
- Before activation, rolling back is safe.
- After activation, a binary that does not support the active version refuses to start with `unsupported FSM version`. Roll forward instead. To keep a rollback window, set `-metastore.fsm-version.activation-delay`, or pin the version with `-metastore.fsm-version.max-version`.
- Binaries that predate FSM versioning report no version, so the leader does not activate anything while one of them is a raft member. They also ignore `SET_FSM_VERSION`, so they must not rejoin the cluster after a version was activated.
