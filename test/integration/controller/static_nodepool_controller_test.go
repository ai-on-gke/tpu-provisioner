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
    numSubblocks: 2
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
				return provider.getStaticNodepoolsCreated("gsc-block-1")
			}, timeout, interval).Should(BeTrue())
		})
	})
})
