# TPU Provisioner

TPU Provisioner is a custom k8s controller which dynamically provisions TPU slices for [JobSets](https://jobset.sigs.k8s.io) based on the workload requirements, and manages the lifecycle of those slices.

## Description

The provisioning process starts with an unschedulable "leader" pod (pod with Job completion index 0) for each Job in the
JobSet. Once the TPU slice is created, the remaining pods for each Job will be created and follow their leader pod
onto the same slice it is running on.

Node Pools are cleaned up when the JobSet whose pods triggered the node pool creation is either **completed, failed, or deleted**.

## Setup

### Export the Environment Variables
```bash
GCP_PROJECT_ID=your-project \
GCP_CLUSTER_LOCATION=your-cluster-region \
GCP_ZONE=your-tpu-zone \
GCP_CLUSTER=your-cluster \
GCP_NODE_SERVICE_ACCOUNT=YOUR_PROJECT_NUMBER-compute@developer.gserviceaccount.com
```

### Create a GKE Cluster with workload identity enabled and no release channel

The TPU Provisioner requires [Workload Identity for GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) to be enabled, and cannot be on a release channel (auto upgrades
are disabled on node pools created by the TPU provisioner, to minimize disruptions to training workloads).

Refer to the [public docs](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) and follow
the steps to create a cluster with workload identity enabled.

You should also ensure your cluster is not enrolled in a release channel. The easiest way to do this is in the Google
Cloud Console UI. Search "Kubernetes Engine" in the search bar and select `Kubernetes Engine` from the dropdown,
then click on your cluster to pull up settings that can be configured. Find the `Release Channel` setting, click the
edit button and select `No channel`.

Also note, if you plan to [preload container images via secondary boot disks](https://cloud.google.com/kubernetes-engine/docs/how-to/data-container-image-preloading#create-cluster-secondary-disk) to reduce pod startup latency, you'll
need to set the `ENABLE_IMAGE_STREAMING` configuration to `true` in the TPU Provisioner environment. If `ENABLE_IMAGE_STREAMING` is `false`, any provided secondary boot disk configuration will be ignored and a warning will be logged. Setting `ENABLE_IMAGE_STREAMING` to `true` also enables `GcfsConfig` on the provisioned GKE node pools, while setting it to `false` explicitly disables it to override cluster-wide defaults.

### Install JobSet

TPU Provisioner dynamically provisions TPU slices for [JobSets](https://jobset.sigs.k8s.io) based on the workload
requirements. 

JobSet is a k8s native API
for running distributed ML training workloads, and is the recommended solution for TPU Multislice training. However, it
is generic and can be used for any arbitrary batch workload as well (GPUs, CPUs, etc). 

Follow the [installation steps](https://jobset.sigs.k8s.io/docs/installation/) to install the latest release of JobSet
in your cluster.

### Permissions

Create TPU Provisioner Service Account, which will be the IAM service account used by the
k8s service account `tpu-provisioner-controller-manageer` to authenticate with Workload Identity.

```sh
gcloud iam service-accounts create tpu-provisioner
export PROVISIONER_SERVICE_ACCOUNT=tpu-provisioner@${GCP_PROJECT_ID}.iam.gserviceaccount.com
```

Give the Service Accounts permissions to administer GKE clusters.

```bash
gcloud projects add-iam-policy-binding $GCP_PROJECT_ID --member="serviceAccount:${PROVISIONER_SERVICE_ACCOUNT}" --role='roles/container.clusterAdmin'
```

Bind the GCP Service Account to the Kubernetes Service Account that will be attached to the controller Pod.

```sh
gcloud iam service-accounts add-iam-policy-binding ${PROVISIONER_SERVICE_ACCOUNT} \
    --role roles/iam.workloadIdentityUser \
    --member "serviceAccount:${GCP_PROJECT_ID}.svc.id.goog[tpu-provisioner-system/tpu-provisioner-controller-manager]"
```

The tpu-provisioner service account will also need `iam.serviceAccountUser` on the service account to be used by the nodes in the nodepool:

```sh

gcloud iam service-accounts add-iam-policy-binding ${NODE_SERVICE_ACCOUNT} \
    --member="serviceAccount:${PROVISIONER_SERVICE_ACCOUNT}" \
    --role="roles/iam.serviceAccountUser" \
    --project=${PROJECT_ID}
```

### Deployment directory setup

TPU Provisioner deployment configurations are defined on a per cluster level, using config files which live in
a directory structure like follows:

`${REPO_ROOT}/deploy/${GCP_PROJECT_ID}/${GCP_CLUSTER}`

You will need to create the `deploy/${GCP_PROJECT_ID}/${GCP_CLUSTER}` directory for each cluster you deploy
the provisioner on.

Next, copy the files from `deploy/example-project/example-cluster-v5p` for `v5p`tpu type or `deploy/example-project/example-cluster-v7x` for `v7x` tpu type into your new `deploy/${PROJECT_ID}/${CLUSTER_NAME}` directory.

For `v6e` use the same Templates for `v5p`

Update the templated values in the .yaml files to match your own.

### Building and Deploying the Controller

Build and push your image:

```bash
export CONTAINER_IMAGE=us-docker.pkg.dev/${GCP_PROJECT_ID}/default/tpu-provisioner:$(git rev-parse --short HEAD)
make docker-build docker-push IMG=${CONTAINER_IMAGE}
```

Note: for multi-platform builds or when building on a platform that does not match the target architecture, the easiest method is to use Cloud Build.
This command will automatically build and push the image to artifact registry:

```bash
gcloud builds submit --tag $CONTAINER_IMAGE --project=$PROJECT_ID .
```

Set the container image in the manifests.

```bash
cd ./deploy/${GCP_PROJECT_ID}/${GCP_CLUSTER}
kustomize edit set image controller=${CONTAINER_IMAGE}
cd -
```

Edit the settings in the `./deploy/${GCP_PROJECT_ID}/${GCP_CLUSTER}/` directory to match your project (ConfigMap values and ServiceAccount annotation).

Deploy controller.

```sh
kubectl apply --server-side -k ./deploy/${GCP_PROJECT_ID}/${GCP_CLUSTER}
```


## Run an example

After deploying the TPU provisioner on your cluster following the steps above, you can run an example workload to
test that the configurations are set up correctly.

There are 2 things to keep in mind here:

1. You need sufficient quota for whatever TPU machine type you intend to run your workload on.
2. TPU Provisioner operates on [JobSets](https://jobset.sigs.k8s.io) so you'll need to deploy your workload as a JobSet.
See these [JobSet examples](https://jobset.sigs.k8s.io/docs/tasks/) to get started.

This repo includes a simple distributed Jax workload on TPU v4 machines which can be used to verify
your setup is correct.

To apply it, simply run: `k apply -f examples/jobset.yaml` (note: you can tweak JobSet configuration
to define the TPU machine type, number of TPU slices, and their topology).

Next, run `kubectl get pods` to ensure pods have been created - you should see some pending pods.

These pending pods should trigger node pool creation requests for TPU v4 slices of 2x2x2 topology.

Within a few minutes, the node pool creation operations should complete and you should see the pods
transition from `Pending` to `Ready`. In the container logs, you should see the total TPU device count.

## Development

This project is written in Go and uses the [Kubebuilder](https://book.kubebuilder.io/) tool.

For local development and quick manual testing, you can do the following:

Note you’ll need a Kubernetes cluster to run against.

Impersonate the Service Account created above:

```bash
# Assuming you have GCP_PROJECT_ID set in your environment...
gcloud config set auth/impersonate_service_account ${PROVISIONER_SERVICE_ACCOUNT}
```

Run the controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```bash
make run
```

**Note:** When using `make run`, your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

Test that you can apply a TPU Job.

```bash
kubectl apply -f ./examples/ironwood-jobset-32.yaml/
```

### Ironwood / tpu7x support

In order to support ironwood we need to have Workload Policy's attached to nodepool creation step, in order to work with the tpu-provisioner you will need Workload Policy resources in the project and region with the following syntax: `tpu-provisioner-$TPU_TOPOLOGY`, e.g. `tpu-provisioner-2x2x2` or `tpu-provisioner-8x8x16`. See the script in [./docs/ironwood-policy-bootstrap.sh](./docs/ironwood-policy-bootstrap.sh), which would need to run ahead of time in the project for each region where ironwood capacity is landing.

### Static Nodepool Provisioner

In addition to dynamic nodepool creation, the TPU provisioner also supports pre-provisioning nodepools based on a static configuration. This functionality is designed to be used with gSC reservations and superslicing.

The static nodepool provisioner is configured via a `ConfigMap` in the same namespace as the provisioner. Note that the name needs to be set to `tpu-provisioner-static-nodepools-config` because the name is used to filter the objects returned by the Kubernetes API. The provisioner will watch for changes to this `ConfigMap` and create or update nodepools accordingly.

Here is an example of the `tpu-provisioner-static-nodepools-config` `ConfigMap`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tpu-provisioner-static-nodepools-config
data:
  reservations: |
    - name: "test-reservation"
      gscBlocks:
        - name: "test-reservation-block-0001"
          subblocks: "0001-0002" # Can be a range, e.g. 0001-0002, or a single subblock, e.g. 0001
          nodepoolPrefix: "my-static-nodepool" # Optional
  nodepoolConfig: |
    machineType: "tpu7x-standard-4t"
    accelerator: "tpu7x"
    topology: "4x4x4"
    nodeCount: 16
    nodeLabels:
      label-key: "label-value"
    shieldedIntegrityMonitoring: true
    maxPodsPerNode: 8
    enableAutorepair: true
    placementPolicy: "tpu-provisioner-4x4x4"
```

#### Configuration Parameters

The `ConfigMap` has two main keys: `reservations` and `nodepoolConfig`.

##### `reservations`

This key contains a list of TPU reservations. Each reservation has a `name` and a list of `gscBlocks`. Each `gscBlock` has a `name`, the `subblocks` to provision within that block, and an optional `nodepoolPrefix`. If provided, it will be used as the prefix for the nodepool name. If not provided, the nodepool name will be equal to the subblock name, which is derived from the reservation and block names, as well as the subblock index.

##### `nodepoolConfig`

This key contains the configuration for the nodepools that will be created. The following parameters are supported:

*   `machineType`: The GCE machine type for the nodes.
*   `accelerator`: The type of TPU accelerator.
*   `topology`: The TPU topology.
*   `nodeCount`: The number of nodes in the nodepool.
*   `nodeLabels`: A map of key-value pairs to set as labels on the nodes.
*   `shieldedIntegrityMonitoring`: (Optional) `true` or `false` to enable/disable shielded integrity monitoring. Defaults to `nil` (GKE default).
*   `shieldedSecureBoot`: (Optional) `true` or `false` to enable/disable shielded secure boot. Defaults to `nil` (GKE default).
*   `maxPodsPerNode`: (Optional) The maximum number of pods that can run on a node. For static nodepools, this takes precedence over the global `GKE_MAX_PODS_PER_NODE` environment variable.
*   `enableAutorepair`: (Optional) `true` or `false` to enable/disable node auto-repair. Defaults to `nil` (GKE default).
*   `placementPolicy`: (Optional) The placement policy for the nodes (e.g., `COMPACT` or `tpu-provisioner-4x4x4`).

##### Global Defaults (Environment Variables)

Some configuration parameters are set via environment variables for the provisioner itself. These provide the default values for nodepools managed by the provisioner:

*   `CONCURRENCY`: (Optional) The maximum number of concurrent reconcile operations for dynamic provisioning. Defaults to `3`.
*   `BACKOFF_BASE_DELAY`: (Optional) The base delay for exponential backoff on retriable errors. Defaults to `5s`.
*   `BACKOFF_MAX_DELAY`: (Optional) The maximum delay for exponential backoff on retriable errors. Defaults to `5m`.
*   `GKE_MAX_PODS_PER_NODE`: (Optional) The maximum number of pods that can run on a node. Defaults to `15`. 
    *   For **dynamic nodepools**, this is used for all provisioned node pools.
    *   For **static nodepools**, this is the default value if `maxPodsPerNode` is not specified in the 
    `tpu-provisioner-static-nodepools-config` `ConfigMap`.
    
    In GKE, the system default is 110. For large clusters, using the default of 110 can result in quickly exceeding the available IP space in the cluster's pod IP range. Setting a lower value like the `tpu-provisioner` default of 15 is recommended for TPU-intensive workloads where each node typically only runs a single large pod.

*   `ENABLE_IMAGE_STREAMING`: (Optional) Whether to enable GKE Image Streaming (`GcfsConfig`) on dynamic and static node pools. Defaults to `false`.
    *   If `true`, `GcfsConfig.Enabled` is set to `true` on the node pools, and any configured `GCP_NODE_SECONDARY_DISK` (secondary boot disk) is populated.
    *   If `false`, `GcfsConfig.Enabled` is explicitly set to `false` and `GCP_NODE_SECONDARY_DISK` if present is ignored (with a logged warning).

Some configuration parameters come from environment variables rather than the configmap, particularly those that are shared across both statically and dynamically created nodepools managed by the provisioner. This includes the following environment variables typically set in the manager configmap (note that the `STATIC_NODEPOOL_CREATE_CONCURRENCY` environment variable is distinct from the `CONCURRENCY` environment variable to allow for separate nodepool create operation limits between static and dynamic nodepools):

```bash
STATIC_NODEPOOL_CREATE_CONCURRENCY: "3"
BACKOFF_BASE_DELAY: "5s"
BACKOFF_MAX_DELAY: "5m"
GCP_PROJECT_ID: my-project
GCP_CLUSTER_LOCATION: us-central1
GCP_ZONE: us-central1-c
GCP_CLUSTER: test-cluster
GCP_NODE_ADDITIONAL_NETWORKS: test-network:test-subnet
GCP_NODE_TAGS: test-tag
GCP_NODE_SERVICE_ACCOUNT: my-service-account-email
GKE_MAX_PODS_PER_NODE: "15"
ENABLE_IMAGE_STREAMING: "false"
```

Nodepools created by the static provisioner are labeled with `tpu-provisioner-static-nodepool` in order to ensure that their lifecycle is managed independently of dynamic nodepools. Nodepools with this label are omitted from the standard garbage collection loop used for dynamically-provisioned nodepools, and are instead cleaned up when their corresponding subblock, block, or reservation specifications are removed from the configmap.
