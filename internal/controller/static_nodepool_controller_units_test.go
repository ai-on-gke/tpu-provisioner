package controller

import (
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetInUseNodepools(t *testing.T) {
	tests := []struct {
		name     string
		slices   []v1beta1.Slice
		nodes    []corev1.Node
		expected map[string]bool
	}{
		{
			name:     "No slices or nodes",
			slices:   []v1beta1.Slice{},
			nodes:    []corev1.Node{},
			expected: map[string]bool{},
		},
		{
			name: "Slice matches Node partition ID",
			slices: []v1beta1.Slice{
				{
					Spec: v1beta1.SliceSpec{
						PartitionIds: []string{"uuid-123"},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							cloud.GKENodePoolNameLabel:                    "pool-1",
							"cloud.google.com/gke-tpu-partition-4x4x4-id": "uuid-123",
						},
					},
				},
			},
			expected: map[string]bool{
				"pool-1": true,
			},
		},
		{
			name: "Slice does not match Node",
			slices: []v1beta1.Slice{
				{
					Spec: v1beta1.SliceSpec{
						PartitionIds: []string{"uuid-999"},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							cloud.GKENodePoolNameLabel:                    "pool-1",
							"cloud.google.com/gke-tpu-partition-4x4x4-id": "uuid-123",
						},
					},
				},
			},
			expected: map[string]bool{},
		},
		{
			name: "Multiple nodes, one in use",
			slices: []v1beta1.Slice{
				{
					Spec: v1beta1.SliceSpec{
						PartitionIds: []string{"uuid-123"},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							cloud.GKENodePoolNameLabel:                    "pool-1",
							"cloud.google.com/gke-tpu-partition-4x4x4-id": "uuid-123",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							cloud.GKENodePoolNameLabel:                    "pool-2",
							"cloud.google.com/gke-tpu-partition-4x4x4-id": "uuid-456",
						},
					},
				},
			},
			expected: map[string]bool{
				"pool-1": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetInUseNodepools(tt.slices, tt.nodes)
			if len(got) != len(tt.expected) {
				t.Errorf("GetInUseNodepools() returned %d items, want %d", len(got), len(tt.expected))
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("GetInUseNodepools()[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestParseNodepoolConfig(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name     string
		yamlData string
		want     *cloud.StaticNodePoolConfig
	}{
		{
			name: "full config with lifecycle recreateOnError false",
			yamlData: `
machineType: "tpu7x-standard-4t"
accelerator: "tpu7x"
topology: "4x4x4"
nodeCount: 16
nodeLabels:
  label-key-1: "label-value-1"
  label-key-2: "label-value-2"
shieldedIntegrityMonitoring: true
shieldedSecureBoot: false
maxPodsPerNode: 8
enableAutorepair: true
placementPolicy: "tpu-provisioner-4x4x4"
lifecycle:
  recreateOnError: false
`,
			want: &cloud.StaticNodePoolConfig{
				MachineType: "tpu7x-standard-4t",
				Accelerator: "tpu7x",
				Topology:    "4x4x4",
				NodeCount:   16,
				NodeLabels: map[string]string{
					"label-key-1": "label-value-1",
					"label-key-2": "label-value-2",
				},
				ShieldedIntegrityMonitoring: boolPtr(true),
				ShieldedSecureBoot:          boolPtr(false),
				MaxPodsPerNode:              8,
				EnableAutoRepair:            boolPtr(true),
				PlacementPolicy:             "tpu-provisioner-4x4x4",
				Lifecycle: cloud.StaticNodePoolLifecycleConfig{
					RecreateOnError: boolPtr(false),
				},
			},
		},
		{
			name: "minimal config with omitted lifecycle",
			yamlData: `
machineType: "ct5p-hightpu-4t"
nodeCount: 4
`,
			want: &cloud.StaticNodePoolConfig{
				MachineType: "ct5p-hightpu-4t",
				NodeCount:   4,
				Lifecycle:   cloud.StaticNodePoolLifecycleConfig{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNodepoolConfig(tt.yamlData)
			if err != nil {
				t.Fatalf("parseNodepoolConfig() unexpected error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseNodepoolConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

