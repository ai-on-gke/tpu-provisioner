package controllertest

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	Context("when a valid static nodepool configmap is deleted", func() {
		It("should delete the nodepools", func() {
			ctx := context.Background()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "static-nodepool-config-to-delete",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"reservations": `
- name: "reservation-to-delete"
  gscBlocks:
  - name: "gsc-block-to-delete"
    subblocks: "0001-0002"
    nodepoolPrefix: "delete-test-nodepool"
`,
					"nodepoolConfig": `
machineType: "tpu7x"
`,
				},
			}

			By("Creating a configmap with static nodepools to be deleted")
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Checking that the nodepools were created")
			Eventually(func() bool {
				nodePools, err := provider.ListNodePools()
				if err != nil {
					return false
				}
				found1 := false
				found2 := false
				for _, np := range nodePools {
					if np.Name == "delete-test-nodepool-0001" {
						found1 = true
					}
					if np.Name == "delete-test-nodepool-0002" {
						found2 = true
					}
				}
				return found1 && found2
			}, timeout, interval).Should(BeTrue())

			// Assert that they are not deleted yet
			_, deleted1 := provider.getDeleted("delete-test-nodepool-0001")
			Expect(deleted1).To(BeFalse())
			_, deleted2 := provider.getDeleted("delete-test-nodepool-0002")
			Expect(deleted2).To(BeFalse())

			By("Deleting the configmap")
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())

			By("Checking that the nodepools were deleted")
			Eventually(func() bool {
				_, deleted1 := provider.getDeleted("delete-test-nodepool-0001")
				_, deleted2 := provider.getDeleted("delete-test-nodepool-0002")
				return deleted1 && deleted2
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

})
