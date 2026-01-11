package cloud

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	keyPrefix = "google.com/"

	LabelNodepoolManager             = keyPrefix + "nodepool-manager"
	LabelNodepoolManagerTPUPodinator = "tpu-provisioner"

	LabelParentKind      = keyPrefix + "tpu-provisioner-parent-kind"
	LabelParentName      = keyPrefix + "tpu-provisioner-parent-name"
	LabelParentNamespace = keyPrefix + "tpu-provisioner-parent-namespace"

	LabelJobSetName      = keyPrefix + "tpu-provisioner-jobset-name"
	LabelJobSetNamespace = keyPrefix + "tpu-provisioner-jobset-namespace"

	LabelNodePoolHash = keyPrefix + "tpu-provisioner-nodepool-hash"

	LabelProvisionerNodepoolID = "provisioner-nodepool-id"

	// AnnotationCopyLabels is a comma-separated list of labels to copy from the Pod to the node pool config (Nodes).
	AnnotationCopyLabels = "tpu-provisioner.cloud.google.com/copy-labels"
	// AnnotationAdditionalNodeNetworks is a comma-separated list of additional networks and subnets to attach to the node pool.
	// Format: "<network-name>:<subnet-name>, ..."
	AnnotationAdditionalNodeNetworks = "tpu-provisioner.cloud.google.com/additional-node-networks"
	// AnnotatationServiceAccount is the GCP service account to use for the node pool.
	AnnotationNodeServiceAccount = "tpu-provisioner.cloud.google.com/node-service-account"

	EventNodePoolCreationStarted   = "NodePoolCreationStarted"
	EventNodePoolCreationSucceeded = "NodePoolCreationSucceeded"
	EventNodePoolCreationFailed    = "NodePoolCreationFailed"

	EventNodePoolDeletionStarted   = "NodePoolDeletionStarted"
	EventNodePoolDeletionSucceeded = "NodePoolDeletionSucceeded"
	EventNodePoolDeletionFailed    = "NodePoolDeletionFailed"

	EventNodePoolNotFound = "NodePoolNotFound"
)

type StaticNodePoolConfig struct {
	MachineType                 string            `yaml:"machineType"`
	Accelerator                 string            `yaml:"accelerator"`
	Topology                    string            `yaml:"topology"`
	NodeCount                   int               `yaml:"nodeCount"`
	NodeLabels                  map[string]string `yaml:"nodeLabels"`
	ShieldedIntegrityMonitoring *bool             `yaml:"shieldedIntegrityMonitoring"`
	ShieldedSecureBoot          *bool             `yaml:"shieldedSecureBoot"`
	MaxPodsPerNode              int64             `yaml:"maxPodsPerNode"`
	EnableAutoRepair            *bool             `yaml:"enableAutorepair"`
	PlacementPolicy             string            `yaml:"placementPolicy"`

	// NodepoolNameSuffix is a custom suffix for the nodepool name.
	// If not provided, the reservation name is used as the suffix.
	NodepoolNameSuffix string `yaml:"nodepoolNameSuffix"`
}

type Provider interface {
	NodePoolLabelKey() string
	EnsureNodePoolForPod(*corev1.Pod, string) error
	DeleteNodePoolForNode(*corev1.Node, string) error
	DeleteNodePool(string, client.Object, string) error
	ListNodePools() ([]NodePoolRef, error)
	EnsureStaticNodePools(ctx context.Context, reservationName, blockName string, numSubblocks int, config *StaticNodePoolConfig, concurrency int) error
}

var ErrDuplicateRequest = errors.New("duplicate request")

type NodePoolRef struct {
	Name string

	CreationTime time.Time

	CreatedForJobSet types.NamespacedName

	Error   bool
	Message string
}
