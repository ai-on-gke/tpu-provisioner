#!/bin/bash
#
# This script creates a 'HIGH_THROUGHPUT' workload resource policy
# for each TPU topology defined in the `supported_topologies` array.

# --- Configuration ---
# Please set the PROJECT_ID and REGION environment variables before running.
# export PROJECT_ID="your-gcp-project"
# export REGION="your-gcp-region"

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

supported_topologies=(
    '12x12x12'
    '12x12x16'
    '12x12x20'
    '12x12x24'
    '12x12x28'
    '12x12x36'
    '12x16x16'
    '12x16x20'
    '12x16x24'
    '12x16x28'
    '12x20x20'
    '12x20x24'
    '12x24x24'
    '16x16x16'
    '16x16x20'
    '16x16x24'
    '16x16x32'
    '16x20x28'
    '16x24x24'
    '2x2x1'
    '2x2x2'
    '2x2x4'
    '2x4x4'
    '4x12x116'
    '4x12x12'
    '4x12x124'
    '4x12x20'
    '4x12x28'
    '4x12x44'
    '4x12x52'
    '4x12x68'
    '4x12x76'
    '4x12x92'
    '4x20x20'
    '4x20x28'
    '4x20x44'
    '4x20x52'
    '4x20x68'
    '4x20x76'
    '4x28x28'
    '4x28x44'
    '4x28x52'
    '4x4x116'
    '4x4x12'
    '4x4x124'
    '4x4x148'
    '4x4x164'
    '4x4x172'
    '4x4x188'
    '4x4x20'
    '4x4x212'
    '4x4x236'
    '4x4x244'
    '4x4x28'
    '4x4x4'
    '4x4x44'
    '4x4x52'
    '4x4x68'
    '4x4x76'
    '4x4x8'
    '4x4x92'
    '4x8x116'
    '4x8x12'
    '4x8x124'
    '4x8x148'
    '4x8x164'
    '4x8x172'
    '4x8x188'
    '4x8x20'
    '4x8x28'
    '4x8x44'
    '4x8x52'
    '4x8x68'
    '4x8x76'
    '4x8x8'
    '4x8x92'
    '8x12x12'
    '8x12x16'
    '8x12x20'
    '8x12x28'
    '8x12x44'
    '8x12x52'
    '8x16x16'
    '8x16x20'
    '8x16x28'
    '8x16x44'
    '8x20x20'
    '8x20x28'
    '8x8x12'
    '8x8x16'
    '8x8x20'
    '8x8x28'
    '8x8x44'
    '8x8x52'
    '8x8x68'
    '8x8x76'
    '8x8x8'
    '8x8x92'
)

for topology in "${supported_topologies[@]}"; do
  # Create a name for the policy by replacing 'x' with '-'
  # e.g., '12x12x12' becomes '12-12-12'
  shape_name="${topology//x/-}"
  workload_policy_name="tpu-provisioner-${shape_name}"

  echo "Creating resource policy '${workload_policy_name}' for topology '${topology}'..."

  gcloud compute resource-policies create workload-policy "${workload_policy_name}" \
    --type=HIGH_THROUGHPUT \
    --accelerator-topology="${topology}" \
    --project="${PROJECT_ID}" \
    --region="${REGION}"

  echo "Successfully created policy '${workload_policy_name}'."
  echo "---"
  sleep 1
done

echo "All resource policies created."