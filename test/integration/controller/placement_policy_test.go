package controllertest

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	containerv1beta1 "google.golang.org/api/container/v1beta1"
	"google.golang.org/api/googleapi"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apires "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
)

var _ = Describe("Placement Policy Isolation", func() {
	var ns *corev1.Namespace

	BeforeEach(func() {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-placement-policy-",
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())
		provider.ResetCounters()
	})

	AfterEach(func() {
		Expect(deleteNamespace(context.Background(), k8sClient, ns)).To(Succeed())
	})

	It("should provision a workload without new placement policy change and ensure a subsequent workload with new placement policy changes does not affect the old workload", func() {
		ctx := context.Background()

		gkeSvc := &placementTestGKEService{
			creates:   make(map[string]int),
			deletes:   make(map[string]int),
			nodePools: make(map[string]*containerv1beta1.NodePool),
		}
		gke := &cloud.GKE{
			NodePools: gkeSvc,
			ClusterContext: cloud.GKEContext{
				ProjectID:       "test-project",
				ClusterLocation: "us-central1",
				Cluster:         "test-cluster",
				NodeZone:        "us-central1-a",
				MaxPodsPerNode:  16,
			},
			Recorder: record.NewFakeRecorder(100),
		}

		// 1. Run an existing ("old") workload provisioned before PlacementPolicy was included in nodePoolHash.
		oldJobSetName := "jobset-old-placement"
		oldJobKey := "oldkey"
		oldNodePoolName := "jobset-old-placement-oldke"

		oldJS := makeJobSet(oldJobSetName)
		oldJS.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, oldJS)).To(Succeed())

		oldPod := makePodWithAccelerator(
			"leader-pod-old",
			"0",
			oldJobSetName,
			oldJobKey,
			cloud.V5pPodSliceAccelerator,
			"2x2x2",
		)
		oldPod.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, oldPod)).To(Succeed())
		updatePodStatus(ctx, k8sClient, oldPod, *makePendingStatus())

		By("Verifying node pool creation is triggered for the old workload")
		assertNodePoolCreationTriggered(oldPod)

		// Provision the old workload's node pool and overwrite its hash label with the legacy hash
		// (calculated without PlacementPolicy) to simulate a workload created prior to the change.
		Expect(gke.EnsureNodePoolForPod(oldPod, "initial provisioning for old workload")).To(Succeed())
		oldNP := gkeSvc.nodePools[oldNodePoolName]
		Expect(oldNP).NotTo(BeNil())

		legacyHash, err := legacyNodePoolHashWithoutPlacementPolicy(oldNP)
		Expect(err).NotTo(HaveOccurred())
		Expect(oldNP.Config.Labels[cloud.LabelNodePoolHash]).NotTo(Equal(legacyHash),
			"New hash (with PlacementPolicy) should differ from legacy hash (without PlacementPolicy)")
		oldNP.Config.Labels[cloud.LabelNodePoolHash] = legacyHash

		// Create the corresponding Node and mark the old Pod as scheduled/running so the old workload is active.
		oldNode := makeNodeWithLabels(oldNodePoolName, map[string]string{
			cloud.LabelNodepoolManager: cloud.LabelNodepoolManagerTPUPodinator,
			cloud.GKENodePoolNameLabel: oldNodePoolName,
			cloud.LabelJobSetName:      oldJobSetName,
			cloud.LabelJobSetNamespace: ns.Name,
			cloud.LabelNodePoolHash:    legacyHash,
		})
		Expect(k8sClient.Create(ctx, oldNode)).To(Succeed())
		defer func() {
			Expect(deleteNode(ctx, k8sClient, oldNode)).To(Succeed())
		}()

		updatePodStatus(ctx, k8sClient, oldPod, *makeRunningStatus())

		// 2. Run another workload with the new placement policy hash changes.
		newJobSetName := "jobset-new-placement"
		newJobKey := "newkey"
		newNodePoolName := "jobset-new-placement-newke"

		newJS := makeJobSet(newJobSetName)
		newJS.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, newJS)).To(Succeed())

		newPod := makePodWithAccelerator(
			"leader-pod-new",
			"0",
			newJobSetName,
			newJobKey,
			cloud.V5pPodSliceAccelerator,
			"2x2x2",
		)
		newPod.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, newPod)).To(Succeed())
		updatePodStatus(ctx, k8sClient, newPod, *makePendingStatus())

		By("Verifying node pool creation for the second workload with new placement policy hash")
		assertNodePoolCreationTriggered(newPod)
		Expect(gke.EnsureNodePoolForPod(newPod, "provisioning new workload")).To(Succeed())

		newNP := gkeSvc.nodePools[newNodePoolName]
		Expect(newNP).NotTo(BeNil())
		Expect(newNP.PlacementPolicy).NotTo(BeNil())
		Expect(newNP.PlacementPolicy.TpuTopology).To(Equal("2x2x2"))
		Expect(newNP.PlacementPolicy.Type).To(Equal("COMPACT"))

		newLegacyHash, err := legacyNodePoolHashWithoutPlacementPolicy(newNP)
		Expect(err).NotTo(HaveOccurred())
		Expect(newNP.Config.Labels[cloud.LabelNodePoolHash]).NotTo(Equal(newLegacyHash))

		By("Updating placement policy topology on the second workload and verifying it triggers recreation for the second workload only")
		newPodUpdated := newPod.DeepCopy()
		newPodUpdated.Spec.NodeSelector[cloud.GKETPUNodeSelector] = "2x2x4"
		err = gke.EnsureNodePoolForPod(newPodUpdated, "topology updated on new workload")
		Expect(err).To(MatchError(cloud.ErrNodePoolDeletedToBeRecreated))
		Expect(gkeSvc.deletes[newNodePoolName]).To(Equal(1))

		// Second pass recreates the new workload's node pool with the updated placement policy.
		Expect(gke.EnsureNodePoolForPod(newPodUpdated, "recreating new workload node pool")).To(Succeed())
		Expect(gkeSvc.creates[newNodePoolName]).To(Equal(2))
		Expect(gkeSvc.nodePools[newNodePoolName].PlacementPolicy.TpuTopology).To(Equal("2x2x4"))

		By("Verifying the first (old) workload is not affected")
		oldPodNN := types.NamespacedName{Name: oldPod.Name, Namespace: oldPod.Namespace}
		Expect(provider.getCreated(oldPodNN)).To(BeTrue())
		Expect(gkeSvc.creates[oldNodePoolName]).To(Equal(1), "Old workload node pool should not be recreated")
		Expect(gkeSvc.deletes[oldNodePoolName]).To(Equal(0), "Old workload node pool should not be deleted in GKE")
		Expect(gkeSvc.nodePools[oldNodePoolName]).NotTo(BeNil())
		Expect(gkeSvc.nodePools[oldNodePoolName].Config.Labels[cloud.LabelNodePoolHash]).To(Equal(legacyHash))

		Consistently(func() bool {
			_, deleted := provider.getDeleted(oldNode.Name)
			return deleted
		}, 3*time.Second, interval).Should(BeFalse(), "Existing workload node pool should not be deleted by controller")
	})
})

// legacyNodePoolHashWithoutPlacementPolicy computes the dynamic node pool hash using the
// pre-change structure (where PlacementPolicy was omitted from npToHash).
func legacyNodePoolHashWithoutPlacementPolicy(np *containerv1beta1.NodePool) (string, error) {
	labels := make(map[string]string, len(np.Config.Labels))
	for k, v := range np.Config.Labels {
		if k == cloud.LabelNodePoolHash {
			continue
		}
		labels[k] = v
	}
	npToHash := &containerv1beta1.NodePool{
		Config: &containerv1beta1.NodeConfig{
			Spot:                np.Config.Spot,
			Labels:              labels,
			MachineType:         np.Config.MachineType,
			ReservationAffinity: np.Config.ReservationAffinity,
		},
	}
	jsn, err := json.Marshal(npToHash)
	if err != nil {
		return "", err
	}
	h := fnv.New32a()
	h.Write(jsn)
	return rand.SafeEncodeString(fmt.Sprint(h.Sum32())), nil
}

type placementTestGKEService struct {
	creates   map[string]int
	deletes   map[string]int
	nodePools map[string]*containerv1beta1.NodePool
}

func (g *placementTestGKEService) Get(_ context.Context, name string) (*containerv1beta1.NodePool, error) {
	np, ok := g.nodePools[name]
	if !ok {
		return nil, &googleapi.Error{Code: http.StatusNotFound}
	}
	return np, nil
}

func (g *placementTestGKEService) List(_ context.Context) (*containerv1beta1.ListNodePoolsResponse, error) {
	var resp containerv1beta1.ListNodePoolsResponse
	for _, np := range g.nodePools {
		resp.NodePools = append(resp.NodePools, np)
	}
	return &resp, nil
}

func (g *placementTestGKEService) Create(_ context.Context, req *containerv1beta1.CreateNodePoolRequest, _ cloud.OpCallbacks) error {
	if _, exists := g.nodePools[req.NodePool.Name]; exists {
		return &googleapi.Error{Code: http.StatusConflict}
	}
	g.nodePools[req.NodePool.Name] = req.NodePool
	g.creates[req.NodePool.Name]++
	return nil
}

func (g *placementTestGKEService) Delete(_ context.Context, name string, _ cloud.OpCallbacks) error {
	if _, exists := g.nodePools[name]; !exists {
		return &googleapi.Error{Code: http.StatusNotFound}
	}
	delete(g.nodePools, name)
	g.deletes[name]++
	return nil
}

func makePodWithAccelerator(name, completionIndex, jobsetName, jobKey, accelerator, topology string) *corev1.Pod {
	isController := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"jobset.sigs.k8s.io/jobset-name": jobsetName,
				"jobset.sigs.k8s.io/job-key":     jobKey,
			},
			Annotations: map[string]string{
				"jobset.sigs.k8s.io/jobset-name":     jobsetName,
				batchv1.JobCompletionIndexAnnotation: completionIndex,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "batch/v1",
					Kind:               "Job",
					Name:               jobsetName,
					UID:                types.UID(jobsetName + "-uid"),
					Controller:         &isController,
					BlockOwnerDeletion: &isController,
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				cloud.GKEAcceleratorNodeSelector: accelerator,
				cloud.GKETPUNodeSelector:         topology,
			},
			Containers: []corev1.Container{
				{
					Name:  "container",
					Image: "test-image",
					Resources: corev1.ResourceRequirements{
						Limits: map[corev1.ResourceName]apires.Quantity{
							corev1.ResourceName(resourceName):            apires.MustParse("4"),
							corev1.ResourceName(cloud.GoogleTPUResource): apires.MustParse("4"),
						},
						Requests: map[corev1.ResourceName]apires.Quantity{
							corev1.ResourceName(resourceName):            apires.MustParse("4"),
							corev1.ResourceName(cloud.GoogleTPUResource): apires.MustParse("4"),
						},
					},
				},
			},
		},
	}
}
