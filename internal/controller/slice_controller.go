package controller

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

type SliceReconciler struct {
	client.Client
	Recorder record.EventRecorder
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

	slices := jobsetSlices(&js)

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
			return ctrl.Result{}, fmt.Errorf("getting slice: %w", err)
		}
		// Does not exist - create.
		if err := r.Create(ctx, &slice); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating slice: %w", err)
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
				!autoProvisioningDisabledForJobSet(js) &&
				acceleratorsForJobSet(js)[v7xAccelerator]
		})).
		Owns(&v1alpha1.Slice{}).
		Complete(r)
}

func jobsetSlices(js *jobset.JobSet) []v1alpha1.Slice {
	var slices []v1alpha1.Slice

	for _, rj := range js.Spec.ReplicatedJobs {
		podNodeSelector := rj.Template.Spec.Template.Spec.NodeSelector
		if podNodeSelector == nil {
			continue
		}
		accel := podNodeSelector[acceleratorSelector]
		if accel != v7xAccelerator {
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

		for i := 0; i < int(rj.Replicas); i++ {
			s := v1alpha1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: js.Namespace,
					Name:      sliceName(js, rj.Name, i),
				},
				Spec: v1alpha1.SliceSpec{
					// TODO: Check that this is the correct accelerator value to use.
					AcceleratorType: accel,
					// TODO: check that this is the correct topology value to use.
					AcceleratorTopology: topo,
					// NodeSelector is a required field, should that be changed?
					NodeSelector: map[string][]string{},
				},
			}
			slices = append(slices, s)
		}
	}

	return slices
}

// sliceName formats a name for a Slice using the following pattern:
// {jobset_name[:34]}-{jobset_uid[:8]}-{replicated_job_name[:20]}-{replicated_job_replica}
// which should result in a max of 3 + 34 + 1 + 8 + 1 + 10 + 1 + 2 = 60 chars.
func sliceName(js *jobset.JobSet, replicatedJobName string, replicatedJobReplica int) string {
	return fmt.Sprintf("js-%s-%s-%s-%d",
		js.Name[:min(34, len(js.Name))],
		js.UID[:8],
		replicatedJobName[:min(10, len(replicatedJobName))],
		replicatedJobReplica,
	)
}
