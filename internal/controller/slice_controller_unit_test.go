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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-1",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-2",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
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
												acceleratorSelector: v7xAccelerator,
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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-1-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-1-1",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-2-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x2",
						NodeSelector:        map[string][]string{},
					},
				},
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
												acceleratorSelector: v7xAccelerator,
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
												acceleratorSelector: v7xAccelerator,
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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector: map[string][]string{
							cubeSelectionLabel: {"cube-1", "cube-2"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-1",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector: map[string][]string{
							cubeSelectionLabel: {"cube-3", "cube-4"},
						},
					},
				},
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
												acceleratorSelector: v7xAccelerator,
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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector: map[string][]string{
							cubeSelectionLabel: {"cube-1", "cube-2"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-worker-1",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-this-is-a-very-long-jobset-name--test-uid-long-repli-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
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
												acceleratorSelector: v7xAccelerator,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "js-test-jobset-test-uid-v7x-worker-0",
					},
					Spec: v1alpha1.SliceSpec{
						AcceleratorType:     v7xAccelerator,
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
				},
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

func TestSliceName(t *testing.T) {
	tests := []struct {
		name                 string
		jobSetName           string
		jobSetUID            types.UID
		replicatedJobName    string
		replicatedJobReplica int
		want                 string
	}{
		{
			name:                 "basic slice name",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 0,
			want:                 "js-test-jobset-12345678-worker-0",
		},
		{
			name:                 "slice name with long jobset name (should be truncated)",
			jobSetName:           "this-is-a-very-long-jobset-name-that-exceeds-the-limit",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 0,
			want:                 "js-this-is-a-very-long-jobset-name--12345678-worker-0",
		},
		{
			name:                 "slice name with long replicated job name (should be truncated)",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "very-long-replicated-job-name",
			replicatedJobReplica: 0,
			want:                 "js-test-jobset-12345678-very-long--0",
		},
		{
			name:                 "slice name with high replica index",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 99,
			want:                 "js-test-jobset-12345678-worker-99",
		},
		{
			name:                 "slice name with short jobset name",
			jobSetName:           "js",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "w",
			replicatedJobReplica: 5,
			want:                 "js-js-12345678-w-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.jobSetName,
					UID:  tt.jobSetUID,
				},
			}
			got := sliceName(js, tt.replicatedJobName, tt.replicatedJobReplica)
			if got != tt.want {
				t.Errorf("sliceName() = %v, want %v", got, tt.want)
			}
		})
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
