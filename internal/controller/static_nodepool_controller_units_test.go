package controller

import (
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
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
