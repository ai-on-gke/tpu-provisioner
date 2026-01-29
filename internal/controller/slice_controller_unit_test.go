package controller

import (
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
	fakeNow := time.Date(2026, 1, 28, 20, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return fakeNow }
	defer func() { Now = oldNow }()

	tests := []struct {
		name                     string
		desired                  []v1beta1.Slice
		existing                 []v1beta1.Slice
		recreateConditionReasons []RecreateCondition
		conditionalRecreateWait  time.Duration
		wantToDelete             []diffedSlice
		wantToCreate             []diffedSlice
		wantRequeueAfter         time.Duration
	}{
		{
			name: "create new slices when none exist",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
				makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}),
			},
			existing:                 []v1beta1.Slice{},
			recreateConditionReasons: nil,
			wantToDelete:             nil,
			wantToCreate: []diffedSlice{
				{slice: makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}), reason: "desired slice does not exist"},
				{slice: makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}), reason: "desired slice does not exist"},
			},
		},
		{
			name: "delete slices with changed NodeSelector without creating replacements",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-5", "cube-6"}}),
				makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-7", "cube-8"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
				makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}),
			},
			recreateConditionReasons: nil,
			wantToDelete: []diffedSlice{
				{slice: makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}), reason: "partition IDs changed"},
				{slice: makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}), reason: "partition IDs changed"},
			},
			wantToCreate: nil,
		},
		{
			name: "no changes when NodeSelectors match",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
				makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
				makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}),
			},
			recreateConditionReasons: nil,
			wantToDelete:             nil,
			wantToCreate:             nil,
		},
		{
			name: "recreate slice when Ready condition matches reason",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  "tpu-v7x",
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:    v1beta1.SliceStateConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "FailedToProvision",
							Message: "Some internal error occurred",
						},
					},
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}},
			wantToDelete: []diffedSlice{
				{
					slice: makeSliceWithAccel(sliceOptions{
						name:       "slice-1",
						accelType:  "tpu-v7x",
						topology:   "4x4x8",
						partitions: []string{"cube-1", "cube-2"},
						conditions: []metav1.Condition{
							{
								Type:    v1beta1.SliceStateConditionType,
								Status:  metav1.ConditionFalse,
								Reason:  "FailedToProvision",
								Message: "Some internal error occurred",
							},
						},
					}),
					reason: "recreation condition matched: FailedToProvision: Some internal error occurred",
				},
			},
			wantToCreate: nil,
		},
		{
			name: "recreate slice when Ready condition matches reason and message substring",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  "tpu-v7x",
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:    v1beta1.SliceStateConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "FailedToProvision",
							Message: "Internal error: permission denied",
						},
					},
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision", MessageSubstring: "permission denied"}},
			wantToDelete: []diffedSlice{
				{
					slice: makeSliceWithAccel(sliceOptions{
						name:       "slice-1",
						accelType:  "tpu-v7x",
						topology:   "4x4x8",
						partitions: []string{"cube-1", "cube-2"},
						conditions: []metav1.Condition{
							{
								Type:    v1beta1.SliceStateConditionType,
								Status:  metav1.ConditionFalse,
								Reason:  "FailedToProvision",
								Message: "Internal error: permission denied",
							},
						},
					}),
					reason: "recreation condition matched: FailedToProvision: Internal error: permission denied",
				},
			},
			wantToCreate: nil,
		},
		{
			name: "do not recreate slice when Reason matches but message substring does not",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  "tpu-v7x",
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:    v1beta1.SliceStateConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "FailedToProvision",
							Message: "Internal error: timeout",
						},
					},
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision", MessageSubstring: "permission denied"}},
			wantToDelete:             nil,
			wantToCreate:             nil,
		},
		{
			name: "do not recreate slice when Ready condition reason does not match",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  "tpu-v7x",
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:   v1beta1.SliceStateConditionType,
							Status: metav1.ConditionFalse,
							Reason: "SomeOtherReason",
						},
					},
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}},
			wantToDelete:             nil,
			wantToCreate:             nil,
		},
		{
			name: "recreate slice when Ready condition is Unknown and matches reason",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  "tpu-v7x",
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:   v1beta1.SliceStateConditionType,
							Status: metav1.ConditionUnknown,
							Reason: "ProvisioningTimeout",
						},
					},
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}, {Reason: "ProvisioningTimeout"}},
			wantToDelete: []diffedSlice{
				{
					slice: makeSliceWithAccel(sliceOptions{
						name:       "slice-1",
						accelType:  "tpu-v7x",
						topology:   "4x4x8",
						partitions: []string{"cube-1", "cube-2"},
						conditions: []metav1.Condition{
							{
								Type:   v1beta1.SliceStateConditionType,
								Status: metav1.ConditionUnknown,
								Reason: "ProvisioningTimeout",
							},
						},
					}),
					reason: "recreation condition matched: ProvisioningTimeout",
				},
			},
			wantToCreate: nil,
		},
		{
			name: "do not recreate slice when reason matches but Ready is True",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: "tpu-v7x", topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  "tpu-v7x",
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:    v1beta1.SliceStateConditionType,
							Status:  metav1.ConditionTrue,
							Reason:  "FailedToProvision",
							Message: "Actually it worked",
						},
					},
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}},
			wantToDelete:             nil,
			wantToCreate:             nil,
		},
		{
			name: "do not delete slice if younger than conditionalRecreateWait",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: tpu7xAccelerator, topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  tpu7xAccelerator,
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:   v1beta1.SliceStateConditionType,
							Status: metav1.ConditionFalse,
							Reason: "FailedToProvision",
						},
					},
					creationTimestamp: metav1.NewTime(fakeNow.Add(-30 * time.Minute)),
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}},
			conditionalRecreateWait:  time.Hour,
			wantToDelete:             nil,
			wantToCreate:             nil,
			wantRequeueAfter:         30 * time.Minute,
		},
		{
			name: "delete slice if older than conditionalRecreateWait",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: tpu7xAccelerator, topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  tpu7xAccelerator,
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:   v1beta1.SliceStateConditionType,
							Status: metav1.ConditionFalse,
							Reason: "FailedToProvision",
						},
					},
					creationTimestamp: metav1.NewTime(fakeNow.Add(-90 * time.Minute)),
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}},
			conditionalRecreateWait:  time.Hour,
			wantToDelete: []diffedSlice{
				{
					slice: makeSliceWithAccel(sliceOptions{
						name:       "slice-1",
						accelType:  tpu7xAccelerator,
						topology:   "4x4x8",
						partitions: []string{"cube-1", "cube-2"},
						conditions: []metav1.Condition{
							{
								Type:   v1beta1.SliceStateConditionType,
								Status: metav1.ConditionFalse,
								Reason: "FailedToProvision",
							},
						},
						creationTimestamp: metav1.NewTime(fakeNow.Add(-90 * time.Minute)),
					}),
					reason: "recreation condition matched: FailedToProvision",
				},
			},
			wantToCreate: nil,
		},
		{
			name: "select minimum requeueAfter for multiple slices",
			desired: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{name: "slice-1", accelType: tpu7xAccelerator, topology: "4x4x8", partitions: []string{"cube-1", "cube-2"}}),
				makeSliceWithAccel(sliceOptions{name: "slice-2", accelType: tpu7xAccelerator, topology: "4x4x8", partitions: []string{"cube-3", "cube-4"}}),
			},
			existing: []v1beta1.Slice{
				makeSliceWithAccel(sliceOptions{
					name:       "slice-1",
					accelType:  tpu7xAccelerator,
					topology:   "4x4x8",
					partitions: []string{"cube-1", "cube-2"},
					conditions: []metav1.Condition{
						{
							Type:   v1beta1.SliceStateConditionType,
							Status: metav1.ConditionFalse,
							Reason: "FailedToProvision",
						},
					},
					creationTimestamp: metav1.NewTime(fakeNow.Add(-45 * time.Minute)),
				}),
				makeSliceWithAccel(sliceOptions{
					name:       "slice-2",
					accelType:  tpu7xAccelerator,
					topology:   "4x4x8",
					partitions: []string{"cube-3", "cube-4"},
					conditions: []metav1.Condition{
						{
							Type:   v1beta1.SliceStateConditionType,
							Status: metav1.ConditionFalse,
							Reason: "FailedToProvision",
						},
					},
					creationTimestamp: metav1.NewTime(fakeNow.Add(-30 * time.Minute)),
				}),
			},
			recreateConditionReasons: []RecreateCondition{{Reason: "FailedToProvision"}},
			conditionalRecreateWait:  time.Hour,
			wantToDelete:             nil,
			wantToCreate:             nil,
			wantRequeueAfter:         15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToDelete, gotToCreate, gotRequeueAfter := diffSlices(tt.desired, tt.existing, tt.recreateConditionReasons, tt.conditionalRecreateWait)

			compareSlices := func(msg string, want, got []diffedSlice) {
				if len(want) != len(got) {
					t.Errorf("%s length mismatch: want %d, got %d", msg, len(want), len(got))
					return
				}
				for i := range want {
					if want[i].slice.Name != got[i].slice.Name {
						t.Errorf("%s[%d] slice name mismatch: want %s, got %s", msg, i, want[i].slice.Name, got[i].slice.Name)
					}
					if want[i].reason != got[i].reason {
						t.Errorf("%s[%d] reason mismatch: want %s, got %s", msg, i, want[i].reason, got[i].reason)
					}
				}
			}

			compareSlices("toDelete", tt.wantToDelete, gotToDelete)
			compareSlices("toCreate", tt.wantToCreate, gotToCreate)
			if tt.wantRequeueAfter != 0 {
				if gotRequeueAfter < tt.wantRequeueAfter || gotRequeueAfter > tt.wantRequeueAfter+3*time.Second {
					t.Errorf("diffSlices() requeueAfter = %v, want between %v and %v", gotRequeueAfter, tt.wantRequeueAfter, tt.wantRequeueAfter+3*time.Second)
				}
			} else if gotRequeueAfter != 0 {
				t.Errorf("diffSlices() requeueAfter = %v, want 0", gotRequeueAfter)
			}
		})
	}
}

func TestParseRecreateConditions(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
		want []RecreateCondition
	}{
		{
			name: "empty input",
			raw:  []string{},
			want: nil,
		},
		{
			name: "skip empty strings",
			raw:  []string{"", "Reason1", ""},
			want: []RecreateCondition{{Reason: "Reason1"}},
		},
		{
			name: "reasons only",
			raw:  []string{"Reason1", "Reason2"},
			want: []RecreateCondition{{Reason: "Reason1"}, {Reason: "Reason2"}},
		},
		{
			name: "reason with substring",
			raw:  []string{"Reason1:'substring1'"},
			want: []RecreateCondition{{Reason: "Reason1", MessageSubstring: "substring1"}},
		},
		{
			name: "reason with substring containing spaces",
			raw:  []string{"Reason1: 'substring with spaces' "},
			want: []RecreateCondition{{Reason: "Reason1", MessageSubstring: "substring with spaces"}},
		},
		{
			name: "mixed formats",
			raw:  []string{"Reason1", "Reason2:'substring2'", "Reason3:''"},
			want: []RecreateCondition{
				{Reason: "Reason1"},
				{Reason: "Reason2", MessageSubstring: "substring2"},
				{Reason: "Reason3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRecreateConditions(tt.raw)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseRecreateConditions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type sliceOptions struct {
	name              string
	accelType         string
	topology          string
	partitions        []string
	conditions        []metav1.Condition
	creationTimestamp metav1.Time
}

// Helper function to create a Slice object for testing
func makeSlice(name, topology string, cubes ...string) v1beta1.Slice {
	return makeSliceWithAccel(sliceOptions{
		name:       name,
		accelType:  tpu7xAccelerator,
		topology:   topology,
		partitions: cubes,
	})
}

// Helper function to create a Slice object with custom accelerator type
func makeSliceWithAccel(opts sliceOptions) v1beta1.Slice {
	s := makeSliceWithJobSet(opts.name, opts.accelType, opts.topology, "test-jobset", "default", opts.partitions...)
	s.Status.Conditions = opts.conditions
	s.CreationTimestamp = opts.creationTimestamp
	return s
}

// Helper function to create a Slice object with custom JobSet name and namespace
func makeSliceWithJobSet(name, accelType, topology, jobsetName, jobsetNamespace string, cubes ...string) v1beta1.Slice {
	return v1beta1.Slice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				SliceOwnerKindLabel:      "jobset",
				SliceOwnerNameLabel:      jobsetName,
				SliceOwnerNamespaceLabel: jobsetNamespace,
			},
		},
		Spec: v1beta1.SliceSpec{
			Type:         v1beta1.Type(accelType),
			Topology:     topology,
			PartitionIds: cubes,
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

func sliceCompareOptions() []cmp.Option {
	return []cmp.Option{
		cmpopts.IgnoreUnexported(v1beta1.Slice{}, v1beta1.SliceSpec{}, metav1.ObjectMeta{}, metav1.Condition{}, metav1.Time{}),
		cmp.Comparer(func(a, b metav1.Time) bool {
			return a.Time.Equal(b.Time)
		}),
		cmp.Comparer(func(a, b metav1.Condition) bool {
			return a.Type == b.Type && a.Status == b.Status && a.Reason == b.Reason && a.Message == b.Message
		}),
	}
}
