package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1alpha1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
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

// Finalizer to ensure Slices are cleaned up when JobSet is deleted
const SliceCleanupFinalizer = "tpu-provisioner.cloud.google.com/slice-cleanup"

// Labels used to track which resource (i.e. JobSet) owns a Slice
// since Cluster scopred resources cannot use owner references to Namespaced resources.
const (
	SliceOwnerKindLabel      = "tpu-provisioner.cloud.google.com/owner-kind"
	SliceOwnerNameLabel      = "tpu-provisioner.cloud.google.com/owner-name"
	SliceOwnerNamespaceLabel = "tpu-provisioner.cloud.google.com/owner-namespace"
)

/*
Example value:

	{
	  "replicated_job_name": [
	    ["cube-1", "cube-2"], # Replica 0
		["cube-3", "cube-4"]  # Replica 1
	  ]
	}
*/
const SliceSelectionAnnotation = "tpu-provisioner.cloud.google.com/slice-selection"

type SliceReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

func (r *SliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.V(3).Info("Reconciling JobSet to Slices")

	var js jobset.JobSet
	if err := r.Get(ctx, req.NamespacedName, &js); err != nil {
		if apierrors.IsNotFound(err) {
			// Don't requeue, JobSet no longer exists.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting jobset: %w", err)
	}

	// Get all existing slices owned by this JobSet (using labels instead of owner references)
	var existingSliceList v1alpha1.SliceList
	if err := r.List(ctx, &existingSliceList,
		client.MatchingLabels{
			SliceOwnerKindLabel:      "jobset",
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
					return ctrl.Result{}, fmt.Errorf("deleting slice %s: %w", slice.Name, err)
				}
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

	desiredSlices, err := jobsetSlices(&js)
	if err != nil {
		log.Error(err, "Error converting JobSet to Slices")
		return ctrl.Result{}, nil
	}

	// Determine which slices to delete and create
	toDelete, toCreate := diffSlices(desiredSlices, existingSliceList.Items)

	// Delete slices that have changed
	for _, slice := range toDelete {
		if slice.DeletionTimestamp != nil {
			log.Info("Skipping deletion of Slice due to NodeSelector change since the Slice is already marked for deletion", "slice", slice.Name)
			continue
		}
		log.Info("Deleting Slice due to NodeSelector change", "slice", slice.Name)
		if err := r.Delete(ctx, &slice); err != nil {
			return ctrl.Result{}, fmt.Errorf("deleting slice %s: %w", slice.Name, err)
		}
	}

	// Create new slices
	for _, slice := range toCreate {
		log.Info("Creating Slice for JobSet", "slice", slice.Name,
			"partitionCount", len(slice.Spec.PartitionIds))

		if err := r.Create(ctx, &slice); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating slice %s: %w", slice.Name, err)
		}
	}

	// Handle sync mode: suspend JobSet until all Slices are Ready
	if utils.GetProvisioningMode(&js) == utils.SliceProvisioningModeSync {
		if err := r.handleSyncMode(ctx, &js, desiredSlices, existingSliceList.Items); err != nil {
			return ctrl.Result{}, fmt.Errorf("handling sync mode: %w", err)
		}
	}

	// Requeue in order to recreate.
	// NOTE: This should happen via an automatic re-reconcile after the DELETE, but
	// in integration tests it appears not to happen.
	var res ctrl.Result
	if len(toDelete) > 0 {
		res.RequeueAfter = time.Second
	}

	return res, nil
}

func (r *SliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
			&v1alpha1.Slice{},
			handler.EnqueueRequestsFromMapFunc(r.sliceToJobSetRequests),
		).
		Complete(r)
}

// sliceToJobSetRequests maps a Slice to its owning JobSet using labels
func (r *SliceReconciler) sliceToJobSetRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	slice, ok := obj.(*v1alpha1.Slice)
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

	if ownerKind != "jobset" || !hasName || !hasNamespace {
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

func jobsetSlices(js *jobset.JobSet) ([]v1alpha1.Slice, error) {
	var slices []v1alpha1.Slice

	sliceSelection, err := parseSliceSelection(js)
	if err != nil {
		return nil, fmt.Errorf("parsing slice selection: %w", err)
	}

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
			s := v1alpha1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: utils.SliceName(js.Name, string(js.UID), rj.Name, i),
					Labels: map[string]string{
						// Track ownership with labels (can't use owner references since Slice is Cluster scoped)
						SliceOwnerKindLabel:      "jobset",
						SliceOwnerNameLabel:      js.Name,
						SliceOwnerNamespaceLabel: js.Namespace,
					},
				},
				Spec: v1alpha1.SliceSpec{
					// TODO: Check that this is the correct accelerator value to use.
					Type: v1alpha1.Type(accel),
					// TODO: check that this is the correct topology value to use.
					Topology: topo,
				},
			}
			if len(cubeSelection) >= i+1 {
				s.Spec.PartitionIds = cubeSelection[i]
			} else {
				// PartitionIds is a required field, should that be changed?
				// TODO: Revisit this - I commented out the requirement in the test CRD for now
				// since there is also a min(1) requirement.
				//s.Spec.PartitionIds = []string{}
			}
			slices = append(slices, s)
		}
	}

	return slices, nil
}

// parseSliceSelection returns a map["replicated_job_name"][replicated_job_index]["cube", "cube"]
// from the parsed annotation.
// returns an empty map if there is no annotation.
func parseSliceSelection(js *jobset.JobSet) (map[string][][]string, error) {
	var sliceSelection map[string][][]string
	if js.Annotations != nil {
		selectionJSON, ok := js.Annotations[SliceSelectionAnnotation]
		if ok {
			if err := json.Unmarshal([]byte(selectionJSON), &sliceSelection); err != nil {
				return nil, fmt.Errorf(`slice selection should be of the format {"replicated_job_name": [["cube-1","cube-2"],["cube-3","cube-4"]]}: %w`, err)
			}
			return sliceSelection, nil
		}
	}
	return make(map[string][][]string), nil
}

// partitionsEqual compares two partition slices for equality.
func partitionsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// diffSlices compares desired slices with existing slices and returns
// lists of slices to delete and create.
// Slices are considered different if their NodeSelectors differ.
// When a slice needs to be deleted due to NodeSelector change, it is NOT included
// in toCreate - the creation will happen in a subsequent reconciliation pass.
func diffSlices(desired []v1alpha1.Slice, existing []v1alpha1.Slice) (toDelete, toCreate []v1alpha1.Slice) {
	// Create a map of existing slices by name for quick lookup
	existingMap := make(map[string]*v1alpha1.Slice)
	for i := range existing {
		existingMap[existing[i].Name] = &existing[i]
	}

	// Check each desired slice
	for _, desiredSlice := range desired {
		if existingSlice, exists := existingMap[desiredSlice.Name]; exists {
			// Slice exists - check if NodeSelector has changed
			if !partitionsEqual(existingSlice.Spec.PartitionIds, desiredSlice.Spec.PartitionIds) {
				// NodeSelector changed - delete existing (creation will happen in next reconcile)
				toDelete = append(toDelete, *existingSlice)
			}
			// Otherwise, slice matches - no action needed
		} else {
			// Slice doesn't exist - create it
			toCreate = append(toCreate, desiredSlice)
		}
	}

	return toDelete, toCreate
}

// handleSyncMode handles the sync provisioning mode by suspending the JobSet
// until all expected Slices are Ready, then unsuspending it.
func (r *SliceReconciler) handleSyncMode(ctx context.Context, js *jobset.JobSet, desiredSlices []v1alpha1.Slice, existingSlices []v1alpha1.Slice) error {
	log := ctrllog.FromContext(ctx)

	allReady := allSlicesReady(desiredSlices, existingSlices)
	currentlySuspended := js.Spec.Suspend != nil && *js.Spec.Suspend

	if allReady && currentlySuspended {
		// All slices are ready, unsuspend the JobSet
		log.Info("All Slices are Ready, unsuspending JobSet")
		suspendValue := false
		js.Spec.Suspend = &suspendValue
		if err := r.Update(ctx, js); err != nil {
			return fmt.Errorf("unsuspending jobset: %w", err)
		}
		r.Recorder.Event(js, "Normal", "Unsuspended", "All Slices are Ready")
	} else if !allReady && !currentlySuspended {
		// Not all slices are ready, suspend the JobSet
		log.Info("Not all Slices are Ready, suspending JobSet",
			"readyCount", countReadySlices(existingSlices),
			"expectedCount", len(desiredSlices))
		suspendValue := true
		js.Spec.Suspend = &suspendValue
		if err := r.Update(ctx, js); err != nil {
			return fmt.Errorf("suspending jobset: %w", err)
		}
		r.Recorder.Event(js, "Normal", "Suspended", "Waiting for all Slices to be Ready")
	}

	return nil
}

// allSlicesReady checks if all desired Slices exist and have the Ready condition set to true.
func allSlicesReady(desiredSlices []v1alpha1.Slice, existingSlices []v1alpha1.Slice) bool {
	if len(desiredSlices) == 0 {
		return true
	}

	// Create a map of existing slices by name for quick lookup
	existingMap := make(map[string]*v1alpha1.Slice)
	for i := range existingSlices {
		existingMap[existingSlices[i].Name] = &existingSlices[i]
	}

	// Check that all desired slices exist and are Ready
	for _, desired := range desiredSlices {
		existing, exists := existingMap[desired.Name]
		if !exists {
			// Slice doesn't exist yet
			return false
		}
		if !isSliceReady(existing) {
			// Slice exists but is not Ready
			return false
		}
	}

	return true
}

// isSliceReady checks if a Slice has the Ready condition set to true.
func isSliceReady(slice *v1alpha1.Slice) bool {
	for _, condition := range slice.Status.Conditions {
		if condition.Type == v1alpha1.SliceStateConditionType &&
			condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// countReadySlices returns the number of Slices that have the Ready condition set to true.
func countReadySlices(slices []v1alpha1.Slice) int {
	count := 0
	for i := range slices {
		if isSliceReady(&slices[i]) {
			count++
		}
	}
	return count
}
