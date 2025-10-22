package controllertest

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1alpha1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/controller"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	jobset "sigs.k8s.io/jobset/api/jobset/v1alpha2"
)

// +kubebuilder:docs-gen:collapse=Imports

// ExpectedSliceSpec embeds SliceSpec and adds a Replicas field to specify
// how many Slices with this spec are expected.
type ExpectedSliceSpec struct {
	v1alpha1.SliceSpec
	Replicas int
}

var _ = Describe("Slice controller", func() {

	// A test case contains a JobSet to create and whether we expect Slice resources to be created.
	type testCase struct {
		jobSet            *jobset.JobSet
		wantSliceCreation bool
		expectedSlices    []ExpectedSliceSpec
	}

	DescribeTable("JobSets are created and Slices are reconciled",
		// Logic for each test case.
		func(tc *testCase) {
			ctx := context.Background()
			// Create test namespace for each entry to isolate each test case.
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-ns-",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Clean up temporary namespace after each test case.
			defer func() {
				Expect(deleteNamespace(ctx, k8sClient, ns)).To(Succeed())
			}()

			// Create JobSet in test namespace.
			js := tc.jobSet
			js.Namespace = ns.Name

			By(fmt.Sprintf("Creating JobSet %s", js.Name))
			Expect(k8sClient.Create(ctx, js)).To(Succeed())

			// Check Slice creation.
			if tc.wantSliceCreation {
				By("Checking that the JobSet triggered Slice creation")
				assertSlicesCreated(ctx, js, tc.expectedSlices)
			} else {
				By("Checking that JobSet did not trigger Slice creation")
				assertSlicesNotCreated(ctx, js)
			}
		},
		// Test cases.
		Entry("JobSet with slice provisioning enabled and v7x accelerator should create Slices", &testCase{
			jobSet: constructJobSet("test-js-1",
				withAnnotation(controller.SliceProvisioningLabel, "true"),
				withReplicatedJob("worker", 2, makeJobTemplateWithTPU("tpu-v7x", "2x2x4")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						AcceleratorType:     "tpu-v7x",
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
					Replicas: 2,
				},
			},
		}),
		Entry("JobSet with slice provisioning enabled but no v7x accelerator should not create Slices", &testCase{
			jobSet: constructJobSet("test-js-2",
				withAnnotation(controller.SliceProvisioningLabel, "true"),
				withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu-v6e", "2x2x4")),
			),
			wantSliceCreation: false,
		}),
		Entry("JobSet without slice provisioning annotation should not create Slices", &testCase{
			jobSet: constructJobSet("test-js-3",
				withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu-v7x", "2x2x4")),
			),
			wantSliceCreation: false,
		}),
		Entry("JobSet with slice provisioning but auto-provisioning disabled should not create Slices", &testCase{
			jobSet: constructJobSet("test-js-4",
				withAnnotation(controller.SliceProvisioningLabel, "true"),
				withAnnotation(controller.DisableAutoProvisioningLabel, "true"),
				withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu-v7x", "2x2x4")),
			),
			wantSliceCreation: false,
		}),
		Entry("JobSet with slice provisioning and multiple replicas should create multiple Slices", &testCase{
			jobSet: constructJobSet("test-js-5",
				withAnnotation(controller.SliceProvisioningLabel, "true"),
				withReplicatedJob("worker", 3, makeJobTemplateWithTPU("tpu-v7x", "2x2x4")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						AcceleratorType:     "tpu-v7x",
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
					Replicas: 3,
				},
			},
		}),
		Entry("JobSet with slice provisioning and 2 replicated jobs should create Slices for both", &testCase{
			jobSet: constructJobSet("test-js-6",
				withAnnotation(controller.SliceProvisioningLabel, "true"),
				withReplicatedJob("worker-1", 2, makeJobTemplateWithTPU("tpu-v7x", "2x2x4")),
				withReplicatedJob("worker-2", 1, makeJobTemplateWithTPU("tpu-v7x", "2x2x2")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						AcceleratorType:     "tpu-v7x",
						AcceleratorTopology: "2x2x4",
						NodeSelector:        map[string][]string{},
					},
					Replicas: 2,
				},
				{
					SliceSpec: v1alpha1.SliceSpec{
						AcceleratorType:     "tpu-v7x",
						AcceleratorTopology: "2x2x2",
						NodeSelector:        map[string][]string{},
					},
					Replicas: 1,
				},
			},
		}),
		1)
})

// JobSetOption is a function that modifies a JobSet.
type JobSetOption func(*jobset.JobSet)

// constructJobSet creates a JobSet with the given name, replicas, and options.
func constructJobSet(name string, opts ...JobSetOption) *jobset.JobSet {
	js := &jobset.JobSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: jobset.JobSetSpec{
			Network: &jobset.Network{},
		},
	}

	// Apply all options
	for _, opt := range opts {
		opt(js)
	}

	return js
}

// withAnnotation adds an annotation to the JobSet.
func withAnnotation(key, value string) JobSetOption {
	return func(js *jobset.JobSet) {
		if js.Annotations == nil {
			js.Annotations = make(map[string]string)
		}
		js.Annotations[key] = value
	}
}

// withReplicatedJob appends a replicated job to the JobSet with the given name, replica count, and template.
func withReplicatedJob(name string, replicas int32, template batchv1.JobTemplateSpec) JobSetOption {
	return func(js *jobset.JobSet) {
		// Append the new replicated job
		js.Spec.ReplicatedJobs = append(js.Spec.ReplicatedJobs, jobset.ReplicatedJob{
			Name:     name,
			Replicas: replicas,
			Template: template,
		})
	}
}

// makeJobTemplate creates a basic Job template.
func makeJobTemplate() batchv1.JobTemplateSpec {
	return batchv1.JobTemplateSpec{
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "test-image",
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

// makeJobTemplateWithTPU creates a Job template with TPU accelerator and topology.
func makeJobTemplateWithTPU(accelerator, topology string) batchv1.JobTemplateSpec {
	template := makeJobTemplate()
	template.Spec.Template.Annotations = map[string]string{
		"cloud.google.com/gke-tpu-topology": topology,
	}
	template.Spec.Template.Spec.NodeSelector = map[string]string{
		"cloud.google.com/gke-tpu-accelerator": accelerator,
	}
	return template
}

// assertSlicesCreated validates that Slice resources were created for the given JobSet.
func assertSlicesCreated(ctx context.Context, js *jobset.JobSet, expectedSlices []ExpectedSliceSpec) {
	// Calculate total expected count
	totalExpectedCount := 0
	for _, expected := range expectedSlices {
		totalExpectedCount += expected.Replicas
	}

	var sliceList v1alpha1.SliceList
	Eventually(func() int {
		err := k8sClient.List(ctx, &sliceList)
		if err != nil {
			return 0
		}

		// Count Slices that belong to this JobSet.
		count := 0
		for _, slice := range sliceList.Items {
			if slice.Namespace == js.Namespace {
				// Check if the Slice is owned by the JobSet.
				for _, ownerRef := range slice.OwnerReferences {
					if ownerRef.UID == js.UID {
						count++
						break
					}
				}
			}
		}
		return count
	}, 3*time.Second, time.Second).Should(Equal(totalExpectedCount))

	// Create a map to count slices matching each expected spec
	specCounts := make(map[int]int)

	for _, slice := range sliceList.Items {
		if slice.Namespace != js.Namespace {
			continue
		}

		// Check if the Slice is owned by the JobSet.
		isOwned := false
		for _, ownerRef := range slice.OwnerReferences {
			if ownerRef.UID == js.UID {
				isOwned = true
				Expect(ownerRef.Kind).To(Equal("JobSet"))
				Expect(ownerRef.Name).To(Equal(js.Name))
				Expect(*ownerRef.Controller).To(BeTrue())
				break
			}
		}

		if isOwned {
			// Find which expected spec this slice matches
			matched := false
			for i, expected := range expectedSlices {
				diff := cmp.Diff(expected.SliceSpec, slice.Spec)
				if diff == "" {
					specCounts[i]++
					matched = true
					break
				}
			}

			// If no match found, show diffs against all expected specs for debugging
			if !matched {
				var diffs string
				for i, expected := range expectedSlices {
					diff := cmp.Diff(expected.SliceSpec, slice.Spec)
					diffs += fmt.Sprintf("\nDiff against expected spec %d:\n%s", i, diff)
				}
				Fail(fmt.Sprintf("Slice should match one of the expected specs.%s", diffs))
			}
		}
	}

	// Verify that the count of slices matching each spec equals the expected replicas
	for i, expected := range expectedSlices {
		Expect(specCounts[i]).To(Equal(expected.Replicas),
			"Expected %d slices matching spec %d, but found %d", expected.Replicas, i, specCounts[i])
	}
}

// assertSlicesNotCreated validates that no Slice resources were created for the given JobSet.
func assertSlicesNotCreated(ctx context.Context, js *jobset.JobSet) {
	Consistently(func() int {
		var sliceList v1alpha1.SliceList
		err := k8sClient.List(ctx, &sliceList)
		if err != nil {
			return 0
		}

		// Count Slices that belong to this JobSet.
		count := 0
		for _, slice := range sliceList.Items {
			if slice.Namespace == js.Namespace {
				// Check if the Slice is owned by the JobSet.
				for _, ownerRef := range slice.OwnerReferences {
					if ownerRef.UID == js.UID {
						count++
						break
					}
				}
			}
		}
		return count
	}, timeout, interval).Should(Equal(0))
}
