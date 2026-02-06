package controller

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"
)

func TestLWSSlices(t *testing.T) {
	testUID := types.UID("test-uid-lws")

	tests := []struct {
		name      string
		lwset     *lws.LeaderWorkerSet
		want      []v1beta1.Slice
		wantErr   bool
		errSubstr string
	}{
		{
			name: "basic LeaderWorkerSet with single replica (no slices should be created)",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: lws.LeaderWorkerSetSpec{
					LeaderWorkerTemplate: lws.LeaderWorkerTemplate{
						WorkerTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									topologyAnnotation: "4x4x4",
								},
							},
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									acceleratorSelector: tpu7xAccelerator,
								},
							},
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "LeaderWorkerSet with multiple replicas (no slices should be created)",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: lws.LeaderWorkerSetSpec{
					Replicas: func(i int32) *int32 { return &i }(3),
					LeaderWorkerTemplate: lws.LeaderWorkerTemplate{
						WorkerTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									topologyAnnotation: "4x4x4",
								},
							},
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									acceleratorSelector: tpu7xAccelerator,
								},
							},
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "LeaderWorkerSet with slice selection annotation",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `[{"workers":[["cube-1","cube-2"]]},{"workers":[["cube-3","cube-4"]]}]`,
					},
				},
				Spec: lws.LeaderWorkerSetSpec{
					Replicas: func(i int32) *int32 { return &i }(2),
					LeaderWorkerTemplate: lws.LeaderWorkerTemplate{
						WorkerTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									topologyAnnotation: "4x4x8",
								},
							},
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									acceleratorSelector: tpuV7xAccelerator,
								},
							},
						},
					},
				},
			},
			want: []v1beta1.Slice{
				makeLWSSlice("lws-test-lws-test-uid-worker-0", tpuV7xAccelerator, "4x4x8", "test-lws", "default", "cube-1", "cube-2"),
				makeLWSSlice("lws-test-lws-test-uid-worker-1", tpuV7xAccelerator, "4x4x8", "test-lws", "default", "cube-3", "cube-4"),
			},
			wantErr: false,
		},
		{
			name: "LeaderWorkerSet with leader and worker slices",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `[{"leader":["cube-0"],"workers":[["cube-1","cube-2"]]}]`,
					},
				},
				Spec: lws.LeaderWorkerSetSpec{
					Replicas: func(i int32) *int32 { return &i }(1),
					LeaderWorkerTemplate: lws.LeaderWorkerTemplate{
						LeaderTemplate: &corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									topologyAnnotation: "4x4x1",
								},
							},
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									acceleratorSelector: tpu7xAccelerator,
								},
							},
						},
						WorkerTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									topologyAnnotation: "4x4x4",
								},
							},
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									acceleratorSelector: tpu7xAccelerator,
								},
							},
						},
					},
				},
			},
			want: []v1beta1.Slice{
				makeLWSSlice("lws-test-lws-test-uid-leader", tpu7xAccelerator, "4x4x1", "test-lws", "default", "cube-0"),
				makeLWSSlice("lws-test-lws-test-uid-worker-0", tpu7xAccelerator, "4x4x4", "test-lws", "default", "cube-1", "cube-2"),
			},
			wantErr: false,
		},
		{
			name: "LeaderWorkerSet with invalid slice selection",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker": invalid}`,
					},
				},
				Spec: lws.LeaderWorkerSetSpec{
					LeaderWorkerTemplate: lws.LeaderWorkerTemplate{
						WorkerTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									topologyAnnotation: "4x4x4",
								},
							},
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									acceleratorSelector: tpu7xAccelerator,
								},
							},
						},
					},
				},
			},
			want:      nil,
			wantErr:   true,
			errSubstr: "parsing slice selection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lwsSlices(tt.lwset)
			if (err != nil) != tt.wantErr {
				t.Errorf("lwsSlices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("lwsSlices() error = %v, expected to contain %q", err, tt.errSubstr)
				}
				return
			}
			if diff := cmp.Diff(tt.want, got, sliceCompareOptions()...); diff != "" {
				t.Errorf("lwsSlices() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseLWSSliceSelection(t *testing.T) {
	tests := []struct {
		name      string
		lwset     *lws.LeaderWorkerSet
		want      lwsSliceSelection
		wantErr   bool
		errSubstr string
	}{
		{
			name: "no slice selection annotation",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "valid slice selection annotation",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					Annotations: map[string]string{
						SliceSelectionAnnotation: `[{"leader":["cube-0"],"workers":[["cube-1","cube-2"]]},{"leader":["cube-3"],"workers":[["cube-4","cube-5"]]}]`,
					},
				},
			},
			want: lwsSliceSelection{
				{
					Leader: []string{"cube-0"},
					Workers: [][]string{
						{"cube-1", "cube-2"},
					},
				},
				{
					Leader: []string{"cube-3"},
					Workers: [][]string{
						{"cube-4", "cube-5"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid JSON",
			lwset: &lws.LeaderWorkerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lws",
					Namespace: "default",
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker": invalid}`,
					},
				},
			},
			want:      lwsSliceSelection{},
			wantErr:   true,
			errSubstr: "slice selection should be of the format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLWSSliceSelection(tt.lwset)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLWSSliceSelection() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("parseLWSSliceSelection() error = %v, expected to contain %q", err, tt.errSubstr)
				}
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseLWSSliceSelection() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSliceToLWSRequests(t *testing.T) {
	r := &LeaderWorkerSetSliceReconciler{}

	tests := []struct {
		name string
		obj  client.Object
		want []reconcile.Request
	}{
		{
			name: "slice with LWS owner",
			obj: &v1beta1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
					Labels: map[string]string{
						SliceOwnerKindLabel:      LWSOwnerKind,
						SliceOwnerNameLabel:      "test-lws",
						SliceOwnerNamespaceLabel: "default",
					},
				},
			},
			want: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      "test-lws",
						Namespace: "default",
					},
				},
			},
		},
		{
			name: "slice with JobSet owner",
			obj: &v1beta1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
					Labels: map[string]string{
						SliceOwnerKindLabel:      jobSetOwnerKind,
						SliceOwnerNameLabel:      "test-jobset",
						SliceOwnerNamespaceLabel: "default",
					},
				},
			},
			want: nil,
		},
		{
			name: "slice with no labels",
			obj: &v1beta1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
			},
			want: nil,
		},
		{
			name: "not a slice object",
			obj:  &corev1.Node{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.sliceToLWSRequests(context.Background(), tt.obj)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("sliceToLWSRequests() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Helper to make a Slice for LWS tests
func makeLWSSlice(name, accel, topology, lwsName, lwsNamespace string, partitions ...string) v1beta1.Slice {
	return v1beta1.Slice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				SliceOwnerKindLabel:      LWSOwnerKind,
				SliceOwnerNameLabel:      lwsName,
				SliceOwnerNamespaceLabel: lwsNamespace,
			},
		},
		Spec: v1beta1.SliceSpec{
			Type:         v1beta1.Type(accel),
			Topology:     topology,
			PartitionIds: partitions,
		},
	}
}
