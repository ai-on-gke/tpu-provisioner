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
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"
)

type LeaderWorkerSetSliceReconciler struct {
	client.Client
	Recorder                record.EventRecorder
	Scheme                  *runtime.Scheme
	RecreateConditions      []RecreateCondition
	ConditionalRecreateWait time.Duration
}

func (r *LeaderWorkerSetSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	log.V(3).Info("Reconciling LeaderWorkerSet to Slices")

	now := time.Now()

	var lwset lws.LeaderWorkerSet
	if err := r.Get(ctx, req.NamespacedName, &lwset); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting leaderworkerset: %w", err)
	}

	// Get all existing slices owned by this LWS
	var existingSliceList v1beta1.SliceList
	if err := r.List(ctx, &existingSliceList,
		client.MatchingLabels{
			SliceOwnerKindLabel:      LWSOwnerKind,
			SliceOwnerNameLabel:      lwset.Name,
			SliceOwnerNamespaceLabel: lwset.Namespace,
		}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing existing slices: %w", err)
	}

	// Check if LWS is being deleted
	if lwset.DeletionTimestamp != nil {
		log.Info("LeaderWorkerSet is being deleted, cleaning up Slices", "sliceCount", len(existingSliceList.Items))

		for _, slice := range existingSliceList.Items {
			if slice.DeletionTimestamp == nil {
				log.Info("Deleting Slice for LWS cleanup", "slice", slice.Name)
				if err := r.Delete(ctx, &slice); err != nil && !apierrors.IsNotFound(err) {
					r.Recorder.Eventf(&lwset, corev1.EventTypeWarning, "SliceDeleteFailed", "Failed to delete Slice %s: %v", slice.Name, err)
					return ctrl.Result{}, fmt.Errorf("deleting slice %s: %w", slice.Name, err)
				}
				r.Recorder.Eventf(&lwset, corev1.EventTypeNormal, "SliceDeleted", "Deleted Slice %s", slice.Name)
			}
		}

		if controllerutil.ContainsFinalizer(&lwset, SliceCleanupFinalizer) {
			log.Info("Removing finalizer from LeaderWorkerSet", "finalizer", SliceCleanupFinalizer)
			controllerutil.RemoveFinalizer(&lwset, SliceCleanupFinalizer)
			if err := r.Update(ctx, &lwset); err != nil {
				return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&lwset, SliceCleanupFinalizer) {
		log.Info("Adding finalizer to LeaderWorkerSet", "finalizer", SliceCleanupFinalizer)
		controllerutil.AddFinalizer(&lwset, SliceCleanupFinalizer)
		if err := r.Update(ctx, &lwset); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
	}

	desiredSlices, err := lwsSlices(&lwset)
	if err != nil {
		log.Error(err, "Error converting LeaderWorkerSet to Slices")
		return ctrl.Result{}, nil
	}

	// Determine which slices to delete and create
	toDelete, toCreate, diffRequeueAfter := diffSlices(desiredSlices, existingSliceList.Items, now, r.RecreateConditions, r.ConditionalRecreateWait)

	applyRequeueAfter, err := applySliceChanges(ctx, r.Client, r.Recorder, &lwset, toDelete, toCreate)
	if err != nil {
		return ctrl.Result{}, err
	}

	requeueAfter := minDuration(diffRequeueAfter, applyRequeueAfter)

	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

func (r *LeaderWorkerSetSliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lws.LeaderWorkerSet{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			if lwset, ok := object.(*lws.LeaderWorkerSet); ok {
				return utils.SliceProvisioningEnabled(lwset) &&
					(lwset.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector[acceleratorSelector] == tpu7xAccelerator ||
						lwset.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector[acceleratorSelector] == tpuV7xAccelerator)
			}
			return true
		})).
		Watches(
			&v1beta1.Slice{},
			handler.EnqueueRequestsFromMapFunc(r.sliceToLWSRequests),
		).
		Complete(r)
}

func (r *LeaderWorkerSetSliceReconciler) sliceToLWSRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	slice, ok := obj.(*v1beta1.Slice)
	if !ok || slice.Labels == nil {
		return nil
	}

	ownerKind := slice.Labels[SliceOwnerKindLabel]
	name := slice.Labels[SliceOwnerNameLabel]
	namespace := slice.Labels[SliceOwnerNamespaceLabel]

	if ownerKind != LWSOwnerKind {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func lwsSlices(lwset *lws.LeaderWorkerSet) ([]v1beta1.Slice, error) {
	var slices []v1beta1.Slice

	nodeSelector := lwset.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector
	accel := nodeSelector[acceleratorSelector]
	topo := lwset.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations[topologyAnnotation]

	replicas := 1
	if lwset.Spec.Replicas != nil {
		replicas = int(*lwset.Spec.Replicas)
	}

	// Parse slice selection annotation if present
	selection, err := parseLWSSliceSelection(lwset)
	if err != nil {
		return nil, fmt.Errorf("parsing slice selection: %w", err)
	}

	for i := 0; i < replicas; i++ {
		var partitionIds []string
		if s, ok := selection[lwset.Name]; ok && i < len(s) {
			partitionIds = s[i]
		}

		s := v1beta1.Slice{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.LWSSliceName(lwset.Name, string(lwset.UID), i),
				Labels: map[string]string{
					SliceOwnerKindLabel:      LWSOwnerKind,
					SliceOwnerNameLabel:      lwset.Name,
					SliceOwnerNamespaceLabel: lwset.Namespace,
				},
			},
			Spec: v1beta1.SliceSpec{
				Type:         v1beta1.Type(accel),
				Topology:     topo,
				PartitionIds: partitionIds,
			},
		}
		slices = append(slices, s)
	}

	return slices, nil
}

func parseLWSSliceSelection(lwset *lws.LeaderWorkerSet) (map[string][][]string, error) {
	if lwset.Annotations == nil {
		return nil, nil
	}
	val, ok := lwset.Annotations[SliceSelectionAnnotation]
	if !ok {
		return nil, nil
	}

	var selection map[string][][]string
	if err := json.Unmarshal([]byte(val), &selection); err != nil {
		return nil, fmt.Errorf("slice selection should be of the format '{\"<lws-name>\": [[\"cube-1\", \"cube-2\"], [...]]}': %w", err)
	}

	return selection, nil
}
