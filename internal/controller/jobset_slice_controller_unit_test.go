package controller

import (
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
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
		want      []v1beta1.Slice
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
					},
				},
			},
			want: []v1beta1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "4x4x4"),
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
					},
				},
			},
			want: []v1beta1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "4x4x4"),
				makeSlice("js-test-jobset-test-uid-worker-1", "4x4x4"),
				makeSlice("js-test-jobset-test-uid-worker-2", "4x4x4"),
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
						{
							Name:     "worker-2",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "4x4x8",
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
			want: []v1beta1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-1-0", "4x4x4"),
				makeSlice("js-test-jobset-test-uid-worker-1-1", "4x4x4"),
				makeSlice("js-test-jobset-test-uid-worker-2-0", "4x4x8"),
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
												topologyAnnotation: "4x4x4",
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
												topologyAnnotation: "4x4x4",
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
												topologyAnnotation: "4x4x8",
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
			want: []v1beta1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "4x4x8", "cube-1", "cube-2"),
				makeSlice("js-test-jobset-test-uid-worker-1", "4x4x8", "cube-3", "cube-4"),
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
												topologyAnnotation: "4x4x8",
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
			want: []v1beta1.Slice{
				makeSlice("js-test-jobset-test-uid-worker-0", "4x4x8", "cube-1", "cube-2"),
				makeSlice("js-test-jobset-test-uid-worker-1", "4x4x8"),
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
					},
				},
			},
			want: []v1beta1.Slice{
				makeSliceWithJobSet("js-this-is-a-very-long-jobset-name--test-uid-long-repli-0", tpu7xAccelerator, "4x4x4", "this-is-a-very-long-jobset-name-that-exceeds-the-character-limit", "default"),
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
						{
							Name:     "v6e-worker",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Annotations: map[string]string{
												topologyAnnotation: "4x4x8",
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
			want: []v1beta1.Slice{
				makeSlice("js-test-jobset-test-uid-v7x-worker-0", "4x4x4"),
			},
			wantErr: false,
		},
		{
			name: "JobSet with duplicate partition IDs in slice selection",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker":[["cube-1"],["cube-1"]]}`,
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
					},
				},
			},
			want:      nil,
			wantErr:   true,
			errSubstr: `duplicate partition ID "cube-1" found`,
		},
		{
			name: "JobSet with duplicate partition IDs across different workers",
			jobSet: &jobset.JobSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-jobset",
					Namespace: "default",
					UID:       testUID,
					Annotations: map[string]string{
						SliceSelectionAnnotation: `{"worker-1":[["cube-1"]],"worker-2":[["cube-1"]]}`,
					},
				},
				Spec: jobset.JobSetSpec{
					ReplicatedJobs: []jobset.ReplicatedJob{
						{
							Name:     "worker-1",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
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
						{
							Name:     "worker-2",
							Replicas: 1,
							Template: batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: corev1.PodTemplateSpec{
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
					},
				},
			},
			want:      nil,
			wantErr:   true,
			errSubstr: `duplicate partition ID "cube-1" found`,
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
