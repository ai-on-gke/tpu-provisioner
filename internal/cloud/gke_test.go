package cloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/cmd/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	container "google.golang.org/api/container/v1beta1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestEnsureStaticNodePool(t *testing.T) {
	gke, svc := newTestGKE(t)

	ctx := context.Background()

	// Test np-name nodepool prefix.
	npNameConfig := &StaticNodePoolConfig{
		MachineType: "tpu7x-standard-4t",
		Accelerator: V7xSliceAccelerator,
		Topology:    "4x4x4",
		NodeCount:   16,
	}

	desired1 := []*DesiredStaticNodePool{
		{
			Name:              "np-name-0001",
			SubblockToConsume: "projects/test-project/reservations/res-1/reservationBlocks/np-name-block/reservationSubBlocks/np-name-block-subblock-0001",
			Config:            npNameConfig,
		},
		{
			Name:              "np-name-0002",
			SubblockToConsume: "projects/test-project/reservations/res-1/reservationBlocks/np-name-block/reservationSubBlocks/np-name-block-subblock-0002",
			Config:            npNameConfig,
		},
	}
	// This call should create np-name-0001 and np-name-0002
	if err := gke.EnsureStaticNodePools(ctx, desired1, 1, nil); err != nil {
		t.Fatalf("EnsureStaticNodePools(): %v", err)
	}
	if got := svc.creates["np-name-0001"]; got != 1 {
		t.Errorf("expected 1 create for np-name-0001, got %d", got)
	}
	if got := svc.creates["np-name-0002"]; got != 1 {
		t.Errorf("expected 1 create for np-name-0002, got %d", got)
	}

	if got := len(svc.nodePools); got != 2 { // 2 from npNameConfig
		t.Fatalf("expected 2 node pools, got %d", got)
	}

	np1 := svc.nodePools["np-name-0001"]
	if np1 == nil {
		t.Fatal("nodepool np-name-0001 not found")
	}
	if got, want := np1.Config.Labels[LabelProvisionerNodepoolID], "np-name-0001"; got != want {
		t.Errorf("got label %q, want %q", got, want)
	}

	np2 := svc.nodePools["np-name-0002"]
	if np2 == nil {
		t.Fatal("nodepool np-name-0002 not found")
	}
	if got, want := np2.Config.Labels[LabelProvisionerNodepoolID], "np-name-0002"; got != want {
		t.Errorf("got label %q, want %q", got, want)
	}

	// Test np-prefix nodepool prefix with different block name.
	npPrefixConfig := &StaticNodePoolConfig{
		MachineType: "tpu7x-standard-4t",
		Accelerator: V7xSliceAccelerator,
		Topology:    "4x4x4",
		NodeCount:   16,
	}
	desired2 := []*DesiredStaticNodePool{
		{
			Name:              "np-prefix-0001",
			SubblockToConsume: "projects/test-project/reservations/res-2/reservationBlocks/block-name-ignored/reservationSubBlocks/block-name-ignored-subblock-0001",
			Config:            npPrefixConfig,
		},
		{
			Name:              "np-prefix-0002",
			SubblockToConsume: "projects/test-project/reservations/res-2/reservationBlocks/block-name-ignored/reservationSubBlocks/block-name-ignored-subblock-0002",
			Config:            npPrefixConfig,
		},
	}
	// This call should create np-prefix-0001 and np-prefix-0002
	if err := gke.EnsureStaticNodePools(ctx, desired2, 1, nil); err != nil {
		t.Fatalf("EnsureStaticNodePools(): %v", err)
	}
	if got := svc.creates["np-prefix-0001"]; got != 1 {
		t.Errorf("expected 1 create for np-prefix-0001, got %d", got)
	}
	if got := svc.creates["np-prefix-0002"]; got != 1 {
		t.Errorf("expected 1 create for np-prefix-0002, got %d", got)
	}

	if got := len(svc.nodePools); got != 4 { // 2 from npNameConfig, 2 from this call
		t.Fatalf("expected 4 node pools, got %d", got)
	}

	np3 := svc.nodePools["np-prefix-0001"]
	if np3 == nil {
		t.Fatal("nodepool np-prefix-0001 not found")
	}
	if got, want := np3.Config.Labels[LabelProvisionerNodepoolID], "np-prefix-0001"; got != want {
		t.Errorf("got label %q, want %q", got, want)
	}

	np4 := svc.nodePools["np-prefix-0002"]
	if np4 == nil {
		t.Fatal("nodepool np-prefix-0002 not found")
	}
	if got, want := np4.Config.Labels[LabelProvisionerNodepoolID], "np-prefix-0002"; got != want {
		t.Errorf("got label %q, want %q", got, want)
	}
}

func newTestGKE(t *testing.T) (*GKE, *mockGKEService) {
	t.Helper()
	gkeSvc := &mockGKEService{
		creates:   make(map[string]int),
		deletes:   make(map[string]int),
		nodePools: make(map[string]*container.NodePool),
	}
	clusterCtx := GKEContext{
		ProjectID:              "test-project",
		MaxPodsPerNode:         config.DefaultGKEMaxPodsPerNode,
		ClusterLocation:        "us-east5",
		Cluster:                "test-cluster",
		NodeZone:               "us-east5-a",
		NodeServiceAccount:     "test-sa@test-project.iam.gserviceaccount.com",
		NodeAdditionalNetworks: "",
		NodeSecondaryDisk:      "test-disk",
		NodeTags:               []string{"foo", "bar"},
		PodToNodeLabels:        nil,
		NodeSecureBoot:         true,
		ForceOnDemand:          false,
	}
	rec := &mockEventRecorder{}
	gke := &GKE{
		NodePools:      gkeSvc,
		ClusterContext: clusterCtx,
		Recorder:       rec,
	}
	return gke, gkeSvc
}

func TestEnsureNodePoolForPod(t *testing.T) {
	gke, svc := newTestGKE(t)

	cases := []struct {
		name          string
		pod           podBuild
		expNPCreation bool
		expNPDeletion bool
	}{
		{
			name:          "simple creation",
			pod:           podBuild{},
			expNPCreation: true,
		},
		{
			name:          "duplicate pod",
			pod:           podBuild{},
			expNPCreation: false,
		},
		{
			name: "same pod - with spot now",
			pod: podBuild{
				additionalSelector: map[string]string{
					"cloud.google.com/gke-spot": "true",
				},
			},
			expNPCreation: false,
			expNPDeletion: true,
		},
		{
			name: "same pod - with spot - 2nd pass",
			pod: podBuild{
				additionalSelector: map[string]string{
					"cloud.google.com/gke-spot": "true",
				},
			},
			expNPCreation: true,
			expNPDeletion: false,
		},
		{
			name: "different jobset",
			pod: podBuild{
				jobsetNameSuffix: "-2",
			},
			expNPCreation: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pod := buildPod(c.pod)
			npName, err := podToNodePoolName(pod)
			if err != nil {
				t.Fatalf("podToNodePoolName(): %v", err)
			}

			createsBefore := svc.creates[npName]
			deletesBefore := svc.deletes[npName]

			err = gke.EnsureNodePoolForPod(pod, "test")
			if c.expNPDeletion {
				if !errors.Is(err, ErrNodePoolDeletedToBeRecreated) {
					t.Fatalf("expected ErrNodePoolDeletedToBeRecreated, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("EnsureNodePoolForPod(%v): %v", pod.Name, err)
				}
			}

			createsAfter := svc.creates[npName]
			deletesAfter := svc.deletes[npName]

			if c.expNPCreation {
				if createsAfter-createsBefore != 1 {
					t.Fatalf("expected create for node pool %q, got none", npName)
				}
			}
			if c.expNPDeletion {
				if deletesAfter-deletesBefore != 1 {
					t.Fatalf("expected delete for node pool %q, got none", npName)
				}
			}
		})
	}
}

type mockEventRecorder struct{}

func (r *mockEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {}
func (r *mockEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}
func (r *mockEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}

type mockGKEService struct {
	creates   map[string]int
	deletes   map[string]int
	nodePools map[string]*container.NodePool
}

func (g *mockGKEService) Get(ctx context.Context, name string) (*container.NodePool, error) {
	np, ok := g.nodePools[name]
	if !ok {
		return nil, &googleapi.Error{
			Code: http.StatusNotFound,
		}
	}
	return np, nil
}

func (g *mockGKEService) List(ctx context.Context) (*container.ListNodePoolsResponse, error) {
	var resp container.ListNodePoolsResponse
	for _, np := range g.nodePools {
		resp.NodePools = append(resp.NodePools, np)
	}
	return &resp, nil
}

func (g *mockGKEService) Create(ctx context.Context, req *container.CreateNodePoolRequest, callbacks OpCallbacks) error {
	_, alreadyExists := g.nodePools[req.NodePool.Name]
	if alreadyExists {
		return &googleapi.Error{
			Code: http.StatusConflict,
		}
	}
	g.nodePools[req.NodePool.Name] = req.NodePool
	g.creates[req.NodePool.Name]++
	return nil
}

func (g *mockGKEService) Delete(ctx context.Context, name string, callbacks OpCallbacks) error {
	_, ok := g.nodePools[name]
	if !ok {
		return &googleapi.Error{
			Code: http.StatusNotFound,
		}
	}
	delete(g.nodePools, name)
	g.deletes[name]++
	return nil
}

func Test_tpuTopologyToNodeCount(t *testing.T) {
	cases := []struct {
		accel string
		topo  string
		count int
		err   bool
	}{
		{
			accel: "tpu-v4-podslice",
			topo:  "2x2x1",
			count: 1,
		},
		{
			accel: "tpu-v4-podslice",
			topo:  "2x2x2",
			count: 2,
		},
		{
			accel: "tpu-v5p-slice",
			topo:  "2x2x2",
			count: 2,
		},
		{
			accel: "tpu-v4-podslice",
			topo:  "2x2x4",
			count: 4,
		},
		{
			accel: "tpu-v5p-slice",
			topo:  "2x2x4",
			count: 4,
		},
		{
			accel: "tpu-v4-podslice",
			topo:  "2x4x4",
			count: 8,
		},
		{
			accel: "tpu-v5-lite-podslice",
			topo:  "1x1",
			count: 1,
		},
		{
			accel: "tpu-v5-lite-podslice",
			topo:  "2x4",
			count: 2,
		},
		{
			accel: "tpu-v6e-slice",
			topo:  "1x1",
			count: 1,
		},
		{
			accel: "tpu-v6e-slice",
			topo:  "2x2",
			count: 1,
		},
		{
			accel: "tpu-v6e-slice",
			topo:  "2x4",
			count: 2,
		},
		{
			accel: "tpu-v6e-slice",
			topo:  "4x4",
			count: 4,
		},
		{
			accel: "not-an-accel",
			topo:  "2x4",
			err:   true,
		},
		{
			accel: "tpu-v4-podslice",
			topo:  "not-a-topo",
			err:   true,
		},
		{
			accel: "tpu-v6e-slice",
			topo:  "16x16",
			count: 64,
		},
		{
			accel: "tpu-v6e-slice",
			topo:  "1x1x1",
			err:   true,
		},
		{
			accel: "tpu7x",
			topo:  "1x2x2",
			count: 1,
		},
		{
			accel: "tpu7x",
			topo:  "2x2x2",
			count: 2,
		},
		{
			accel: "tpu7x",
			topo:  "2x2x4",
			count: 4,
		},
		{
			accel: "tpu7x",
			topo:  "2x4x4",
			count: 8,
		},
		{
			accel: "tpu7x",
			topo:  "4x4x4",
			count: 16,
		},
		{
			accel: "tpu7x",
			topo:  "4x4x8",
			count: 32,
		},
		{
			accel: "tpu7x",
			topo:  "8x8x8",
			count: 128,
		},
		{
			accel: "tpu7x",
			topo:  "8x8x16",
			count: 256,
		},
	}

	for _, c := range cases {
		t.Run(c.accel+"_"+c.topo, func(t *testing.T) {
			count, err := tpuTopologyToNodeCount(c.accel, c.topo)
			if (err != nil) != c.err {
				t.Fatalf("error: expected: %v", c.err)
			}
			if exp, got := c.count, count; exp != got {
				t.Fatalf("count: expected: %v, got: %v", exp, got)
			}
		})
	}
}

func Test_tpuMachineType(t *testing.T) {
	cases := []struct {
		accel       string
		tpuRequest  int
		machineType string
		err         bool
	}{
		{
			accel:       "tpu-v4-podslice",
			tpuRequest:  4,
			machineType: "ct4p-hightpu-4t",
		},
		{
			accel:       "tpu-v5-lite-podslice",
			tpuRequest:  1,
			machineType: "ct5lp-hightpu-1t",
		},
		{
			accel:       "tpu-v5-lite-podslice",
			tpuRequest:  4,
			machineType: "ct5lp-hightpu-4t",
		},
		{
			accel:       "tpu-v5-lite-podslice",
			tpuRequest:  8,
			machineType: "ct5lp-hightpu-8t",
		},
		{
			accel:       "tpu-v5p-slice",
			tpuRequest:  4,
			machineType: "ct5p-hightpu-4t",
		},
		{
			accel:       "tpu-v6e-slice",
			tpuRequest:  4,
			machineType: "ct6e-standard-4t",
		},
		{
			accel:       "tpu-v6e-slice",
			tpuRequest:  8,
			machineType: "ct6e-standard-8t",
		},
		{
			accel:      "not-an-accel",
			tpuRequest: 4,
			err:        true,
		},
		{
			accel:      "tpu-v5p-slice",
			tpuRequest: -1,
			err:        true,
		},
		{
			accel:       "tpu7x",
			tpuRequest:  4,
			machineType: "tpu7x-standard-4t",
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%v_accel_%v_tpus", c.accel, c.tpuRequest), func(t *testing.T) {
			machineType, err := tpuMachineType(c.accel, c.tpuRequest)
			if (err != nil) != c.err {
				t.Fatalf("error: expected: %v", c.err)
			}
			if exp, got := c.machineType, machineType; exp != got {
				t.Fatalf("machineType: expected: %v, got: %v", exp, got)
			}
		})
	}
}

func TestPodToNodePoolName(t *testing.T) {
	var jobKey = "759730a97e4373f3a0ee12805db065e3a4a649a5"

	testCases := []struct {
		name          string
		pod           *corev1.Pod
		expectedName  string
		expectedError bool
	}{
		{
			name: "Missing JobSetName label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Labels: map[string]string{
						jobset.JobKey: jobKey,
					},
				},
			},
			expectedError: true,
		},
		{
			name: "Missing JobKey label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Labels: map[string]string{
						jobset.JobSetNameKey: "some-job-set-name",
					},
				},
			},
			expectedError: true,
		},
		{
			name: "jobset name less than 34 chars",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Labels: map[string]string{
						jobset.JobSetNameKey: "myjobset",
						jobset.JobKey:        jobKey,
					},
				},
			},
			expectedName: fmt.Sprintf("myjobset-%s", jobKey[:jobKeySuffixLength]),
		},
		{
			name: "jobset name more than 34 chars",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Labels: map[string]string{
						jobset.JobSetNameKey: "myjobset-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						jobset.JobKey:        jobKey,
					},
				},
			},
			expectedName: fmt.Sprintf("%s-%s", "myjobset-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"[:maxJobSetPrefixLength], jobKey[:jobKeySuffixLength]),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := podToNodePoolName(tc.pod)

			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tc.expectedName {
				t.Errorf("Expected node pool name %s, got %s", tc.expectedName, result)
			}
		})
	}
}

func TestNodePoolForPod(t *testing.T) {
	tests := []struct {
		desc       string
		gkeContext GKEContext
		pod        podBuild
		want       *container.NodePool
	}{
		{
			desc: "simple case",
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "simple case 1x1 topology",
			pod: podBuild{
				selector: map[string]string{
					"cloud.google.com/gke-tpu-accelerator": "tpu-v5-lite-podslice",
					"cloud.google.com/gke-tpu-topology":    "1x1",
				},
				tpuResource: "1",
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5lp-hightpu-1t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  1,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				PlacementPolicy:   &container.PlacementPolicy{},
				Name:              "jobset-test-rando",
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "2x2 topology should have no placement policy",
			pod: podBuild{
				selector: map[string]string{
					"cloud.google.com/gke-tpu-accelerator": "tpu-v6e-slice",
					"cloud.google.com/gke-tpu-topology":    "2x2",
				},
				tpuResource: "4",
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct6e-standard-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  1,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				PlacementPolicy:   &container.PlacementPolicy{},
				Name:              "jobset-test-rando",
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "4x4 topology should have placement policy",
			pod: podBuild{
				selector: map[string]string{
					"cloud.google.com/gke-tpu-accelerator": "tpu-v6e-slice",
					"cloud.google.com/gke-tpu-topology":    "4x4",
				},
				tpuResource: "16",
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct6e-standard-16t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  4,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				PlacementPolicy: &container.PlacementPolicy{
					TpuTopology: "4x4",
					Type:        "COMPACT",
				},
				Name:            "jobset-test-rando",
				UpgradeSettings: &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "spot",
			pod: podBuild{
				additionalSelector: map[string]string{
					"cloud.google.com/gke-spot": "true",
				},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
					Spot:                   true,
					Taints: []*container.NodeTaint{
						{Effect: "NO_SCHEDULE", Key: "cloud.google.com/gke-spot", Value: "true"},
					},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc:       "spot with forced on demand",
			gkeContext: GKEContext{ForceOnDemand: true},
			pod: podBuild{
				additionalSelector: map[string]string{
					"cloud.google.com/gke-spot": "true",
				},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
					Spot:                   false,
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "pod with reservation selector",
			pod: podBuild{
				additionalSelector: map[string]string{"cloud.google.com/reservation-name": "tpu-rsv"},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType: "ct5p-hightpu-4t",
					ReservationAffinity: &container.ReservationAffinity{
						ConsumeReservationType: "SPECIFIC_RESERVATION",
						Key:                    "compute.googleapis.com/reservation-name",
						Values:                 []string{"tpu-rsv"},
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "pod with cross-project reservation selector",
			pod: podBuild{
				additionalSelector: map[string]string{
					"cloud.google.com/reservation-name":    "tpu-rsv",
					"cloud.google.com/reservation-project": "tpu-rsv-project",
				},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType: "ct5p-hightpu-4t",
					ReservationAffinity: &container.ReservationAffinity{
						ConsumeReservationType: "SPECIFIC_RESERVATION",
						Key:                    "compute.googleapis.com/reservation-name",
						Values:                 []string{"projects/tpu-rsv-project/reservations/tpu-rsv"},
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "pod with reservation selector but on demand is forced",
			pod: podBuild{
				additionalSelector: map[string]string{"cloud.google.com/reservation-name": "tpu-rsv"},
			},
			gkeContext: GKEContext{ForceOnDemand: true},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ReservationAffinity:    nil,
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "pod with disabling ICI resiliency selector",
			pod: podBuild{
				additionalSelector: map[string]string{"cloud.google.com/gke-tpu-ici-resiliency": "false"},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
						"cloud.google.com/gke-tpu-ici-resiliency":     "false",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				}, InitialNodeCount: 512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc:       "pod with secondary boot disk",
			gkeContext: GKEContext{NodeSecondaryDisk: "projects/my-gcp-project/global/images/my-disk-image"},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
					SecondaryBootDisks: []*container.SecondaryBootDisk{
						{
							DiskImage: "projects/my-gcp-project/global/images/my-disk-image",
							Mode:      "CONTAINER_IMAGE_CACHE",
						},
					},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "pod with location hint node selector",
			pod: podBuild{
				additionalSelector: map[string]string{"cloud.google.com/gke-location-hint": "test-location-hint"},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
						"cloud.google.com/gke-location-hint":          "test-location-hint",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "labels to copy from pod to node",
			gkeContext: GKEContext{
				PodToNodeLabels: []string{"should-be-copied"},
			},
			pod: podBuild{
				additionalLabels: map[string]string{
					"should-be-copied":     "val-a",
					"should-not-be-copied": "val-b",
				},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
						"should-be-copied":                            "val-a",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "labels to copy from pod to node by annotation",
			pod: podBuild{
				additionalLabels: map[string]string{
					"copy-me":      "val-x",
					"dont-copy-me": "val-y",
				},
				additionalAnnotations: map[string]string{
					"tpu-provisioner.cloud.google.com/copy-labels": "copy-me",
				},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
						"copy-me": "val-x",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
		{
			desc: "additional node networks configured in cluster context",
			gkeContext: GKEContext{
				NodeAdditionalNetworks: "network-1:subnet-1, network-2:subnet-2",
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
				NetworkConfig: &container.NodeNetworkConfig{
					AdditionalNodeNetworkConfigs: []*container.AdditionalNodeNetworkConfig{
						{
							Network:    "network-1",
							Subnetwork: "subnet-1",
						},
						{
							Network:    "network-2",
							Subnetwork: "subnet-2",
						},
					},
				},
			},
		},
		{
			desc: "pod requesting additional node networks",
			gkeContext: GKEContext{
				NodeAdditionalNetworks: "should-be-overriden-1:should-be-overriden-2",
			},
			pod: podBuild{
				additionalAnnotations: map[string]string{
					"tpu-provisioner.cloud.google.com/additional-node-networks": "network-1:subnet-1, network-2:subnet-2",
				},
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:            "ct5p-hightpu-4t",
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
				NetworkConfig: &container.NodeNetworkConfig{
					AdditionalNodeNetworkConfigs: []*container.AdditionalNodeNetworkConfig{
						{
							Network:    "network-1",
							Subnetwork: "subnet-1",
						},
						{
							Network:    "network-2",
							Subnetwork: "subnet-2",
						},
					},
				},
			},
		},
		{
			desc: "confidential disk configured in cluster context",
			gkeContext: GKEContext{
				NodeConfidentialStorage: true,
				NodeDiskType:            "hyperdisk-balanced",
				NodeBootDiskKMSKey:      "my-kms-key",
			},
			want: &container.NodePool{
				Config: &container.NodeConfig{
					Labels: map[string]string{
						"google.com/nodepool-manager":                 "tpu-provisioner",
						"google.com/tpu-provisioner-jobset-name":      "jobset-test",
						"google.com/tpu-provisioner-jobset-namespace": "default",
						"google.com/tpu-provisioner-parent-kind":      "job",
						"google.com/tpu-provisioner-parent-name":      "jobset-test-job-1-0",
						"google.com/tpu-provisioner-parent-namespace": "default",
					},
					MachineType:               "ct5p-hightpu-4t",
					ShieldedInstanceConfig:    &container.ShieldedInstanceConfig{EnableIntegrityMonitoring: true},
					EnableConfidentialStorage: true,
					BootDiskKmsKey:            "my-kms-key",
					DiskType:                  "hyperdisk-balanced",
				},
				InitialNodeCount:  512,
				Locations:         []string{""},
				Management:        &container.NodeManagement{AutoRepair: true, AutoUpgrade: false},
				MaxPodsConstraint: &container.MaxPodsConstraint{MaxPodsPerNode: config.DefaultGKEMaxPodsPerNode},
				Name:              "jobset-test-rando",
				PlacementPolicy:   &container.PlacementPolicy{TpuTopology: "8x16x16", Type: "COMPACT"},
				UpgradeSettings:   &container.UpgradeSettings{MaxSurge: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.gkeContext.MaxPodsPerNode == 0 {
				tc.gkeContext.MaxPodsPerNode = config.DefaultGKEMaxPodsPerNode
			}
			gke := &GKE{
				ClusterContext: tc.gkeContext,
			}
			pod := buildPod(tc.pod)
			got, err := gke.nodePoolForPod(pod)
			if err != nil {
				t.Errorf("Got error: %v", err)
			}

			// Populating a hash in test cases is a hassle, so we will just check for existance.
			gotHash := got.Config.Labels[LabelNodePoolHash]
			t.Logf("Node pool hash: %s", gotHash)
			if gotHash == "" {
				t.Errorf("Node pool hash should be populated")
			}
			delete(got.Config.Labels, LabelNodePoolHash)

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TestNodePoolForPod() return unexpected node pool, diff (-want +got): \n%s", diff)
			}
		})
	}
}

type podBuild struct {
	jobsetNameSuffix      string
	additionalLabels      map[string]string
	additionalAnnotations map[string]string
	selector              map[string]string
	additionalSelector    map[string]string
	tpuResource           string
}

func buildPod(b podBuild) *corev1.Pod {
	trueVar := true

	if b.selector == nil {
		b.selector = map[string]string{
			"cloud.google.com/gke-tpu-accelerator": "tpu-v5p-slice",
			"cloud.google.com/gke-tpu-topology":    "8x16x16",
		}
	}

	if b.tpuResource == "" {
		b.tpuResource = "4"
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"alpha.jobset.sigs.k8s.io/exclusive-topology": "cloud.google.com/gke-nodepool",
				"batch.kubernetes.io/job-completion-index":    "0",
				"jobset.sigs.k8s.io/job-index":                "0",
				"jobset.sigs.k8s.io/job-key":                  "random-key",
				"jobset.sigs.k8s.io/jobset-name":              "jobset-test" + b.jobsetNameSuffix,
				"jobset.sigs.k8s.io/replicatedjob-name":       "job-1",
				"jobset.sigs.k8s.io/replicatedjob-replicas":   "1",
				"jobset.sigs.k8s.io/restart-attempt":          "0",
			},
			Labels: map[string]string{
				"batch.kubernetes.io/controller-uid":        "8484279a-de52-4ca1-b01e-130fbded30fb",
				"batch.kubernetes.io/job-name":              "jobset-test-job-1-0",
				"controller-uid":                            "8484279a-de52-4ca1-b01e-130fbded30fb",
				"job-name":                                  "jobset-test-job-1-0",
				"jobset.sigs.k8s.io/job-index":              "0",
				"jobset.sigs.k8s.io/job-key":                "random-key",
				"jobset.sigs.k8s.io/jobset-name":            "jobset-test" + b.jobsetNameSuffix,
				"jobset.sigs.k8s.io/replicatedjob-name":     "job-1",
				"jobset.sigs.k8s.io/replicatedjob-replicas": "1",
				"jobset.sigs.k8s.io/restart-attempt":        "0",
			},
			Finalizers: []string{"batch.kubernetes.io/job-tracking"},
			Name:       "job-test-6gfwq",
			Namespace:  "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "batch/v1",
					Kind:               "Job",
					UID:                "8484279a-de52-4ca1-b01e-130fbded30fb",
					Name:               "jobset-test-job-1-0",
					Controller:         &trueVar,
					BlockOwnerDeletion: &trueVar,
				},
			},
			GenerateName:    "jobset-test-job-1-0-0-",
			ResourceVersion: "70731715",
			UID:             "f6a99195-268e-4b68-91de-22e75f9100bc",
		},
		Spec: corev1.PodSpec{
			NodeSelector: b.selector,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"google.com/tpu": resource.MustParse(b.tpuResource),
						},
						Limits: corev1.ResourceList{
							"google.com/tpu": resource.MustParse(b.tpuResource),
						},
					},
				},
			},
		},
	}

	for k, v := range b.additionalAnnotations {
		pod.Annotations[k] = v
	}
	for k, v := range b.additionalLabels {
		pod.Labels[k] = v
	}
	for k, v := range b.additionalSelector {
		pod.Spec.NodeSelector[k] = v
	}

	return pod
}

func Test_nodePoolSelectiveHash(t *testing.T) {
	cases := []struct {
		name        string
		A           *container.NodePool
		B           *container.NodePool
		expSameHash bool
	}{
		{
			name:        "two empty",
			A:           &container.NodePool{Config: &container.NodeConfig{}},
			B:           &container.NodePool{Config: &container.NodeConfig{}},
			expSameHash: true,
		},
		{
			name: "different machine type",
			A: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "ct5p-hightpu-4t",
				},
			},
			B: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "ct5p-hightpu-8t",
				},
			},
			expSameHash: false,
		},
		{
			name: "different labels",
			A: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "ct5p-hightpu-4t",
					Labels: map[string]string{
						"a": "b",
					},
				},
			},
			B: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "ct5p-hightpu-4t",
					Labels: map[string]string{
						"a": "c",
					},
				},
			},
			expSameHash: false,
		},
		{
			name: "different label order for static nodepool",
			A: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "tpu7x-standard-4t",
					Labels: map[string]string{
						LabelTPUProvisionerStaticNodepool: "true",
						"a":                               "b",
						"c":                               "d",
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{},
				},
				PlacementPolicy: &container.PlacementPolicy{},
			},
			B: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "tpu7x-standard-4t",
					Labels: map[string]string{
						LabelTPUProvisionerStaticNodepool: "true",
						"c":                               "d",
						"a":                               "b",
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{},
				},
				PlacementPolicy: &container.PlacementPolicy{},
			},
			expSameHash: true,
		},
		{
			name: "different label order for static nodepool",
			A: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "tpu7x-standard-4t",
					Labels: map[string]string{
						LabelTPUProvisionerStaticNodepool: "true",
						"a":                               "b",
						"c":                               "d",
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{},
				},
				PlacementPolicy: &container.PlacementPolicy{},
			},
			B: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "tpu7x-standard-4t",
					Labels: map[string]string{
						LabelTPUProvisionerStaticNodepool: "true",
						"c":                               "d",
						"a":                               "b",
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{},
				},
				PlacementPolicy: &container.PlacementPolicy{},
			},
			expSameHash: true,
		},
		{
			name: "non hashed upgrade settings",
			A: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "ct5p-hightpu-4t",
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
				UpgradeSettings: &container.UpgradeSettings{
					MaxSurge: 1,
				},
			},
			B: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "ct5p-hightpu-4t",
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
				UpgradeSettings: &container.UpgradeSettings{
					MaxSurge: 2,
				},
			},
			expSameHash: true,
		},
		{
			name: "different placement policy for static nodepool",
			A: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "tpu7x-standard-4t",
					Labels: map[string]string{
						LabelTPUProvisionerStaticNodepool: "true",
						"a":                               "b",
						"c":                               "d",
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{},
				},
				PlacementPolicy: &container.PlacementPolicy{
					PolicyName: "policy-a",
				},
			},
			B: &container.NodePool{
				Config: &container.NodeConfig{
					MachineType: "tpu7x-standard-4t",
					Labels: map[string]string{
						LabelTPUProvisionerStaticNodepool: "true",
						"a":                               "b",
						"c":                               "d",
					},
					ShieldedInstanceConfig: &container.ShieldedInstanceConfig{},
				},
				PlacementPolicy: &container.PlacementPolicy{
					PolicyName: "policy-b",
				},
			},
			expSameHash: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hashA, err := nodePoolHash(c.A)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			hashB, err := nodePoolHash(c.B)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if c.expSameHash {
				if hashA != hashB {
					t.Errorf("Expected same hash, got %s and %s", hashA, hashB)
				}
			} else {
				if hashA == hashB {
					t.Errorf("Expected different hash, got %s", hashA)
				}
			}
		})
	}
}

func TestParseAdditionalNodeNetworks(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expected      []*container.AdditionalNodeNetworkConfig
		expectedError bool
	}{
		{
			name:          "empty string",
			input:         "",
			expected:      nil,
			expectedError: false,
		},
		{
			name:  "single network",
			input: "vpc1:subnet1",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "vpc1", Subnetwork: "subnet1"},
			},
			expectedError: false,
		},
		{
			name:  "multiple networks",
			input: "vpc1:subnet1,vpc2:subnet2",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "vpc1", Subnetwork: "subnet1"},
				{Network: "vpc2", Subnetwork: "subnet2"},
			},
			expectedError: false,
		},
		{
			name:  "with whitespace",
			input: "  vpc1:subnet1,  vpc2:subnet2  ",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "vpc1", Subnetwork: "subnet1"},
				{Network: "vpc2", Subnetwork: "subnet2"},
			},
			expectedError: false,
		},
		{
			name:          "invalid format",
			input:         "vpc1subnet1",
			expected:      nil,
			expectedError: true,
		},
		{
			name:  "missing subnet",
			input: "vpc1:",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "vpc1", Subnetwork: ""},
			},
			expectedError: false,
		},
		{
			name:  "missing vpc",
			input: ":subnet1",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "", Subnetwork: "subnet1"},
			},
			expectedError: false,
		},
		{
			name:          "just a comma",
			input:         ",",
			expected:      nil,
			expectedError: false,
		},
		{
			name:  "trailing comma",
			input: "vpc1:subnet1,",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "vpc1", Subnetwork: "subnet1"},
			},
			expectedError: false,
		},
		{
			name:  "leading comma",
			input: ",vpc1:subnet1",
			expected: []*container.AdditionalNodeNetworkConfig{
				{Network: "vpc1", Subnetwork: "subnet1"},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseAdditionalNodeNetworks(tc.input)
			if (err != nil) != tc.expectedError {
				t.Fatalf("parseAdditionalNodeNetworks() error = %v, wantErr %v", err, tc.expectedError)
			}
			if diff := cmp.Diff(tc.expected, result); diff != "" {
				t.Errorf("parseAdditionalNodeNetworks() returned diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseSubBlocks(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expectedStart int
		expectedEnd   int
		expectedError bool
	}{
		{
			name:          "valid range",
			input:         "1-10",
			expectedStart: 1,
			expectedEnd:   10,
			expectedError: false,
		},
		{
			name:          "single subblock",
			input:         "5",
			expectedStart: 5,
			expectedEnd:   5,
			expectedError: false,
		},
		{
			name:          "single subblock with leading zeros",
			input:         "0005",
			expectedStart: 5,
			expectedEnd:   5,
			expectedError: false,
		},
		{
			name:          "invalid range, start > end",
			input:         "10-1",
			expectedError: true,
		},
		{
			name:          "invalid single subblock, not a number",
			input:         "abc",
			expectedError: true,
		},
		{
			name:          "invalid range, not numbers",
			input:         "a-b",
			expectedError: true,
		},
		{
			name:          "empty string",
			input:         "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := ParseSubBlocks(tc.input)
			if (err != nil) != tc.expectedError {
				t.Fatalf("ParseSubBlocks() error = %v, wantErr %v", err, tc.expectedError)
			}
			if !tc.expectedError {
				if start != tc.expectedStart {
					t.Errorf("ParseSubBlocks() start = %v, want %v", start, tc.expectedStart)
				}
				if end != tc.expectedEnd {
					t.Errorf("ParseSubBlocks() end = %v, want %v", end, tc.expectedEnd)
				}
			}
		})
	}
}

func TestDiffStaticNodePools(t *testing.T) {
	gke, _ := newTestGKE(t)

	// Helper to create a DesiredStaticNodePool and its expected hash
	createDesired := func(name string, machineType string) (*DesiredStaticNodePool, string) {
		config := &StaticNodePoolConfig{
			MachineType: machineType,
			Accelerator: "tpu-v5p-slice",
			Topology:    "2x2x2",
			NodeCount:   2,
			NodeLabels:  map[string]string{"foo": "bar"},
		}
		desired := &DesiredStaticNodePool{
			Name:              name,
			SubblockToConsume: "projects/test-project/reservations/res-1/reservationBlocks/block-1/reservationSubBlocks/" + name,
			Config:            config,
		}
		// Calculate expected hash
		np, err := gke.StaticNodePoolForSubBlock(name, desired.SubblockToConsume, config)
		if err != nil {
			t.Fatalf("failed to create node pool for test helper: %v", err)
		}
		hash, ok := np.Config.Labels[LabelNodePoolHash]
		if !ok {
			t.Fatalf("hash not found in test helper")
		}
		return desired, hash
	}

	desiredA, hashA := createDesired("pool-a", "ct5p-hightpu-4t")
	desiredB, _ := createDesired("pool-b", "ct5p-hightpu-4t")
	desiredAUpdated, hashAUpdated := createDesired("pool-a", "ct5p-hightpu-8t") // Different machine type

	if hashA == hashAUpdated {
		t.Fatalf("hashes should strictly differ for different machine types")
	}

	tests := []struct {
		name              string
		existing          []NodePoolRef
		desired           []*DesiredStaticNodePool
		wantCreate        []string
		wantDeleteMissing []string
		wantDeleteUpdate  []string
		wantDeleteError   []string
	}{
		{
			name:       "Create New Nodepool",
			existing:   []NodePoolRef{},
			desired:    []*DesiredStaticNodePool{desiredA},
			wantCreate: []string{"pool-a"},
		},
		{
			name: "No Change",
			existing: []NodePoolRef{
				{Name: "pool-a", Labels: map[string]string{LabelNodePoolHash: hashA, LabelTPUProvisionerStaticNodepool: "true"}},
			},
			desired: []*DesiredStaticNodePool{desiredA},
		},
		{
			name: "Delete Missing",
			existing: []NodePoolRef{
				{Name: "pool-a", Labels: map[string]string{LabelNodePoolHash: hashA, LabelTPUProvisionerStaticNodepool: "true"}},
			},
			desired:           []*DesiredStaticNodePool{},
			wantDeleteMissing: []string{"pool-a"},
		},
		{
			name: "Delete Update (Hash Mismatch)",
			existing: []NodePoolRef{
				{Name: "pool-a", Labels: map[string]string{LabelNodePoolHash: hashA, LabelTPUProvisionerStaticNodepool: "true"}},
			},
			desired:          []*DesiredStaticNodePool{desiredAUpdated},
			wantCreate:       []string{"pool-a"},
			wantDeleteUpdate: []string{"pool-a"},
		},
		{
			name: "Legacy Nodepool (No Hash)",
			existing: []NodePoolRef{
				{Name: "pool-a", Labels: map[string]string{LabelTPUProvisionerStaticNodepool: "true"}},
			},
			desired: []*DesiredStaticNodePool{desiredA},
		},
		{
			name: "Non-Static Nodepool (Ignored)",
			existing: []NodePoolRef{
				{Name: "pool-b", Labels: map[string]string{}},
			},
			desired: []*DesiredStaticNodePool{},
		},
		{
			name: "Multiple Actions",
			existing: []NodePoolRef{
				{Name: "pool-a", Labels: map[string]string{LabelNodePoolHash: hashA, LabelTPUProvisionerStaticNodepool: "true"}},       // Unchanged
				{Name: "pool-b", Labels: map[string]string{LabelNodePoolHash: "old-hash", LabelTPUProvisionerStaticNodepool: "true"}},  // Update
				{Name: "pool-c", Labels: map[string]string{LabelNodePoolHash: "some-hash", LabelTPUProvisionerStaticNodepool: "true"}}, // Delete
			},
			desired: []*DesiredStaticNodePool{
				desiredA,
				desiredB, // pool-b exists but with hashB vs old-hash
			},
			wantCreate:        []string{"pool-b"},
			wantDeleteMissing: []string{"pool-c"},
			wantDeleteUpdate:  []string{"pool-b"},
		},
		{
			name: "Delete Error (Retry)",
			existing: []NodePoolRef{
				{
					Name:   "pool-a",
					Labels: map[string]string{LabelNodePoolHash: hashA, LabelTPUProvisionerStaticNodepool: "true"},
					Error:  true,
				},
			},
			desired:         []*DesiredStaticNodePool{desiredA},
			wantCreate:      []string{"pool-a"},
			wantDeleteError: []string{"pool-a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			toCreate, toDeleteMissing, toDeleteUpdate, toDeleteError, err := gke.DiffStaticNodePools(tc.existing, tc.desired)
			if err != nil {
				t.Fatalf("DiffStaticNodePools() error = %v", err)
			}

			var gotCreate []string
			for _, np := range toCreate {
				gotCreate = append(gotCreate, np.Name)
			}
			sort.Strings(gotCreate)
			sort.Strings(tc.wantCreate)
			if diff := cmp.Diff(tc.wantCreate, gotCreate, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("toCreate mismatch (-want +got):\n%s", diff)
			}

			sort.Strings(toDeleteMissing)
			sort.Strings(tc.wantDeleteMissing)
			if diff := cmp.Diff(tc.wantDeleteMissing, toDeleteMissing, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("toDeleteMissing mismatch (-want +got):\n%s", diff)
			}

			sort.Strings(toDeleteUpdate)
			sort.Strings(tc.wantDeleteUpdate)
			if diff := cmp.Diff(tc.wantDeleteUpdate, toDeleteUpdate, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("toDeleteUpdate mismatch (-want +got):\n%s", diff)
			}

			sort.Strings(toDeleteError)
			sort.Strings(tc.wantDeleteError)
			if diff := cmp.Diff(tc.wantDeleteError, toDeleteError, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("toDeleteError mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGKEMaxPodsPerNode18(t *testing.T) {
	gke := &GKE{
		ClusterContext: GKEContext{
			ProjectID:       "test-project",
			ClusterLocation: "us-east5",
			Cluster:         "test-cluster",
			NodeZone:        "us-east5-a",
			MaxPodsPerNode:  18,
		},
	}

	// 1. Dynamic Nodepool
	pod := buildPod(podBuild{
		selector: map[string]string{
			"cloud.google.com/gke-tpu-accelerator": "tpu-v4-podslice",
			"cloud.google.com/gke-tpu-topology":    "2x2x1",
		},
		tpuResource: "4",
	})
	dynamicNP, err := gke.nodePoolForPod(pod)
	if err != nil {
		t.Fatalf("nodePoolForPod failed: %v", err)
	}
	if dynamicNP.MaxPodsConstraint == nil || dynamicNP.MaxPodsConstraint.MaxPodsPerNode != 18 {
		if dynamicNP.MaxPodsConstraint != nil {
			t.Errorf("Dynamic Nodepool: expected MaxPodsPerNode to be 18, got %v", dynamicNP.MaxPodsConstraint.MaxPodsPerNode)
		} else {
			t.Errorf("Dynamic Nodepool: expected MaxPodsPerNode to be 18, got nil MaxPodsConstraint")
		}
	}

	// 2. Static Nodepool
	staticConfig := &StaticNodePoolConfig{
		MachineType: "ct5p-hightpu-4t",
		Accelerator: V5pPodSliceAccelerator,
		Topology:    "2x2x2",
		NodeCount:   8,
	}
	staticNP, err := gke.StaticNodePoolForSubBlock("np-test-0001", "projects/test/reservations/res-1/reservationBlocks/np-name-block/reservationSubBlocks/np-name-block-subblock-0001", staticConfig)
	if err != nil {
		t.Fatalf("StaticNodePoolForSubBlock failed: %v", err)
	}
	if staticNP.MaxPodsConstraint == nil || staticNP.MaxPodsConstraint.MaxPodsPerNode != 18 {
		if staticNP.MaxPodsConstraint != nil {
			t.Errorf("Static Nodepool: expected MaxPodsPerNode to be 18, got %v", staticNP.MaxPodsConstraint.MaxPodsPerNode)
		} else {
			t.Errorf("Static Nodepool: expected MaxPodsPerNode to be 18, got nil MaxPodsConstraint")
		}
	}

	// 3. Static Nodepool with Explicit Override
	staticConfigOverride := &StaticNodePoolConfig{
		MachineType:    "ct5p-hightpu-4t",
		Accelerator:    V5pPodSliceAccelerator,
		Topology:       "2x2x2",
		NodeCount:      8,
		MaxPodsPerNode: 25,
	}
	staticNPOverride, err := gke.StaticNodePoolForSubBlock("np-test-0002", "projects/test/reservations/res-1/reservationBlocks/np-name-block/reservationSubBlocks/np-name-block-subblock-0002", staticConfigOverride)
	if err != nil {
		t.Fatalf("StaticNodePoolForSubBlock failed: %v", err)
	}
	if staticNPOverride.MaxPodsConstraint == nil || staticNPOverride.MaxPodsConstraint.MaxPodsPerNode != 25 {
		if staticNPOverride.MaxPodsConstraint != nil {
			t.Errorf("Static Nodepool Override: expected MaxPodsPerNode to be 25, got %v", staticNPOverride.MaxPodsConstraint.MaxPodsPerNode)
		} else {
			t.Errorf("Static Nodepool Override: expected MaxPodsPerNode to be 25, got nil MaxPodsConstraint")
		}
	}
}
