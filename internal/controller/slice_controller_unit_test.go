package controller

import (
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

func TestJobsetSlices(t *testing.T) {
	testUID := types.UID("test-uid-12345678")

	tests := []struct {
		name      string
		jobSet    *jobset.JobSet
		want      []v1alpha1.Slice
		wantErr   bool
		errSubstr string
	}{
		{
			name: "basic JobSet with single replicated job and single replica",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "2x2x4"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with multiple replicas",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 3,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "2x2x4"),
				makeSlice("js-test-jobset-test-uid-worker-1", "2x2x4"),
				makeSlice("js-test-jobset-test-uid-worker-2", "2x2x4"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with multiple replicated jobs",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker-1",
							Replicas: 2,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
						{
							Name:     "worker-2",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x2",
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
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-1-0", "2x2x4"),
				makeSlice("js-test-jobset-test-uid-worker-1-1", "2x2x4"),
				makeSlice("js-test-jobset-test-uid-worker-2-0", "2x2x2"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with no replicated jobs",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "JobSet with replicated job but no node selector",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
											},
										},
										Spec: corev1.PodSpec{
											NodeSelector: nil,
										},
									},
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
			name: "JobSet with wrong accelerator (not v7x)",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
											},
										},
										Spec: corev1.PodSpec{
											NodeSelector: map[string]string{
												acceleratorSelector: "tpu-v6e",
											},
										},
									},
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
			name: "JobSet with v7x accelerator but no topology annotation",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										Spec: corev1.PodSpec{
											NodeSelector: map[string]string{
												acceleratorSelector: tpu7xAccelerator,
											},
										},
									},
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
			name: "JobSet with v7x accelerator but no pod annotations",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: nil,
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
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "JobSet with valid slice selection annotation",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker":[["cube-1","cube-2"],["cube-3","cube-4"]]}`,
					},
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 2,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "2x2x4", "cube-1", "cube-2"),
				makeSlice("js-test-jobset-test-uid-worker-1", "2x2x4", "cube-3", "cube-4"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with invalid slice selection annotation (malformed JSON)",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker": invalid json}`,
					},
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
					},
				},
			},
			want:      nil,
			wantErr:   true,
			errSubstr: "parsing slice selection",
		},
		{
			name: "JobSet with partial cube selection (only for first replica)",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker":[["cube-1","cube-2"]]}`,
					},
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker",
							Replicas: 2,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "2x2x4", "cube-1", "cube-2"),
				makeSlice("js-test-jobset-test-uid-worker-1", "2x2x4"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with long name should be truncated in slice name",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "this-is-a-very-long-jobset-name-that-exceeds-the-character-limit",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "long-replicated-job-name",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-this-is-a-very-long-jobset-name--test-uid-long-repli-0", "2x2x4"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with mixed accelerators (v7x and non-v7x)",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "v7x-worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
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
						{
							Name:     "v6e-worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "2x2x4",
											},
										},
										Spec: corev1.PodSpec{
											NodeSelector: map[string]string{
												acceleratorSelector: "tpu-v6e",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []v1alpha1.Slice{
				makeSlice("js-test-jobset-test-uid-v7x-worker-0", "2x2x4"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jobsetSlices(tt.jobSet)
			if (err != nil) != tt.wantErr {
				t.Errorf("jobsetSlices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("jobsetSlices() error = %v, expected to contain %q", err, tt.errSubstr)
				}
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("jobsetSlices() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseSliceSelection(t *testing.T) {
	tests := []struct {
		name      string
		jobSet    *jobset.JobSet
		want      map[string][][]string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "no slice selection annotation",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
				},
			},
			want:    map[string][][]string{},
			wantErr: false,
		},
		{
			name: "nil annotations",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-jobset",
					Namespace:   "default",
					Annotations: nil,
				},
			},
			want:    map[string][][]string{},
			wantErr: false,
		},
		{
			name: "valid slice selection annotation",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker":[["cube-1","cube-2"],["cube-3","cube-4"]]}`,
					},
				},
			},
			want: map[string][][]string{
				"worker": {
					{"cube-1", "cube-2"},
					{"cube-3", "cube-4"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid slice selection annotation with multiple replicated jobs",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker-1":[["cube-1"]],"worker-2":[["cube-2","cube-3"]]}`,
					},
				},
			},
			want: map[string][][]string{
				"worker-1": {{"cube-1"}},
				"worker-2": {{"cube-2", "cube-3"}},
			},
			wantErr: false,
		},
		{
			name: "invalid JSON in slice selection annotation",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker": invalid}`,
					},
				},
			},
			want:      nil,
			wantErr:   true,
			errSubstr: "slice selection should be of the format",
		},
		{
			name: "empty slice selection annotation",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{}`,
					},
				},
			},
			want:    map[string][][]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSliceSelection(tt.jobSet)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSliceSelection() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("parseSliceSelection() error = %v, expected to contain %q", err, tt.errSubstr)
				}
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseSliceSelection() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDiffSlices(t *testing.T) {
	tests := []struct {
		name         string
		desired      []v1alpha1.Slice
		existing     []v1alpha1.Slice
		wantToDelete []v1alpha1.Slice
		wantToCreate []v1alpha1.Slice
	}{
		{
			name: "create new slices when none exist",
			desired: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-1", "cube-2"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-3", "cube-4"),
			},
			existing:     []v1alpha1.Slice{},
			wantToDelete: nil,
			wantToCreate: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-1", "cube-2"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-3", "cube-4"),
			},
		},
		{
			name: "delete slices with changed NodeSelector without creating replacements",
			desired: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-5", "cube-6"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-7", "cube-8"),
			},
			existing: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-1", "cube-2"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-3", "cube-4"),
			},
			wantToDelete: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-1", "cube-2"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-3", "cube-4"),
			},
			wantToCreate: nil,
		},
		{
			name: "no changes when NodeSelectors match",
			desired: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-1", "cube-2"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-3", "cube-4"),
			},
			existing: []v1alpha1.Slice{
				makeSliceWithAccel("slice-1", "tpu-v7x", "2x2x4", "cube-1", "cube-2"),
				makeSliceWithAccel("slice-2", "tpu-v7x", "2x2x4", "cube-3", "cube-4"),
			},
			wantToDelete: nil,
			wantToCreate: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToDelete, gotToCreate := diffSlices(tt.desired, tt.existing)
			if diff := cmp.Diff(tt.wantToDelete, gotToDelete); diff != "" {
				t.Errorf("diffSlices() toDelete mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantToCreate, gotToCreate); diff != "" {
				t.Errorf("diffSlices() toCreate mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Helper function to create a Slice object for testing
func makeSlice(name, topology string, cubes ...string) v1alpha1.Slice {
	return makeSliceWithAccel(name, tpu7xAccelerator, topology, cubes...)
}

// Helper function to create a Slice object with custom accelerator type
func makeSliceWithAccel(name, accelType, topology string, cubes ...string) v1alpha1.Slice {
	nodeSelector := map[string][]string{}
	if len(cubes) > 0 {
		nodeSelector[cubeSelectionLabel] = cubes
	}
	return v1alpha1.Slice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: v1alpha1.SliceSpec{
			AcceleratorType:     accelType,
			AcceleratorTopology: topology,
			NodeSelector:        nodeSelector,
		},
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
