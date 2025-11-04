package controller

import (
	"context"
	"encoding/json"
	"fmt"

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

const cubeSelectionLabel = "cloud.google.com/gke-nodepool"

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
	lg := ctrllog.FromContext(ctx)

	lg.V(3).Info("Reconciling JobSet to Slices")

	var js jobset.JobSet
	if err := r.Get(ctx, req.NamespacedName, &js); err != nil {
		if apierrors.IsNotFound(err) {
			// Don't requeue, JobSet no longer exists.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting jobset: %w", err)
	}

	slices, err := jobsetSlices(&js)
	if err != nil {
		lg.Error(err, "Error converting JobSet to Slices")
		return ctrl.Result{}, nil
	}

	for _, slice := range slices {
		err := r.Get(ctx, req.NamespacedName, &v1alpha1.Slice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      slice.Name,
				Namespace: slice.Namespace,
			},
		})
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("getting slice %s/%s: %w", slice.Namespace, slice.Name, err)
		}

		if err := controllerutil.SetControllerReference(&js, &slice, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting controller reference on slice %s/%s: %w", slice.Namespace, slice.Name, err)
		}

		// Does not exist - create.
		if err := r.Create(ctx, &slice); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating slice %s/%s: %w", slice.Namespace, slice.Name, err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *SliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jobset.JobSet{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			js, ok := object.(*jobset.JobSet)
			return ok &&
				sliceProvisioningEnabled(js) &&
				!autoProvisioningDisabledForJobSet(js) &&
				acceleratorsForJobSet(js)[tpu7xAccelerator]
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
		if accel != tpu7xAccelerator {
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
					AcceleratorType: accel,
					// TODO: check that this is the correct topology value to use.
					AcceleratorTopology: topo,
				},
			}
			if len(cubeSelection) >= i+1 {
				s.Spec.NodeSelector = map[string][]string{
					cubeSelectionLabel: cubeSelection[i],
				}
			} else {
				// NodeSelector is a required field, should that be changed?
				s.Spec.NodeSelector = map[string][]string{}
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
