package controllertest

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/api/googleapi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Controller Backoff", func() {
	var ns *corev1.Namespace

	BeforeEach(func() {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-backoff-",
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())
		provider.ResetCounters()
		provider.EnsureError = nil
		provider.DeleteError = nil
	})

	AfterEach(func() {
		Expect(deleteNamespace(context.Background(), k8sClient, ns)).To(Succeed())
		provider.EnsureError = nil
		provider.DeleteError = nil
	})

	Context("Creation controller", func() {
		It("should retry with exponential backoff when provider fails", func() {
			provider.EnsureError = errors.New("temporary failure")

			// JobSet name must match what's in makeLeaderPod() which is "test-js"
			testJS := makeJobSet("test-js")
			testJS.Namespace = ns.Name
			Expect(k8sClient.Create(context.Background(), testJS)).To(Succeed())

			pod := makeLeaderPod()
			pod.Namespace = ns.Name
			Expect(k8sClient.Create(context.Background(), pod)).To(Succeed())

			updatePodStatus(context.Background(), k8sClient, pod, *makePendingStatus())

			// First call should happen almost immediately.
			Eventually(func() int {
				return provider.EnsureCalls()
			}, timeout, interval).Should(BeNumerically(">=", 1))

			// With 5s base delay, the second call should happen around 5s after the first.
			// We wait a bit more to be sure.
			// Note: controller-runtime's default rate limiter might have some jitter or overhead.
			Eventually(func() int {
				return provider.EnsureCalls()
			}, 10*time.Second, interval).Should(BeNumerically(">=", 2), "Should have retried at least once after 10 seconds")
		})
	})

	Context("Deletion controller", func() {
		type deleteTestCase struct {
			deleteError error
			nodeName    string
		}

		DescribeTable("should retry with exponential backoff when provider fails",
			func(tc deleteTestCase) {
				provider.DeleteError = tc.deleteError
				node := makeNodeWithLabels(tc.nodeName, map[string]string{
					cloud.LabelNodepoolManager: cloud.LabelNodepoolManagerTPUPodinator,
					cloud.GKENodePoolNameLabel: "test-nodepool",
					cloud.LabelJobSetName:      "non-existent-jobset",
					cloud.LabelJobSetNamespace: ns.Name,
				})

				By("Creating a Node")
				Expect(k8sClient.Create(context.Background(), node)).To(Succeed())

				// First call should happen almost immediately.
				Eventually(func() int {
					return provider.DeleteCalls()
				}, timeout, interval).Should(BeNumerically(">=", 1))

				// With 5s base delay, the second call should happen around 5s after the first.
				Eventually(func() int {
					return provider.DeleteCalls()
				}, 10*time.Second, interval).Should(BeNumerically(">=", 2), "Should have retried at least once after 10 seconds")
			},
			Entry("standard error", deleteTestCase{
				deleteError: errors.New("temporary failure"),
				nodeName:    "node-to-delete",
			}),
			Entry("specific Google API error code (e.g., 429)", deleteTestCase{
				deleteError: &googleapi.Error{Code: http.StatusTooManyRequests, Message: "rate limit"},
				nodeName:    "node-to-delete-429",
			}),
			Entry("other Google API error codes (default handling)", deleteTestCase{
				deleteError: &googleapi.Error{Code: http.StatusBadRequest, Message: "bad request"},
				nodeName:    "node-to-delete-400",
			}),
		)
	})
})
