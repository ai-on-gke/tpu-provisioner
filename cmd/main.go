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

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/cmd/config"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	//_ "k8s.io/client-go/plugin/pkg/client/auth"

	// NOTE: The above auth import does not work for GKE 1.26 and up.
	// https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke
	_ "github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/auth/gcp"

	"cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/controller"
	jobwebhook "github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/webhook"

	containerv1beta1 "google.golang.org/api/container/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
	//+kubebuilder:scaffold:imports
)

var (
	scheme                = runtime.NewScheme()
	setupLog              = ctrl.Log.WithName("setup")
	enableSliceController = os.Getenv("ENABLE_SLICE_CONTROLLER") == "true"
	enableWebhooks        = os.Getenv("ENABLE_WEBHOOKS") == "true"
)

func init() {
	if enableSliceController {
		utilruntime.Must(v1beta1.AddToScheme(scheme))
	}
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(jobset.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	cfg, err := config.ParseEnv()
	if err != nil {
		setupLog.Error(err, "unable to parse environment variables")
		os.Exit(1)
	}

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if cfg.PodNamespace == "" {
		cfg.PodNamespace = "default"
	}

	var webhookServer webhook.Server
	if enableWebhooks {
		webhookServer = webhook.NewServer(
			webhook.Options{
				Port:    9443,
				CertDir: "/certs",
			},
		)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer:          webhookServer,
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ecaf1259.google.com",
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Node{}: {
					// Only listen for Nodes with label selectors indicating that they
					// are managed by this controller.
					Label: labels.SelectorFromSet(labels.Set{cloud.LabelNodepoolManager: cloud.LabelNodepoolManagerTPUPodinator}),
				},
				// Filter based on ConfigMap name.
				&corev1.ConfigMap{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.name": controller.ConfigMapName}),
					Namespaces: map[string]cache.Config{
						cfg.PodNamespace: {},
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	ctx := ctrl.SetupSignalHandler()

	var provider cloud.Provider
	switch p := strings.ToLower(cfg.Provider); p {
	case "gke":
		if metadata.OnGCE() {
			// Attempt to infer cluster information from GKE metadata server.
			md := metadata.NewClient(&http.Client{})
			var err error

			if cfg.GCPProjectID == "" {
				cfg.GCPProjectID, err = md.ProjectID()
				if err != nil {
					setupLog.Error(err, "fetching project id from metadata server")
					os.Exit(1)
				}
			}
			if cfg.GCPCluster == "" {
				cfg.GCPCluster, err = md.InstanceAttributeValue("cluster-name")
				if err != nil {
					setupLog.Error(err, "fetching cluster name from metadata server")
					os.Exit(1)
				}
			}
			if cfg.GCPClusterLocation == "" {
				cfg.GCPClusterLocation, err = md.InstanceAttributeValue("cluster-location")
				if err != nil {
					setupLog.Error(err, "fetching cluster location from metadata server")
					os.Exit(1)
				}
			}
			if cfg.GCPZone == "" {
				cfg.GCPZone, err = md.Zone()
				if err != nil {
					setupLog.Error(err, "fetching zone from metadata server")
					os.Exit(1)
				}
			}
		}

		setupLog.Info("creating gke client",
			"project", cfg.GCPProjectID,
			"clusterLocation", cfg.GCPClusterLocation,
			"cluster", cfg.GCPCluster,
			"zone", cfg.GCPZone,
			"nodeServiceAccount", cfg.GCPNodeServiceAccount,
			"nodeTags", cfg.GCPNodeTags,
			"podToNodeLabels", cfg.GCPPodToNodeLabels,
		)

		clusterCtx := cloud.GKEContext{
			ProjectID:               cfg.GCPProjectID,
			ClusterLocation:         cfg.GCPClusterLocation,
			Cluster:                 cfg.GCPCluster,
			NodeZone:                cfg.GCPZone,
			NodeServiceAccount:      cfg.GCPNodeServiceAccount,
			NodeAdditionalNetworks:  cfg.GCPNodeAdditionalNetworks,
			NodeSecondaryDisk:       cfg.GCPNodeSecondaryDisk,
			NodeTags:                cfg.GCPNodeTags,
			NodeDiskType:            cfg.GCPNodeDiskType,
			NodeConfidentialStorage: cfg.GCPNodeConfidentialStorage,
			NodeBootDiskKMSKey:      cfg.GCPNodeBootDiskKMSKey,
			PodToNodeLabels:         cfg.GCPPodToNodeLabels,
			NodeSecureBoot:          cfg.GCPNodeSecureBoot,
			ForceOnDemand:           cfg.GCPForceOnDemand,
		}

		containers, err := containerv1beta1.NewService(context.Background() /*, option.WithCredentials(creds)*/)
		if err != nil {
			setupLog.Error(err, "unable to create gke client")
			os.Exit(1)
		}
		nodePoolsService := &cloud.GKENodePoolService{
			ClusterContext: clusterCtx,
			Service:        containers,
		}

		gkeProvider := &cloud.GKE{
			NodePools:      nodePoolsService,
			ClusterContext: clusterCtx,
			Recorder:       mgr.GetEventRecorderFor("tpu-provisioner"),
		}
		provider = gkeProvider
	case "mock":
		provider = &cloud.Mock{}
	default:
		setupLog.Error(err, "unrecognized provider", "provider", p)
		os.Exit(1)
	}

	if enableSliceController {
		recreateConditions := controller.ParseRecreateConditions(cfg.SliceRecreateConditions)
		if err := (&controller.SliceReconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  mgr.GetScheme(),
			Recorder:                mgr.GetEventRecorderFor("tpu-provisioner"),
			RecreateConditions:      recreateConditions,
			ConditionalRecreateWait: cfg.SliceConditionalRecreateWait,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "SliceReconciler")
			os.Exit(1)
		}
	}

	if err := (&controller.CreationReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tpu-provisioner"),
		Provider: provider,
		PodCriteria: controller.PodCriteria{
			ResourceType: cfg.PodResourceType,
		},
		Concurrency: cfg.Concurrency,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CreationReconciler")
		os.Exit(1)
	}

	if err := (&controller.DeletionReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tpu-provisioner"),
		Provider: provider,
		NodeCriteria: controller.NodeCriteria{
			MinLifetime:       cfg.NodeMinLifespan,
			PoolDeletionDelay: cfg.NodepoolDeletionDelay,
		},
		Concurrency: cfg.Concurrency,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DeletionReconciler")
		os.Exit(1)
	}

	if err := (&controller.StaticNodepoolReconciler{
		Client:                          mgr.GetClient(),
		Scheme:                          mgr.GetScheme(),
		Recorder:                        mgr.GetEventRecorderFor("tpu-provisioner"),
		Provider:                        provider,
		StaticNodepoolCreateConcurrency: cfg.StaticNodepoolCreateConcurrency,
		StaticNodepoolDeleteConcurrency: cfg.StaticNodepoolDeleteConcurrency,
		StaticNodepoolCreateTimeout:     cfg.StaticNodepoolCreateTimeout,
		Namespace:                       cfg.PodNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StaticNodepoolReconciler")
		os.Exit(1)
	}

	if enableWebhooks {
		// Register webhook handlers
		jobWebhook := &jobwebhook.JobMutationHandler{
			Decoder: admission.NewDecoder(scheme),
		}
		mgr.GetWebhookServer().Register("/mutate", &webhook.Admission{Handler: jobWebhook})
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	gc := &controller.NodePoolGarbageCollector{
		Interval: time.Minute,
		Client:   mgr.GetClient(),
		Provider: provider,
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		gc.Run(ctx)
		wg.Done()
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	setupLog.Info("waiting for all goroutines to finish")
	wg.Wait()
	setupLog.Info("exiting")
}
