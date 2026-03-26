package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

type JobSetSliceReconciler struct {
	client.Client
	Recorder                record.EventRecorder
	Scheme                  *runtime.Scheme
	RecreateConditions      []RecreateCondition
	ConditionalRecreateWait time.Duration
}

func (r *JobSetSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.V(3).Info("Reconciling JobSet to Slices")

	now := Now()

	var js jobset.JobSet
	if err := r.Get(ctx, req.NamespacedName, &js); err != nil {
		if apierrors.IsNotFound(err) {
			// Don't requeue, JobSet no longer exists.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting jobset: %w", err)
	}

	// Get all existing slices owned by this JobSet (using labels instead of owner references)
	var existingSliceList v1beta1.SliceList
	if err := r.List(ctx, &existingSliceList,
		client.MatchingLabels{
			SliceOwnerKindLabel:      jobSetOwnerKind,
			SliceOwnerNameLabel:      js.Name,
			SliceOwnerNamespaceLabel: js.Namespace,
		}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing existing slices: %w", err)
	}

	// Check if JobSet is being deleted or is in a terminal state
	if js.DeletionTimestamp != nil || jobSetCompleted(&js) || jobSetFailed(&js) {
		// Delete all Slices for this JobSet
		log.Info("JobSet is being deleted, cleaning up Slices", "sliceCount", len(existingSliceList.Items))

		for _, slice := range existingSliceList.Items {
			if slice.DeletionTimestamp == nil {
				log.Info("Deleting Slice for JobSet cleanup", "slice", slice.Name)
				if err := r.Delete(ctx, &slice); err != nil && !apierrors.IsNotFound(err) {
					r.Recorder.Eventf(&js, corev1.EventTypeWarning, "SliceDeleteFailed", "Failed to delete Slice %s: %v", slice.Name, err)
					return ctrl.Result{}, fmt.Errorf("deleting slice %s: %w", slice.Name, err)
				}
				r.Recorder.Eventf(&js, corev1.EventTypeNormal, "SliceDeleted", "Deleted Slice %s", slice.Name)
			}
		}

		// Remove finalizer once all deletion requests have been issued
		// (don't wait for Slices to be fully deleted/finalized)
		if controllerutil.ContainsFinalizer(&js, SliceCleanupFinalizer) {
			log.Info("Removing finalizer from JobSet", "finalizer", SliceCleanupFinalizer)
			controllerutil.RemoveFinalizer(&js, SliceCleanupFinalizer)
			if err := r.Update(ctx, &js); err != nil {
				return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&js, SliceCleanupFinalizer) {
		log.Info("Adding finalizer to JobSet", "finalizer", SliceCleanupFinalizer)
		controllerutil.AddFinalizer(&js, SliceCleanupFinalizer)
		if err := r.Update(ctx, &js); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
	}

	desiredSlices, legacyNames, err := jobsetSlices(&js)
	if err != nil {
		log.Error(err, "Error converting JobSet to Slices")
		return ctrl.Result{}, nil
	}

	// Determine which slices to delete and create
	toDelete, toCreate, diffRequeueAfter := diffSlices(desiredSlices, existingSliceList.Items, legacyNames, now, r.RecreateConditions, r.ConditionalRecreateWait)
	if diffRequeueAfter > 0 {
		log.Info("Some Slices need to be deleted, but are in a state that requires requeuing", "requeueAfter", diffRequeueAfter)
	}

	applyRequeueAfter, err := applySliceChanges(ctx, r.Client, r.Recorder, &js, toDelete, toCreate)
	if err != nil {
		return ctrl.Result{}, err
	}

	requeueAfter := minDuration(diffRequeueAfter, applyRequeueAfter)

	// Handle sync mode: suspend JobSet until all Slices are Ready
	if utils.GetProvisioningMode(&js) == utils.SliceProvisioningModeSync {
		if err := r.handleSyncMode(ctx, &js, desiredSlices, existingSliceList.Items, legacyNames); err != nil {
			return ctrl.Result{}, fmt.Errorf("handling sync mode: %w", err)
		}
	}

	if requeueAfter > 0 {
		log.Info("Requeueing JobSet", "requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

func (r *JobSetSliceReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&jobset.JobSet{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			if js, ok := object.(*jobset.JobSet); ok {
				accels := acceleratorsForJobSet(js)
				return utils.SliceProvisioningEnabled(js) &&
					!utils.AutoProvisioningDisabledForJobSet(js) &&
					(accels[tpu7xAccelerator] || accels[tpuV7xAccelerator])
			}
			return true
		})).
		Watches(
			&v1beta1.Slice{},
			handler.EnqueueRequestsFromMapFunc(r.sliceToJobSetRequests),
		).
		Complete(r)
}

// sliceToJobSetRequests maps a Slice to its owning JobSet using labels
func (r *JobSetSliceReconciler) sliceToJobSetRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	slice, ok := obj.(*v1beta1.Slice)
	if !ok {
		return nil
	}

	// Get the owning JobSet from labels
	if slice.Labels == nil {
		return nil
	}

	ownerKind := slice.Labels[SliceOwnerKindLabel]
	jobsetName, hasName := slice.Labels[SliceOwnerNameLabel]
	jobsetNamespace, hasNamespace := slice.Labels[SliceOwnerNamespaceLabel]

	if ownerKind != jobSetOwnerKind || !hasName || !hasNamespace {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      jobsetName,
				Namespace: jobsetNamespace,
			},
		},
	}
}

// jobsetSlices returns the desired slices for a JobSet along with a legacy name
// map (new name -> legacy name) for backwards-compatible matching.
func jobsetSlices(js *jobset.JobSet) ([]v1beta1.Slice, map[string]string, error) {
	var slices []v1beta1.Slice
	legacyNames := make(map[string]string)

	sliceSelection, err := parseJobSetSliceSelection(js)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing slice selection: %w", err)
	}

	usedPartitions := make(map[string]bool)

	for _, rj := range js.Spec.ReplicatedJobs {
		podNodeSelector := rj.Template.Spec.Template.Spec.NodeSelector
		if podNodeSelector == nil {
			continue
		}
		accel := podNodeSelector[acceleratorSelector]
		switch accel {
		case tpu7xAccelerator, tpuV7xAccelerator:
		default:
			continue
		}
		podAnnotations := rj.Template.Spec.Template.Annotations
		if podAnnotations == nil {
			continue
		}
		topo, topoAnnExists := podAnnotations[topologyAnnotation]
		if !topoAnnExists {
			continue
		}

		cubeSelection := sliceSelection[rj.Name]
		for i := 0; i < int(rj.Replicas); i++ {
			newName := utils.SliceName(js.Name, string(js.UID), rj.Name, i)
			legacyName := utils.LegacySliceName(js.Name, string(js.UID), rj.Name, i)
			if newName != legacyName {
				legacyNames[newName] = legacyName
			}

			s := v1beta1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: newName,
					Labels: map[string]string{
						// Track ownership with labels (can't use owner references since Slice is Cluster scoped)
						SliceOwnerKindLabel:      jobSetOwnerKind,
						SliceOwnerNameLabel:      js.Name,
						SliceOwnerNamespaceLabel: js.Namespace,
					},
				},
				Spec: v1beta1.SliceSpec{
					Type:     v1beta1.Type(accel),
					Topology: topo,
				},
			}
			if len(cubeSelection) >= i+1 {
				s.Spec.PartitionIds = cubeSelection[i]
			}
			slices = append(slices, s)

			// Check for internal overlap in the desired state
			for _, p := range s.Spec.PartitionIds {
				if usedPartitions[p] {
					return nil, nil, fmt.Errorf("duplicate partition ID %q found in slice selection", p)
				}
				usedPartitions[p] = true
			}
		}
	}

	return slices, legacyNames, nil
}

// handleSyncMode handles the sync provisioning mode by suspending the JobSet
// until all expected Slices are Ready, then unsuspending it.
func (r *JobSetSliceReconciler) handleSyncMode(ctx context.Context, js *jobset.JobSet, desiredSlices []v1beta1.Slice, existingSlices []v1beta1.Slice, legacyNames map[string]string) error {
	log := ctrllog.FromContext(ctx)

	slicesReady := allSlicesReady(desiredSlices, existingSlices, legacyNames)
	jsCurrentlySuspended := js.Spec.Suspend != nil && *js.Spec.Suspend

	if slicesReady && jsCurrentlySuspended {
		// All slices are ready, unsuspend the JobSet
		log.Info("All Slices are Ready, unsuspending JobSet")
		suspendValue := false
		js.Spec.Suspend = &suspendValue
		if err := r.Update(ctx, js); err != nil {
			r.Recorder.Event(js, corev1.EventTypeNormal, "JobSetUnsuspendFailed", "Failed to unsuspend JobSet")
			return fmt.Errorf("unsuspending jobset: %w", err)
		}
		r.Recorder.Event(js, corev1.EventTypeNormal, "JobSetUnsuspended", "All Slices are Ready")
	} else if !slicesReady && !jsCurrentlySuspended {
		// Not all slices are ready, suspend the JobSet
		log.Info("Not all Slices are Ready, suspending JobSet",
			"readyCount", countReadySlices(existingSlices),
			"expectedCount", len(desiredSlices))
		suspendValue := true
		js.Spec.Suspend = &suspendValue
		if err := r.Update(ctx, js); err != nil {
			r.Recorder.Event(js, corev1.EventTypeNormal, "JobSetSuspendFailed", "Failed to suspend JobSet")
			return fmt.Errorf("suspending jobset: %w", err)
		}
		r.Recorder.Event(js, corev1.EventTypeNormal, "JobSetSuspended", "Waiting for all Slices to be Ready")
	}

	return nil
}

// parseSliceSelection returns a map["replicated_job_name"][replicated_job_index]["cube", "cube"]
// from the parsed annotation.
// returns an empty map if there is no annotation.
func parseJobSetSliceSelection(js *jobset.JobSet) (map[string][][]string, error) {
	var sliceSelection map[string][][]string
	if js.Annotations != nil {
		selectionJSON, ok := js.Annotations[SliceSelectionAnnotation]
		if ok {
			if err := json.Unmarshal([]byte(selectionJSON), &sliceSelection); err != nil {
				return nil, fmt.Errorf(`slice selection should be of the format {"replicated_job_name": [["cube-1","cube-2"],["cube-3","cube-4"]]}: %w`, err)
			}
			return sliceSelection, nil
		} else {
			return nil, fmt.Errorf("missing slice selection annotation: %q", SliceSelectionAnnotation)
		}
	}
	return make(map[string][][]string), nil
}
