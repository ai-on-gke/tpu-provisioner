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
)

const (
	ConfigMapName = "tpu-provisioner-static-nodepools-config"
)

type GscBlock struct {
	Name           string `yaml:"name"`
	Subblocks      string `yaml:"subblocks"`
	NodepoolPrefix string `yaml:"nodepoolPrefix"`
}

type Reservation struct {
	Name      string     `yaml:"name"`
	GscBlocks []GscBlock `yaml:"gscBlocks"`
}

// StaticNodepoolReconciler reconciles static nodepools based on a ConfigMap.
type StaticNodepoolReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	Provider                        cloud.Provider
	StaticNodepoolCreateConcurrency int
	StaticNodepoolDeleteConcurrency int
	StaticNodepoolCreateTimeout     time.Duration
	Namespace                       string
}

//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *StaticNodepoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := ctrllog.FromContext(ctx)

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

	var reservations []Reservation
	if err := yaml.Unmarshal([]byte(reservationsYAML), &reservations); err != nil {
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

	// List nodepools that should exist in the cluster based on the configmap.
	desiredNodePools, err := getNodePoolsFromConfig(reservations)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get desired nodepools from config: %w", err)
	}

	// List all static nodepools that currently exist in the cluster.
	existingNodePools, err := r.Provider.ListNodePools()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list existing nodepools: %w", err)
	}

	nodepoolsToDelete := []string{}
	for _, np := range existingNodePools {
		if np.Labels[cloud.LabelTPUProvisionerStaticNodepool] != "true" {
			continue
		}
		if _, shouldExist := desiredNodePools[np.Name]; !shouldExist {
			nodepoolsToDelete = append(nodepoolsToDelete, np.Name)
		}
	}

	if len(nodepoolsToDelete) > 0 {
		lg.Info("Deleting static nodepools not found in config", "nodepools", nodepoolsToDelete)
		errs := r.Provider.DeleteStaticNodePools(ctx, nodepoolsToDelete, r.StaticNodepoolDeleteConcurrency, &cm, "static nodepool not in config")
		if len(errs) > 0 {
			return ctrl.Result{}, fmt.Errorf("failed to delete some static nodepools: %v", errs)
		}
	}

	var allErrors []error
	for _, reservation := range reservations {
		for _, gscBlock := range reservation.GscBlocks {
			if gscBlock.NodepoolPrefix == "" {
				wrappedErr := fmt.Errorf("nodepoolPrefix cannot be empty for gscBlock: %s", gscBlock.Name)
				lg.Error(wrappedErr, "error ensuring static nodepool for gscBlock")
				allErrors = append(allErrors, wrappedErr)
				continue
			}
			lg.Info(fmt.Sprintf("Ensuring static nodepool for gscBlock: %s", gscBlock.Name))
			createCtx, cancel := context.WithTimeout(ctx, r.StaticNodepoolCreateTimeout)
			defer cancel()
			if err := r.Provider.EnsureStaticNodePools(createCtx, reservation.Name, gscBlock.Name, gscBlock.NodepoolPrefix, gscBlock.Subblocks, &nodepoolConfig, r.StaticNodepoolCreateConcurrency, &cm); err != nil {
				wrappedErr := fmt.Errorf("failed to ensure static nodepool for %s: %w", gscBlock.Name, err)
				lg.Error(wrappedErr, "error ensuring static nodepool for gscBlock")
				allErrors = append(allErrors, wrappedErr)
			}
		}
	}

	if len(allErrors) > 0 {
		return ctrl.Result{}, fmt.Errorf("failed to ensure all static nodepools: %v", allErrors)
	}

	return ctrl.Result{}, nil
}

func getNodePoolsFromConfig(reservations []Reservation) (map[string]struct{}, error) {
	nodePools := make(map[string]struct{})

	for _, reservation := range reservations {
		for _, gscBlock := range reservation.GscBlocks {
			start, end, err := cloud.ParseSubBlocks(gscBlock.Subblocks)
			if err != nil {
				return nil, fmt.Errorf("parsing subblocks for gscBlock %s: %w", gscBlock.Name, err)
			}

			for i := start; i <= end; i++ {
				formattedSubblockIndex := fmt.Sprintf("%04d", i)
				nodePoolID := fmt.Sprintf("%s-%s", gscBlock.NodepoolPrefix, formattedSubblockIndex)
				nodePools[nodePoolID] = struct{}{}
			}
		}
	}

	return nodePools, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticNodepoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(r)
}
