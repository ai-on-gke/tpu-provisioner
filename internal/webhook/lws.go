package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"
)

const (
	LWSNameLabel       = "leaderworkerset.sigs.k8s.io/name"
	LWSGroupIndexLabel = "leaderworkerset.sigs.k8s.io/group-index"
)

// LWSStatefulSetMutationHandler handles admission requests for StatefulSet mutations belonging to an LWS
type LWSStatefulSetMutationHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// Handle processes the admission request
func (h *LWSStatefulSetMutationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	// Decode the StatefulSet object
	sts := &appsv1.StatefulSet{}
	if err := h.Decoder.Decode(req, sts); err != nil {
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

	// Get LWS UID
	var lwsUID string
	if component == "leader" {
		// For leader, we expect the LWS to be the owner
		for _, ref := range sts.OwnerReferences {
			if ref.Kind == "LeaderWorkerSet" {
				lwsUID = string(ref.UID)
				break
			}
		}
	} else {
		// For worker, owner is a Pod from the leader STS. Lookup the LWS directly to get LWS UID.
		lwsObj := &lws.LeaderWorkerSet{}
		var err error
		backoff := []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second}
		for i := 0; i <= len(backoff); i++ {
			err = h.Client.Get(ctx, client.ObjectKey{Name: lwsName, Namespace: sts.Namespace}, lwsObj)
			if err == nil {
				lwsUID = string(lwsObj.UID)
				break
			}
			if !apierrors.IsNotFound(err) {
				return admission.Errored(http.StatusInternalServerError, fmt.Errorf("getting LeaderWorkerSet %s/%s: %w", sts.Namespace, lwsName, err))
			}
			if i < len(backoff) {
				select {
				case <-ctx.Done():
					return admission.Errored(http.StatusInternalServerError, ctx.Err())
				case <-time.After(backoff[i]):
				}
			}
		}
		if err != nil {
			return admission.Errored(http.StatusNotFound, fmt.Errorf("LeaderWorkerSet %s/%s not found after retries: %w", sts.Namespace, lwsName, err))
		}
	}

	if lwsUID == "" {
		return admission.Allowed("missing LeaderWorkerSet owner")
	}

	if sts.Spec.Template.Spec.NodeSelector == nil {
		sts.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	key, val := SliceNodeSelector, utils.LWSSliceName(lwsName, lwsUID, component, replica)
	sts.Spec.Template.Spec.NodeSelector[key] = val

	// Marshal the modified statefulset
	marshaledSts, err := json.Marshal(sts)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Return patch response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledSts)
}
