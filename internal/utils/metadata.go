package utils

import (
	corev1 "k8s.io/api/core/v1"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

// When this pod label is set to "true", the TPU provisioner will not reconcile the pod.
const (
	DisableAutoProvisioningLabel = "tpu-provisioner.cloud.google.com/disable-autoprovisioning"
	SliceProvisioningLabel       = "tpu-provisioner.cloud.google.com/slice-autoprovisioning"

	SliceProvisioningModeAsync = "async"
	SliceProvisioningModeSync  = "sync"
)

// AutoProvisioningDisabled returns true if the pod has
// "tpu-provisioner.cloud.google.com/disable-autoprovisioning=true"
// set as a label or annotation. Otherwise, it returns false.
func AutoProvisioningDisabled(pod *corev1.Pod) bool {
	return pod.Labels[DisableAutoProvisioningLabel] == "true" || pod.Annotations[DisableAutoProvisioningLabel] == "true"
}

// AutoProvisioningDisabledForJobSet returns true if the JobSet or Pod spec has
// "tpu-provisioner.cloud.google.com/disable-autoprovisioning=true"
// set as a label or annotation. Otherwise, it returns false.
func AutoProvisioningDisabledForJobSet(js *jobset.JobSet) bool {
	if js.Labels[DisableAutoProvisioningLabel] == "true" || js.Annotations[DisableAutoProvisioningLabel] == "true" {
		return true
	}
	// Historically, auto provisioning was disabled via the Pod metadata. Keep the same logic.
	for _, rj := range js.Spec.ReplicatedJobs {
		if podLabels := rj.Template.Spec.Template.Labels; podLabels != nil && podLabels[DisableAutoProvisioningLabel] == "true" {
			return true
		}
		if podAnn := rj.Template.Spec.Template.Annotations; podAnn != nil && podAnn[DisableAutoProvisioningLabel] == "true" {
			return true
		}
	}
	return false
}

func SliceProvisioningEnabled(js *jobset.JobSet) bool {
	return js.Labels != nil &&
		(js.Labels[SliceProvisioningLabel] == SliceProvisioningModeAsync ||
			js.Labels[SliceProvisioningLabel] == SliceProvisioningModeSync)
}

// getProvisioningMode returns the provisioning mode from the JobSet annotation.
// Returns empty string if the annotation is not set.
func GetProvisioningMode(js *jobset.JobSet) string {
	if js.Labels == nil {
		return ""
	}
	return js.Labels[SliceProvisioningLabel]
}
