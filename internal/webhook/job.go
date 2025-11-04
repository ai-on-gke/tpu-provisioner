/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	batchv1 "k8s.io/api/batch/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

var log = ctrl.Log.WithName("webhook")

const SliceNodeSelector = "cloud.google.com/gke-tpu-slice"
const InjectSliceSelectorAnnotation = "tpu-provisioner.cloud.google.com/inject-slice-selector"

// JobMutationHandler handles admission requests for Job mutations
type JobMutationHandler struct {
	Decoder admission.Decoder
}

// Handle processes the admission request
func (h *JobMutationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.Info("mutating job", "namespace", req.Namespace, "name", req.Name)

	// Decode the Job object
	job := &batchv1.Job{}
	if err := h.Decoder.Decode(req, job); err != nil {
		log.Error(err, "failed to decode job")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if job.Annotations == nil {
		return admission.Response{}
	}
	if job.Annotations[InjectSliceSelectorAnnotation] != "true" {
		return admission.Response{}
	}
	var jobsetName string
	if jobsetName = job.Annotations[jobset.JobSetNameKey]; jobsetName == "" {
		return admission.Response{}
	}
	var replicatedJobName string
	if replicatedJobName = job.Annotations[jobset.ReplicatedJobNameKey]; replicatedJobName == "" {
		return admission.Response{}
	}
	var replicatedJobIndex int
	if idxStr := job.Annotations[jobset.JobIndexKey]; idxStr != "" {
		x, err := strconv.Atoi(idxStr)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("unable to parse replicated job index: %w", err))
		}
		replicatedJobIndex = x
	} else {
		return admission.Response{}
	}
	var jobsetUID string
	for _, ref := range job.OwnerReferences {
		if ref.Kind == "JobSet" {
			jobsetUID = string(ref.UID)
		}
	}
	if jobsetUID == "" {
		return admission.Response{}
	}

	if job.Spec.Template.Spec.NodeSelector == nil {
		job.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}

	key, val := SliceNodeSelector, utils.SliceName(
		jobsetName, jobsetUID, replicatedJobName, replicatedJobIndex)
	job.Spec.Template.Spec.NodeSelector[key] = val

	log.Info("added node selector to job",
		"namespace", req.Namespace,
		"name", job.Name,
		"key", key, "val", val)

	// Marshal the modified job
	marshaledJob, err := json.Marshal(job)
	if err != nil {
		log.Error(err, "failed to marshal modified job")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Return patch response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledJob)
}

// InjectDecoder injects the decoder
func (h *JobMutationHandler) InjectDecoder(d admission.Decoder) error {
	h.Decoder = d
	return nil
}
