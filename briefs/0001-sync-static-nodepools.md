# 0001: Parallel Static Node Pool Deletions and Creations

## Problem

When reconciling static node pools, the provisioner previously ran all deletions first and stopped if any deletion failed. Because of this:

- A single failed deletion blocked all new static node pools from being created.
- Unrelated creations had to wait for all deletions to finish before starting.

We want deletions and creations to run in parallel without getting stuck on a single failure, while still making sure we never try to create a node pool for a reservation subblock before the old node pool using that subblock is deleted.

## Solution

Replace the separate delete and create steps with a single `SyncStaticNodePools` call:

- **Run in parallel**: Deletions and creations run at the same time using their own concurrency limits (`STATIC_NODEPOOL_DELETE_CONCURRENCY` and `STATIC_NODEPOOL_CREATE_CONCURRENCY`).
- **Pair by subblock**: If a new node pool uses the same reservation subblock as a node pool being deleted, the two operations are paired so the create runs right after the delete finishes.
- **Isolate failures**: If a delete fails, only its paired create (for that same subblock) is skipped. All other deletes and creates continue.
- **Timeout on create only**: A paired create only takes a create concurrency slot and starts its `STATIC_NODEPOOL_CREATE_TIMEOUT` timer once its delete succeeds.
