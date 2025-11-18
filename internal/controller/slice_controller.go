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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

const cubeIDLabel = "cloud.google.com/gke-tpu-slice-4x4x4-id"

const SliceProvisioningLabel = "tpu-provisioner.cloud.google.com/slice-autoprovisioning"

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

	desiredSlices, err := jobsetSlices(&js)
	if err != nil {
		log.Error(err, "Error converting JobSet to Slices")
		return ctrl.Result{}, nil
	}

	// Get all existing slices owned by this JobSet
	var existingSliceList v1alpha1.SliceList
	if err := r.List(ctx, &existingSliceList, client.InNamespace(js.Namespace), client.MatchingFields{".metadata.controller": string(js.UID)}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing existing slices: %w", err)
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
			return ctrl.Result{}, fmt.Errorf("deleting slice %s/%s: %w", slice.Namespace, slice.Name, err)
		}
	}

	// Create new slices
	for _, slice := range toCreate {
		log.Info("Creating Slice for JobSet", "slice", slice.Name,
			"partitionCount", len(slice.Spec.PartitionIds))

		if err := controllerutil.SetControllerReference(&js, &slice, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting controller reference on slice %s/%s: %w", slice.Namespace, slice.Name, err)
		}

		if err := r.Create(ctx, &slice); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating slice %s/%s: %w", slice.Namespace, slice.Name, err)
		}
	}

	// Requeue in order to recreate.
	// NOTE: This should happen via an automatic re-reconcile after the DELETE, but
	// in integration tests it appeared not to happen.
	var res ctrl.Result
	if len(toDelete) > 0 {
		res.RequeueAfter = time.Second
	}

	return res, nil
}

func (r *SliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up an index to list Slices by their owner UID
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.Slice{}, ".metadata.controller", func(rawObj client.Object) []string {
		slice := rawObj.(*v1alpha1.Slice)
		owner := metav1.GetControllerOf(slice)
		if owner == nil {
			return nil
		}
		return []string{string(owner.UID)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&jobset.JobSet{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			js, ok := object.(*jobset.JobSet)
			if !ok {
				return false
			}
			accels := acceleratorsForJobSet(js)
			return sliceProvisioningEnabled(js) &&
				!autoProvisioningDisabledForJobSet(js) &&
				(accels[tpu7xAccelerator] || accels[tpuV7xAccelerator])
		})).
		Owns(&v1alpha1.Slice{}).
		Complete(r)
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
					Namespace: js.Namespace,
					Name:      utils.SliceName(js.Name, string(js.UID), rj.Name, i),
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
