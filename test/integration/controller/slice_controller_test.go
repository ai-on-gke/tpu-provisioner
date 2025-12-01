package controllertest

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1alpha1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/controller"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
				withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withReplicatedJob("worker", 2, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "2x2x4",
						PartitionIds: nil,
					},
					Replicas: 2,
				},
			},
		}),
		Entry("JobSet with slice provisioning enabled but no v7x accelerator should not create Slices", &testCase{
			jobSet: constructJobSet("test-js-2",
				withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu-v6e", "2x2x4")),
			),
			wantSliceCreation: false,
		}),
		Entry("JobSet without slice provisioning annotation should not create Slices", &testCase{
			jobSet: constructJobSet("test-js-3",
				withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
			),
			wantSliceCreation: false,
		}),
		Entry("JobSet with slice provisioning but auto-provisioning disabled should not create Slices", &testCase{
			jobSet: constructJobSet("test-js-4",
				withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withLabel(utils.DisableAutoProvisioningLabel, "true"),
				withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
			),
			wantSliceCreation: false,
		}),
		Entry("JobSet with slice provisioning and multiple replicas should create multiple Slices", &testCase{
			jobSet: constructJobSet("test-js-5",
				withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withReplicatedJob("worker", 3, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "2x2x4",
						PartitionIds: nil,
					},
					Replicas: 3,
				},
			},
		}),
		Entry("JobSet with slice provisioning and 2 replicated jobs should create Slices for both", &testCase{
			jobSet: constructJobSet("test-js-6",
				withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withReplicatedJob("worker-1", 2, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
				withReplicatedJob("worker-2", 1, makeJobTemplateWithTPU("tpu7x", "2x2x2")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "2x2x4",
						PartitionIds: nil,
					},
					Replicas: 2,
				},
				{
					SliceSpec: v1alpha1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "2x2x2",
						PartitionIds: nil,
					},
					Replicas: 1,
				},
			},
		}),
		Entry("JobSet with slice-selection annotation should create Slices with PartitionIds", &testCase{
			jobSet: constructJobSet("test-js-7",
				withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withAnnotation(controller.SliceSelectionAnnotation, `{"worker":[["cube-1","cube-2"],["cube-3","cube-4"]]}`),
				withReplicatedJob("worker", 2, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1alpha1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "2x2x4",
						PartitionIds: []string{"cube-1", "cube-2"},
					},
					Replicas: 1,
				},
				{
					SliceSpec: v1alpha1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "2x2x4",
						PartitionIds: []string{"cube-3", "cube-4"},
					},
					Replicas: 1,
				},
			},
		}),
	)

	It("should recreate Slices when slice-selection annotation changes", func() {
		ctx := context.Background()
		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ns-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Clean up temporary namespace after test
		defer func() {
			Expect(deleteNamespace(ctx, k8sClient, ns)).To(Succeed())
		}()

		// Create JobSet with initial slice selection
		js := constructJobSet("test-js-update",
			withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
			withAnnotation(controller.SliceSelectionAnnotation, `{"worker":[["cube-1","cube-2"]]}`),
			withReplicatedJob("worker", 1, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
		)
		js.Namespace = ns.Name

		By("Creating JobSet with initial slice selection")
		Expect(k8sClient.Create(ctx, js)).To(Succeed())

		By("Verifying initial Slices are created")
		initialExpectedSlices := []ExpectedSliceSpec{
			{
				SliceSpec: v1alpha1.SliceSpec{
					Type:         "tpu7x",
					Topology:     "2x2x4",
					PartitionIds: []string{"cube-1", "cube-2"},
				},
				Replicas: 1,
			},
		}
		assertSlicesCreated(ctx, js, initialExpectedSlices)

		By("Updating slice selection annotation")
		// Fetch the latest version of the JobSet
		var updatedJS jobset.JobSet
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(js), &updatedJS)).To(Succeed())

		// Update the annotation to select different cubes
		updatedJS.Annotations[controller.SliceSelectionAnnotation] = `{"worker":[["cube-3","cube-4"]]}`
		Expect(k8sClient.Update(ctx, &updatedJS)).To(Succeed())

		By("Verifying Slices are recreated with updated PartitionIds")
		updatedExpectedSlices := []ExpectedSliceSpec{
			{
				SliceSpec: v1alpha1.SliceSpec{
					Type:         "tpu7x",
					Topology:     "2x2x4",
					PartitionIds: []string{"cube-3", "cube-4"},
				},
				Replicas: 1,
			},
		}
		assertSlicesCreated(ctx, &updatedJS, updatedExpectedSlices)
	})

	It("should delete Slices when owning JobSet is deleted", func() {
		ctx := context.Background()
		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ns-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Clean up temporary namespace after test
		defer func() {
			Expect(deleteNamespace(ctx, k8sClient, ns)).To(Succeed())
		}()

		// Create JobSet with slice provisioning
		js := constructJobSet("test-js-deletion",
			withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
			withReplicatedJob("worker", 2, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
		)
		js.Namespace = ns.Name

		By("Creating JobSet")
		Expect(k8sClient.Create(ctx, js)).To(Succeed())

		By("Verifying Slices are created")
		expectedSlices := []ExpectedSliceSpec{
			{
				SliceSpec: v1alpha1.SliceSpec{
					Type:         "tpu7x",
					Topology:     "2x2x4",
					PartitionIds: nil,
				},
				Replicas: 2,
			},
		}
		assertSlicesCreated(ctx, js, expectedSlices)

		By("Deleting the JobSet")
		Expect(k8sClient.Delete(ctx, js)).To(Succeed())

		By("Verifying Slices are deleted")
		Eventually(func() int {
			var sliceList v1alpha1.SliceList
			err := k8sClient.List(ctx, &sliceList)
			if err != nil {
				return -1
			}
			// Count slices owned by this JobSet
			count := 0
			for _, slice := range sliceList.Items {
				if slice.Labels != nil {
					isOwnedByJobSet := slice.Labels[controller.SliceOwnerKindLabel] == "jobset"
					jobsetName, hasName := slice.Labels[controller.SliceOwnerNameLabel]
					jobsetNamespace, hasNamespace := slice.Labels[controller.SliceOwnerNamespaceLabel]
					if isOwnedByJobSet && hasName && hasNamespace && jobsetName == js.Name && jobsetNamespace == js.Namespace {
						count++
					}
				}
			}
			return count
		}, 10*time.Second, time.Second).Should(Equal(0))
	})

	It("should suspend JobSet in sync mode until all Slices are Ready", func() {
		ctx := context.Background()
		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ns-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Clean up temporary namespace after test
		defer func() {
			Expect(deleteNamespace(ctx, k8sClient, ns)).To(Succeed())
		}()

		// Create JobSet with sync mode slice provisioning
		js := constructJobSet("test-js-sync",
			withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeSync),
			withReplicatedJob("worker", 2, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
		)
		js.Namespace = ns.Name

		By("Creating JobSet with sync mode")
		Expect(k8sClient.Create(ctx, js)).To(Succeed())

		By("Verifying Slices are created")
		expectedSlices := []ExpectedSliceSpec{
			{
				SliceSpec: v1alpha1.SliceSpec{
					Type:         "tpu7x",
					Topology:     "2x2x4",
					PartitionIds: nil,
				},
				Replicas: 2,
			},
		}
		assertSlicesCreated(ctx, js, expectedSlices)

		By("Verifying JobSet is suspended because Slices are not Ready")
		Eventually(func() bool {
			var updatedJS jobset.JobSet
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(js), &updatedJS)
			if err != nil {
				return false
			}
			return updatedJS.Spec.Suspend != nil && *updatedJS.Spec.Suspend
		}, 5*time.Second, time.Second).Should(BeTrue())

		By("Marking all Slices as Ready")
		var sliceList v1alpha1.SliceList
		err := k8sClient.List(ctx, &sliceList,
			client.MatchingLabels{
				controller.SliceOwnerKindLabel:      "jobset",
				controller.SliceOwnerNameLabel:      js.Name,
				controller.SliceOwnerNamespaceLabel: js.Namespace,
			})
		Expect(err).ToNot(HaveOccurred())
		for _, slice := range sliceList.Items {
			slice.Status.Conditions = []metav1.Condition{
				{
					Type:               v1alpha1.SliceStateConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					Message:            "Slice is ready",
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, &slice)).To(Succeed())
		}

		By("Verifying JobSet is unsuspended after all Slices are Ready")
		Eventually(func() bool {
			var updatedJS jobset.JobSet
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(js), &updatedJS)
			if err != nil {
				return true // Return true to fail the test if we can't get the JobSet
			}
			return updatedJS.Spec.Suspend == nil || !*updatedJS.Spec.Suspend
		}, 5*time.Second, time.Second).Should(BeTrue())
	})

	It("should not suspend JobSet in async mode", func() {
		ctx := context.Background()
		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ns-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Clean up temporary namespace after test
		defer func() {
			Expect(deleteNamespace(ctx, k8sClient, ns)).To(Succeed())
		}()

		// Create JobSet with async mode slice provisioning
		js := constructJobSet("test-js-async",
			withLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
			withReplicatedJob("worker", 2, makeJobTemplateWithTPU("tpu7x", "2x2x4")),
		)
		js.Namespace = ns.Name

		By("Creating JobSet with async mode")
		Expect(k8sClient.Create(ctx, js)).To(Succeed())

		By("Verifying Slices are created")
		expectedSlices := []ExpectedSliceSpec{
			{
				SliceSpec: v1alpha1.SliceSpec{
					Type:         "tpu7x",
					Topology:     "2x2x4",
					PartitionIds: nil,
				},
				Replicas: 2,
			},
		}
		assertSlicesCreated(ctx, js, expectedSlices)

		By("Verifying JobSet remains unsuspended in async mode")
		Consistently(func() bool {
			var updatedJS jobset.JobSet
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(js), &updatedJS)
			if err != nil {
				return true // Return true to fail the test if we can't get the JobSet
			}
			// JobSet should remain unsuspended (nil or false)
			return updatedJS.Spec.Suspend == nil || !*updatedJS.Spec.Suspend
		}, 3*time.Second, time.Second).Should(BeTrue())
	})
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

// withLabel adds a label to the JobSet.
func withLabel(key, value string) JobSetOption {
	return func(js *jobset.JobSet) {
		if js.Labels == nil {
			js.Labels = make(map[string]string)
		}
		js.Labels[key] = value
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

	Eventually(func() error {
		var sliceList v1alpha1.SliceList
		err := k8sClient.List(ctx, &sliceList)
		if err != nil {
			return fmt.Errorf("failed to list slices: %w", err)
		}

		// Count Slices that belong to this JobSet and create a map to count slices matching each expected spec
		count := 0
		specCounts := make(map[int]int)

		for _, slice := range sliceList.Items {
			// Check if the Slice is owned by the JobSet via labels.
			isOwned := false
			if slice.Labels != nil {
				jobsetName, hasName := slice.Labels[controller.SliceOwnerNameLabel]
				jobsetNamespace, hasNamespace := slice.Labels[controller.SliceOwnerNamespaceLabel]
				if hasName && hasNamespace && jobsetName == js.Name && jobsetNamespace == js.Namespace {
					isOwned = true
					count++
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
					return fmt.Errorf("slice should match one of the expected specs.%s", diffs)
				}
			}
		}

		// Verify count matches expected
		if count != totalExpectedCount {
			return fmt.Errorf("expected %d slices, got %d", totalExpectedCount, count)
		}

		// Verify that the count of slices matching each spec equals the expected replicas
		for i, expected := range expectedSlices {
			if specCounts[i] != expected.Replicas {
				return fmt.Errorf("expected %d slices matching spec %d, but found %d", expected.Replicas, i, specCounts[i])
			}
		}

		return nil
	}, 3*time.Second, time.Second).Should(Succeed())
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
			// Check if the Slice is owned by the JobSet via labels.
			if slice.Labels != nil {
				jobsetName, hasName := slice.Labels[controller.SliceOwnerNameLabel]
				jobsetNamespace, hasNamespace := slice.Labels[controller.SliceOwnerNamespaceLabel]
				if hasName && hasNamespace && jobsetName == js.Name && jobsetNamespace == js.Namespace {
					count++
				}
			}
		}
		return count
	}, timeout, interval).Should(Equal(0))
}
