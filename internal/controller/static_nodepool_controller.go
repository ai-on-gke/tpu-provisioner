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

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	ConfigMapName = "static-nodepools-config"
)

// StaticNodepoolReconciler reconciles static nodepools based on a ConfigMap.
type StaticNodepoolReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	Provider                    cloud.Provider
	Concurrency                 int
	StaticNodepoolCreateTimeout time.Duration
}

//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *StaticNodepoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := ctrllog.FromContext(ctx)

	// Only reconcile if this configmap is the one we are looking for.
	if req.Name != ConfigMapName {
		return ctrl.Result{}, nil
	}

	lg.V(3).Info("Reconciling static nodepools")

	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			lg.Info("Static nodepools config map not found. Skipping reconciliation.", "configmap", req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get configmap %s: %w", req.NamespacedName.String(), err)
	}

	reservationsYAML, ok := cm.Data["reservations"]
	if !ok {
		lg.Info("No 'reservations' key in configmap. Skipping reconciliation.", "configmap", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	var reservationNames []string
	if err := yaml.Unmarshal([]byte(reservationsYAML), &reservationNames); err != nil {
		lg.Error(err, "failed to unmarshal reservations from configmap", "configmap", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	nodepoolConfigYAML, ok := cm.Data["nodepoolConfig"]
	if !ok {
		lg.Info("No 'nodepoolConfig' key in configmap. Skipping reconciliation.", "configmap", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	var nodepoolConfig cloud.StaticNodePoolConfig
	if err := yaml.Unmarshal([]byte(nodepoolConfigYAML), &nodepoolConfig); err != nil {
		lg.Error(err, "failed to unmarshal nodepoolConfig from configmap", "configmap", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	var allErrors []error
	for _, reservationName := range reservationNames {
		lg.Info(fmt.Sprintf("Ensuring static nodepool for reservation: %s", reservationName))
		if err := r.Provider.EnsureStaticNodePools(ctx, reservationName, &nodepoolConfig, r.Concurrency, r.StaticNodepoolCreateTimeout); err != nil {
			wrappedErr := fmt.Errorf("failed to ensure static nodepool for %s: %w", reservationName, err)
			lg.Error(wrappedErr, "error ensuring static nodepool for reservation")
			allErrors = append(allErrors, wrappedErr)
		}
	}

	if len(allErrors) > 0 {
		return ctrl.Result{}, fmt.Errorf("failed to ensure all static nodepools: %v", allErrors)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticNodepoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.GetName() == ConfigMapName
		})).
		Complete(r)
}
