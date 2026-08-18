# Release Notes

## Version 1.3.0

- **Configurable Static Nodepool Error Recreation**: Added `lifecycle.recreateOnError` under `nodepoolConfig` in the static nodepools ConfigMap to allow opting out of automatic deletion and recreation of nodepools in an `ERROR` state (defaults to `true`).


## Version 1.2.0
- **Otel Collector Sidecar**: Add in otel collector sidecar to scrape tpu-provisioner controller-runtime metrics. Collector adds a "tpu_provisioner" prefix.


## Version 1.1.0

- **Configurable Exponential Backoff**: Implement configurable exponential backoff via `BACKOFF_BASE_DELAY` and `BACKOFF_MAX_DELAY`.
- **GKE Image Streaming Configuration**: Add GKE Image Streaming (`ENABLE_IMAGE_STREAMING`) config. Defaults to false. Ignores `SecondaryBootDisks` unless `ENABLE_IMAGE_STREAMING` is true.
- **Parallel Integration Testing**: Add `test-integration-parallel` target, significantly reducing integration test time.


## Version 1.0.0

- **TPU7x (Ironwood) Support**: Introduced support for TPU7x, including:
  - Static nodepool provisioning (conditional based on `enableSliceController`).
  - Superslicing capabilities.
- **JobSet Upgrade**: Upgraded to v0.11.1 and updated RBAC to support PATCH operations.
