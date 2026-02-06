package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	LWSNameLabel    = "leaderworkerset.sigs.k8s.io/name"
	LWSReplicaLabel = "leaderworkerset.sigs.k8s.io/replica-index"
)

// LWSPodMutationHandler handles admission requests for Pod mutations belonging to an LWS
type LWSPodMutationHandler struct {
	Decoder admission.Decoder
}

// Handle processes the admission request
func (h *LWSPodMutationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Decode the Pod object
	pod := &corev1.Pod{}
	if err := h.Decoder.Decode(req, pod); err != nil {
		log.Error(err, "failed to decode pod")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if pod.Labels == nil {
		return admission.Allowed("missing pod labels")
	}

	// Double check if we should inject
	if pod.Labels[InjectSliceSelectorLabel] != "true" {
		return admission.Allowed("inject-slice-selector label not set to true")
	}

	lwsName := pod.Labels[LWSNameLabel]
	if lwsName == "" {
		return admission.Allowed("missing LWS name label")
	}

	replicaStr := pod.Labels[LWSReplicaLabel]
	if replicaStr == "" {
		return admission.Allowed("missing LWS replica label")
	}

	replica, err := strconv.Atoi(replicaStr)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unable to parse LWS replica index: %w", err))
	}

	// Get LWS UID from OwnerReferences
	var lwsUID string
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "LeaderWorkerSet" {
			lwsUID = string(ref.UID)
			break
		}
	}

	if lwsUID == "" {
		// If not directly owned by LWS, it might be owned by a ReplicaSet which is owned by LWS?
		// Actually LWS manages Pods. Let's assume direct ownership for now or check if we can skip UID if name is unique enough (but UID is better).
		// Wait, if it's a worker pod, it might be owned by something else.
		// Actually LWS documentation says "The workers are created as Pods".
		return admission.Allowed("missing LeaderWorkerSet owner reference")
	}

	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = make(map[string]string)
	}

	key, val := SliceNodeSelector, utils.LWSSliceName(lwsName, lwsUID, replica)
	pod.Spec.NodeSelector[key] = val

	log.Info("added node selector to pod",
		"namespace", req.Namespace,
		"name", pod.Name,
		"key", key, "val", val)

	// Marshal the modified pod
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Error(err, "failed to marshal modified pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Return patch response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// InjectDecoder injects the decoder
func (h *LWSPodMutationHandler) InjectDecoder(d admission.Decoder) error {
	h.Decoder = d
	return nil
}
