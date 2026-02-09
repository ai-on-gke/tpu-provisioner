package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	LWSNameLabel       = "leaderworkerset.sigs.k8s.io/name"
	LWSGroupIndexLabel = "leaderworkerset.sigs.k8s.io/group-index"
)

// LWSStatefulSetMutationHandler handles admission requests for StatefulSet mutations belonging to an LWS
type LWSStatefulSetMutationHandler struct {
	Decoder admission.Decoder
}

// Handle processes the admission request
func (h *LWSStatefulSetMutationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Decode the StatefulSet object
	sts := &appsv1.StatefulSet{}
	if err := h.Decoder.Decode(req, sts); err != nil {
		log.Error(err, "failed to decode statefulset")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if sts.Labels == nil {
		return admission.Allowed("missing statefulset labels")
	}

	// Double check if we should inject
	if sts.Spec.Template.Labels == nil {
		return admission.Allowed("missing statefulset template labels")
	}
	if sts.Spec.Template.Labels[InjectSliceSelectorLabel] != "true" {
		return admission.Allowed("inject-slice-selector label on pod template not set to true")
	}

	lwsName := sts.Labels[LWSNameLabel]
	if lwsName == "" {
		return admission.Allowed("missing LWS name label")
	}

	// Get LWS UID from OwnerReferences
	var lwsUID string
	for _, ref := range sts.OwnerReferences {
		if ref.Kind == "LeaderWorkerSet" {
			lwsUID = string(ref.UID)
			break
		}
	}

	if lwsUID == "" {
		return admission.Allowed("missing LeaderWorkerSet owner reference")
	}

	if sts.Spec.Template.Spec.NodeSelector == nil {
		sts.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}

	replica := -1
	component := "leader"
	if groupIndexStr, ok := sts.Labels[LWSGroupIndexLabel]; ok {
		var err error
		replica, err = strconv.Atoi(groupIndexStr)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("unable to parse LWS group index: %w", err))
		}
		component = "worker"
	}

	key, val := SliceNodeSelector, utils.LWSSliceName(lwsName, lwsUID, component, replica)
	sts.Spec.Template.Spec.NodeSelector[key] = val

	log.Info("added node selector to statefulset",
		"namespace", req.Namespace,
		"name", sts.Name,
		"key", key, "val", val)

	// Marshal the modified statefulset
	marshaledSts, err := json.Marshal(sts)
	if err != nil {
		log.Error(err, "failed to marshal modified statefulset")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Return patch response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledSts)
}

// InjectDecoder injects the decoder
func (h *LWSStatefulSetMutationHandler) InjectDecoder(d admission.Decoder) error {
	h.Decoder = d
	return nil
}
