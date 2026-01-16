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
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
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

type gscBlock struct {
	Name           string `yaml:"name"`
	Subblocks      string `yaml:"subblocks"`
	NodepoolPrefix string `yaml:"nodepoolPrefix"`
}

type reservation struct {
	Name      string     `yaml:"name"`
	GscBlocks []gscBlock `yaml:"gscBlocks"`
}

// StaticNodepoolReconciler reconciles static nodepools based on a configmap.
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
			// If the configmap is not found, do nothing.
			lg.Info("Static nodepools configmap not found. Taking no action.", "configmap", req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get configmap %s: %w", req.NamespacedName.String(), err)
	}

	reservationsYAML, ok := cm.Data["reservations"]
	if !ok {
		lg.Info("No 'reservations' key in configmap. Skipping reconciliation.", "configmap", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	var reservations []reservation
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
	desiredNodePools, err := r.constructDesiredNodePools(reservations, &nodepoolConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get desired nodepools from config: %w", err)
	}

	// List all static nodepools that currently exist in the cluster.
	existingNodePools, err := r.Provider.ListNodePools()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list existing nodepools: %w", err)
	}

	toCreate, toDeleteMissing, toDeleteUpdate, toDeleteError, err := r.Provider.DiffStaticNodePools(existingNodePools, desiredNodePools)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to diff nodepools: %w", err)
	}

	if len(toDeleteMissing) > 0 {
		lg.Info("Deleting static nodepools not found in config", "nodepools", toDeleteMissing)
		errs := r.Provider.DeleteStaticNodePools(ctx, toDeleteMissing, r.StaticNodepoolDeleteConcurrency, &cm, "static nodepool not in config")
		if len(errs) > 0 {
			return ctrl.Result{}, fmt.Errorf("failed to delete some static nodepools: %v", errs)
		}
	}

	if len(toDeleteError) > 0 {
		lg.Info("Deleting static nodepools in ERROR state to retry", "nodepools", toDeleteError)
		errs := r.Provider.DeleteStaticNodePools(ctx, toDeleteError, r.StaticNodepoolDeleteConcurrency, &cm, "static nodepool in error state")
		if len(errs) > 0 {
			return ctrl.Result{}, fmt.Errorf("failed to delete some static nodepools: %v", errs)
		}
	}

	if len(toDeleteUpdate) > 0 {
		lg.Info("Deleting static nodepools to recreate (config changed)", "nodepools", toDeleteUpdate)
		errs := r.Provider.DeleteStaticNodePools(ctx, toDeleteUpdate, r.StaticNodepoolDeleteConcurrency, &cm, "static nodepool config changed")
		if len(errs) > 0 {
			return ctrl.Result{}, fmt.Errorf("failed to delete some static nodepools: %v", errs)
		}
	}

	if len(toCreate) > 0 {
		lg.Info("Creating static nodepools", "nodepools", toCreate)
		createCtx, cancel := context.WithTimeout(ctx, r.StaticNodepoolCreateTimeout)
		defer cancel()
		if err := r.Provider.EnsureStaticNodePools(createCtx, toCreate, r.StaticNodepoolCreateConcurrency, &cm); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure static nodepools: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *StaticNodepoolReconciler) constructDesiredNodePools(reservations []reservation, nodepoolConfig *cloud.StaticNodePoolConfig) ([]*cloud.DesiredStaticNodePool, error) {
	var desiredNodePools []*cloud.DesiredStaticNodePool

	for _, reservation := range reservations {
		for _, gscBlock := range reservation.GscBlocks {
			start, end, err := cloud.ParseSubBlocks(gscBlock.Subblocks)
			if err != nil {
				return nil, fmt.Errorf("parsing subblocks for gscBlock %s: %w", gscBlock.Name, err)
			}

			for i := start; i <= end; i++ {
				nodePoolID := utils.SetNodePoolName(gscBlock.NodepoolPrefix, gscBlock.Name, i)
				subblockName := fmt.Sprintf("%s-subblock-%04d", gscBlock.Name, i)
				subblockToConsume := fmt.Sprintf("projects/%s/reservations/%s/reservationBlocks/%s/reservationSubBlocks/%s", r.Provider.ProjectID(), reservation.Name, gscBlock.Name, subblockName)

				desiredNodePools = append(desiredNodePools, &cloud.DesiredStaticNodePool{
					Name:              nodePoolID,
					SubblockToConsume: subblockToConsume,
					Config:            nodepoolConfig,
				})
			}
		}
	}

	return desiredNodePools, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticNodepoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(r)
}
