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

package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	// Provider can be "gke" or "mock".
	Provider string `envconfig:"PROVIDER" default:"gke"`

	GCPProjectID          string `envconfig:"GCP_PROJECT_ID"`
	GCPClusterLocation    string `envconfig:"GCP_CLUSTER_LOCATION"`
	GCPZone               string `envconfig:"GCP_ZONE"`
	GCPCluster            string `envconfig:"GCP_CLUSTER"`
	GCPNodeServiceAccount string `envconfig:"GCP_NODE_SERVICE_ACCOUNT"`

	GCPNodeTags               []string `envconfig:"GCP_NODE_TAGS"`
	GCPPodToNodeLabels        []string `envconfig:"GCP_POD_TO_NODE_LABELS"`
	GCPNodeSecondaryDisk      string   `envconfig:"GCP_NODE_SECONDARY_DISK" default:""`
	GCPNodeSecureBoot         bool     `envconfig:"GCP_NODE_SECURE_BOOT" default:"true"`
	GCPNodeAdditionalNetworks string   `envconfig:"GCP_NODE_ADDITIONAL_NETWORKS" default:""`

	GCPNodeDiskType            string `envconfig:"GCP_NODE_DISK_TYPE"`
	GCPNodeConfidentialStorage bool   `envconfig:"GCP_NODE_CONFIDENTIAL_STORAGE"`
	GCPNodeBootDiskKMSKey      string `envconfig:"GCP_NODE_BOOT_DISK_KMS_KEY"`

	// GCPForceOnDemand forces the controller to create nodes on demand, even if
	// the Pod requests a reservation or spot.
	GCPForceOnDemand bool `envconfig:"GCP_FORCE_ON_DEMAND" default:"false"`

	// GKEMaxPodsPerNode sets the max pods per node in provisioned node pools
	GKEMaxPodsPerNode int `envconfig:"GKE_MAX_PODS_PER_NODE" default:"16"`

	// NodeMinLifespan is the amount of time that should pass between a Node object
	// creation and a cleanup of that Node. This is mostly irrelevant now that JobSet
	// existance is checked before deleting a NodePool.
	NodeMinLifespan time.Duration `envconfig:"NODE_MIN_LIFESPAN" default:"10s"`

	NodepoolDeletionDelay time.Duration `envconfig:"NODEPOOL_DELETION_DELAY" default:"30s"`

	PodResourceType string `envconfig:"POD_RESOURCE_TYPE" default:"google.com/tpu"`

	Concurrency int `envconfig:"CONCURRENCY" default:"3"`

	StaticNodepoolCreateConcurrency int           `envconfig:"STATIC_NODEPOOL_CREATE_CONCURRENCY" default:"3"`
	StaticNodepoolCreateTimeout     time.Duration `envconfig:"STATIC_NODEPOOL_CREATE_TIMEOUT" default:"10m"`
	StaticNodepoolDeleteConcurrency int           `envconfig:"STATIC_NODEPOOL_DELETE_CONCURRENCY" default:"3"`

	PodNamespace string `envconfig:"POD_NAMESPACE"`

	SliceRecreateConditions      []string      `envconfig:"SLICE_RECREATE_CONDITIONS"`
	SliceConditionalRecreateWait time.Duration `envconfig:"SLICE_CONDITIONAL_RECREATE_WAIT" default:"60s"`
}

func ParseEnv() (Config, error) {
	cfg := Config{}
	err := envconfig.Process("", &cfg)
	return cfg, err
}
