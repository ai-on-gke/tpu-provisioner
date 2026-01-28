package controllertest

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/controller"
)

var _ = Describe("Static Nodepool controller", func() {
	Context("when a valid static nodepool configmap is created", func() {
		It("should create the nodepools", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "reservation-1"
  gscBlocks:
  - name: "gsc-block-1"
    subblocks: "0001-0002"
    nodepoolPrefix: "test-nodepool"
`,
					"nodepoolConfig": `
machineType: "tpu7x"
`,
				},
			}

			By("Creating a configmap with static nodepools")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Checking that the nodepool was created")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "test-nodepool-0001" || np.Name == "test-nodepool-0002" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("when a valid static nodepool configmap is updated", func() {
		It("should update the nodepools", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "static-nodepool-config-to-update",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "reservation-to-update"
  gscBlocks:
  - name: "gsc-block-to-update"
    subblocks: "0001-0002"
    nodepoolPrefix: "update-test-nodepool"
`,
					"nodepoolConfig": `
machineType: "tpu7x"
`,
				},
			}

			By("Creating a configmap with static nodepools to be updated")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Checking that the initial nodepools were created")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				found1 := false
				found2 := false
				for _, np := range nodePools {
					if np.Name == "update-test-nodepool-0001" {
						found1 = true
					}
					if np.Name == "update-test-nodepool-0002" {
						found2 = true
					}
				}
				return found1 && found2
			}, timeout, interval).Should(BeTrue())

			// Update the configmap
			cm.Data["reservations"] = `
- name: "reservation-to-update"
  gscBlocks:
  - name: "gsc-block-to-update"
    subblocks: "0002-0003"
    nodepoolPrefix: "update-test-nodepool"
`
			By("Updating the configmap")
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("Checking that the nodepools were updated")
			Eventually(func() bool {
				// Check that the old nodepool is deleted
				_, deleted1 := provider.getDeleted("update-test-nodepool-0001")

				// Check that the new nodepool is created
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				found3 := false
				for _, np := range nodePools {
					if np.Name == "update-test-nodepool-0003" {
						found3 = true
					}
				}

				return deleted1 && found3
			}, timeout, interval).Should(BeTrue())

			// Check that the other nodepool is not deleted
			_, deleted2 := provider.getDeleted("update-test-nodepool-0002")
			Expect(deleted2).To(BeFalse())
		})
	})

	Context("when a valid static nodepool configmap is updated with different config", func() {
		It("should recreate the nodepools", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "static-nodepool-config-to-recreate",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "reservation-to-recreate"
  gscBlocks:
  - name: "gsc-block-to-recreate"
    subblocks: "0001"
    nodepoolPrefix: "recreate-test-nodepool"
`,
					"nodepoolConfig": `
machineType: "tpu-v4"
`,
				},
			}

			By("Creating a configmap with a static nodepool")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Checking that the initial nodepool was created")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "recreate-test-nodepool-0001" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Update the configmap
			cm.Data["nodepoolConfig"] = `
machineType: "tpu-v5"
`
			By("Updating the configmap with a new machine type")
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("Checking that the nodepool was recreated")
			Eventually(func() bool {
				// Check that the old nodepool is deleted
				_, deleted := provider.getDeleted("recreate-test-nodepool-0001")

				// Check that the new nodepool is created
				// The mock provider will create a new nodepool with the same name,
				// but in a real scenario, the old one is deleted and a new one is created.
				// Our mock provider simulates this by deleting the old one and creating a new one.
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				created := false
				for _, np := range nodePools {
					if np.Name == "recreate-test-nodepool-0001" {
						created = true
					}
				}

				return deleted && created
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("when a static nodepool is in use by a Slice", func() {
		It("should not delete the nodepool even if removed from config", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "res-slice"
  gscBlocks:
  - name: "block-slice"
    subblocks: "0001"
    nodepoolPrefix: "slice-test-np"
`,
					"nodepoolConfig": `
machineType: "tpu-v4"
`,
				},
			}

			By("Ensuring ConfigMap does not exist")
			_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
			})

			By("Creating configmap")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Waiting for nodepool creation")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "slice-test-np-0001" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Subblock ID constructed based on mock provider project ID and reservation config
			subblockID := "projects/test-project/reservations/res-slice/reservationBlocks/block-slice/reservationSubBlocks/block-slice-subblock-0001"

			By("Creating a Slice that uses this subblock")
			slice := &v1beta1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: v1beta1.SliceSpec{
					PartitionIds: []string{subblockID},
					Type:         "tpu7x",
					Topology:     "2x2x1",
				},
			}
			Expect(k8sClient.Create(ctx, slice)).To(Succeed())

			By("Updating configmap to remove the nodepool")
			cm.Data["reservations"] = ""
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("Verifying nodepool is NOT deleted")
			Consistently(func() bool {
				_, deleted := provider.getDeleted("slice-test-np-0001")
				return deleted
			}, 5*time.Second, interval).Should(BeFalse(), "Nodepool should not be deleted while Slice exists")

			By("Deleting the Slice")
			Expect(k8sClient.Delete(ctx, slice)).To(Succeed())

			By("Verifying nodepool IS deleted eventually")
			Eventually(func() bool {
				_, deleted := provider.getDeleted("slice-test-np-0001")
				return deleted
			}, timeout, interval).Should(BeTrue(), "Nodepool should be deleted after Slice is gone")
		})
	})
	Context("when a static nodepool is renamed while in use", func() {
		It("should not create the new nodepool until the old one releases capacity", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "res-race"
  gscBlocks:
  - name: "block-race"
    subblocks: "0001"
    nodepoolPrefix: "old-pool"
`,
					"nodepoolConfig": `
machineType: "tpu-v4"
`,
				},
			}

			// Clean up any existing configmap to avoid AlreadyExists errors
			_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
			})

			By("Creating initial configmap")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Waiting for old-pool-0001 creation")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "old-pool-0001" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Subblock ID constructed based on mock provider project ID and reservation config
			subblockID := "projects/test-project/reservations/res-race/reservationBlocks/block-race/reservationSubBlocks/block-race-subblock-0001"

			By("Creating a Slice that locks this subblock")
			slice := &v1beta1.Slice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lock-slice",
				},
				Spec: v1beta1.SliceSpec{
					PartitionIds: []string{subblockID},
					Type:         "tpu7x",
					Topology:     "2x2x1",
				},
			}
			Expect(k8sClient.Create(ctx, slice)).To(Succeed())

			By("Updating configmap to rename the nodepool (same subblock)")
			cm.Data["reservations"] = `
- name: "res-race"
  gscBlocks:
  - name: "block-race"
    subblocks: "0001"
    nodepoolPrefix: "new-pool"
`
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("Verifying old-pool-0001 is NOT deleted (protected by Slice)")
			Consistently(func() bool {
				_, deleted := provider.getDeleted("old-pool-0001")
				return deleted
			}, 5*time.Second, interval).Should(BeFalse(), "old-pool-0001 should NOT be deleted while Slice exists")

			By("Verifying new-pool-0001 is NOT created (capacity locked)")
			Consistently(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return true // error, assume not created or transient
				}
				for _, np := range nodePools {
					if np.Name == "new-pool-0001" {
						return true // Found it!
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeFalse(), "new-pool-0001 should NOT be created while old-pool-0001 holds the capacity")

			By("Deleting the Slice to release capacity")
			Expect(k8sClient.Delete(ctx, slice)).To(Succeed())

			By("Verifying old-pool-0001 IS deleted")
			Eventually(func() bool {
				_, deleted := provider.getDeleted("old-pool-0001")
				return deleted
			}, timeout, interval).Should(BeTrue(), "old-pool-0001 should be deleted after Slice is gone")

			By("Verifying new-pool-0001 IS created")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "new-pool-0001" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "new-pool-0001 should be created after capacity is released")
		})
	})

	Context("when creation fails initially", func() {
		It("should retry and eventually succeed (backoff)", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "res-retry"
  gscBlocks:
  - name: "block-retry"
    subblocks: "0001"
    nodepoolPrefix: "retry-pool"
`,
					"nodepoolConfig": `
machineType: "tpu-v4"
`,
				},
			}

			// Clean up previous
			_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
			})

			// Inject 2 failures
			provider.InjectFailure(2)
			provider.ResetCounters()

			By("Creating configmap")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Waiting for eventual success after retries")
			Eventually(func() int {
				return provider.EnsureCalls()
			}, 10*time.Second, interval).Should(BeNumerically(">=", 3), "Should have called Ensure at least 3 times (2 failures + 1 success)")

			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "retry-pool-0001" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "retry-pool-0001 should eventually be created")
		})
	})

	Context("when nodepool enters ERROR state", func() {
		It("should delete and recreate the nodepool", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "res-error-state"
  gscBlocks:
  - name: "block-error"
    subblocks: "0001"
    nodepoolPrefix: "error-pool"
`,
					"nodepoolConfig": `
machineType: "tpu-v4"
`,
				},
			}

			// Clean up previous
			_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ConfigMapName,
					Namespace: testNamespace,
				},
			})

			provider.ResetCounters()

			By("Creating configmap")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Waiting for successful creation")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					if np.Name == "error-pool-0001" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "error-pool-0001 should be created")

			By("Injecting ERROR state")
			provider.SetErrorState("error-pool-0001", true)

			By("Waiting for deletion (recovery)")
			Eventually(func() bool {
				_, deleted := provider.getDeleted("error-pool-0001")
				return deleted
			}, timeout, interval).Should(BeTrue(), "error-pool-0001 should be deleted when in ERROR state")

			By("Waiting for recreation")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				for _, np := range nodePools {
					// We expect it to be present and NOT in error state (SetErrorState clears on delete/recreate usually?
					// Wait, mockProvider Delete clears error state. So recreation should be clean.
					if np.Name == "error-pool-0001" && !np.Error {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "error-pool-0001 should be recreated and healthy")
		})
	})
})
