#!/bin/bash
#
# This script creates a 'HIGH_THROUGHPUT' workload resource policy
# for each TPU topology defined in the external file specified by TOPOLOGY_FILE.
#
# --- Configuration ---
# Set the following environment variables before running:
# export PROJECT_ID="your-gcp-project"
# export REGION="your-gcp-region"
# export TOPOLOGY_FILE="topology-ref-tpu7x.txt"
# export WORKLOAD_POLICY_NAME_PREFIX="tpu-provisioner-" (Optional, defaults to tpu-provisioner-)

# Exit if any command fails
set -e

if [[ -z "${PROJECT_ID}" ]]; then
  echo "Error: PROJECT_ID environment variable is not set."
  exit 1
fi

if [[ -z "${REGION}" ]]; then
  echo "Error: REGION environment variable is not set."
  exit 1
fi

if [[ -z "${TOPOLOGY_FILE}" ]]; then
  echo "Error: TOPOLOGY_FILE environment variable is not set."
  exit 1
fi

if [[ ! -f "${TOPOLOGY_FILE}" ]]; then
  echo "Error: Topology file '${TOPOLOGY_FILE}' does not exist."
  exit 1
fi

# Set default prefix if not provided
WORKLOAD_POLICY_NAME_PREFIX="${WORKLOAD_POLICY_NAME_PREFIX:-tpu-provisioner-}"

echo "--- Configuration Parameters ---"
echo "PROJECT_ID: ${PROJECT_ID}"
echo "REGION: ${REGION}"
echo "TOPOLOGY_FILE: ${TOPOLOGY_FILE}"
echo "WORKLOAD_POLICY_NAME_PREFIX: ${WORKLOAD_POLICY_NAME_PREFIX}"
echo "--------------------------------"

echo "Fetching existing resource policies matching prefix '${WORKLOAD_POLICY_NAME_PREFIX}'..."
existing_policies=$(gcloud compute resource-policies list --filter="region:${REGION} AND name~^${WORKLOAD_POLICY_NAME_PREFIX}" --format="value(name)" --project="${PROJECT_ID}")

while IFS= read -r topology || [[ -n "${topology}" ]]; do
  # Skip empty lines and comments
  [[ -z "${topology}" ]] && continue
  [[ "${topology}" =~ ^#.*$ ]] && continue

  # creates a workload policy for each topology
  workload_policy_name="${WORKLOAD_POLICY_NAME_PREFIX}${topology}"

  # Check if policy already exists
  if echo "${existing_policies}" | grep -Fxq "${workload_policy_name}"; then
    echo "Resource policy '${workload_policy_name}' already exists. Skipping."
  else
    echo "Processing resource policy '${workload_policy_name}' for topology '${topology}'..."
    echo "Creating resource policy '${workload_policy_name}'..."
    gcloud compute resource-policies create workload-policy "${workload_policy_name}" \
      --type=HIGH_THROUGHPUT \
      --accelerator-topology="${topology}" \
      --project="${PROJECT_ID}" \
      --region="${REGION}"
    echo "Successfully created policy '${workload_policy_name}'."
    echo "---"
    sleep 1
  fi
done < "${TOPOLOGY_FILE}"

echo "All resource policies processed."
