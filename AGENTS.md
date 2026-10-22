# AGENTS.md

## Project Overview

`tpu-provisioner` is a Kubernetes controller that manages the lifecycle of Google Kubernetes Engine (GKE) TPU node pools:

- **Dynamic Provisioning**: Watches pending leader Pods (for `JobSet` and `LeaderWorkerSet` workloads) and `Slice` custom resources (`slice.accelerator.gke.io/v1beta1`) to dynamically create and delete TPU node pools on demand.
- **Static Provisioning**: Watches the `tpu-provisioner-static-nodepools-config` `ConfigMap` (`StaticNodepoolReconciler`) to pre-provision, update, and clean up static TPU node pools backed by GCE reservations and subblocks.
- **Admission Webhooks**: Mutating webhooks for `Job` (`/mutate`) and `LeaderWorkerSet` `StatefulSet` (`/mutate-lws`) resources to inject TPU node selectors and scheduling metadata.

---

## Briefs Convention (`briefs/`)

Every pull request, feature, or bug fix must include a corresponding KEP-style numbered markdown file in [`briefs/`](./briefs/) named `briefs/NNNN-<short-title>.md` (see [`briefs/0001-sync-static-nodepools.md`](./briefs/0001-sync-static-nodepools.md) as the reference example):

1. **Naming**: Zero-padded 4-digit sequential prefix followed by a kebab-case slug (`NNNN-<short-title>.md`).
2. **Purpose**: Provide a concise, user-facing summary of the change that can be pasted directly into a PR description, capturing the essence of the user prompts cohesively.
3. **Structure**: Keep it well under one page with two short sections—`## Problem` (why the change is needed and what was broken or limiting before) and `## Solution` (3–5 short bullet points describing how it works now).
4. **Plain Language**: Use simple, everyday words (for example, "pair" instead of "sequentially chain") and focus on user-visible behavior rather than file-by-file code diffs or internal implementation minutiae.

---

## Architecture & Code Map

- `cmd/`: Controller entrypoint (`cmd/main.go`) and environment variable configuration (`cmd/config/`).
- `internal/cloud/`: GKE API client wrapper (`gke.go`, `gke_service.go`, `gke_context.go`), node pool specification builders, and static node pool diff/sync logic (`DiffStaticNodePools`, `SyncStaticNodePools`).
- `internal/controller/`: Reconcilers for dynamic Pod/JobSet creation & deletion (`creation_controller.go`, `deletion_controller.go`), Slice provisioning (`jobset_slice_controller.go`, `lws_slice_controller.go`), static node pools (`static_nodepool_controller.go`), and periodic garbage collection (`nodepool_garbage_collector.go`).
- `internal/webhook/`: Mutating webhook handlers for Jobs and LeaderWorkerSets.
- `internal/utils/`: Helpers for metadata, slice naming, and static node pool naming.
- `test/integration/controller/`: Envtest + Ginkgo integration test suites using a mock GKE provider (`mock_provider.go`).
- `briefs/`: Max-one-page user-facing summaries of PRs, features, and bug fixes.

---

## Quick Reference Commands

```bash
# Run unit tests (includes manifests, go fmt, go vet)
make test-unit

# Run integration tests in parallel via Ginkgo + envtest
make test-integration-parallel

# Run a specific unit test with race detector
go test -race -v ./internal/cloud/... -run TestSyncStaticNodePools

# Build manager binary
make build
```
