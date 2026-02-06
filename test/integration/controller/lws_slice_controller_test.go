package controllertest

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/controller"
	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"
)

var _ = Describe("LeaderWorkerSet Slice controller", func() {

	type testCase struct {
		lwset             *lws.LeaderWorkerSet
		wantSliceCreation bool
		expectedSlices    []ExpectedSliceSpec
	}

	DescribeTable("LeaderWorkerSets are created and Slices are reconciled",
		func(tc *testCase) {
			ctx := context.Background()
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-lws-ns-",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			defer func() {
				Expect(deleteNamespace(ctx, k8sClient, ns)).To(Succeed())
			}()

			lwset := tc.lwset
			lwset.Namespace = ns.Name

			By(fmt.Sprintf("Creating LeaderWorkerSet %s", lwset.Name))
			Expect(k8sClient.Create(ctx, lwset)).To(Succeed())

			if tc.wantSliceCreation {
				By("Checking that the LeaderWorkerSet triggered Slice creation")
				assertLWSSlicesCreated(ctx, lwset, tc.expectedSlices)
			} else {
				By("Checking that LeaderWorkerSet did not trigger Slice creation")
				assertLWSSlicesNotCreated(ctx, lwset)
			}
		},
		Entry("LWS with slice provisioning enabled should create Slices", &testCase{
			lwset: constructLWS("test-lws-1",
				withLWSLabel(utils.SliceProvisioningLabel, utils.SliceProvisioningModeAsync),
				withLWSAnnotation(controller.SliceSelectionAnnotation, `{"test-lws-1":[["lws-cube-1"]]}`),
				withLWSTPU("tpu7x", "4x4x4"),
				withLWSTemplates(1),
			),
			wantSliceCreation: true,
			expectedSlices: []ExpectedSliceSpec{
				{
					SliceSpec: v1beta1.SliceSpec{
						Type:         "tpu7x",
						Topology:     "4x4x4",
						PartitionIds: []string{"lws-cube-1"},
					},
					Replicas: 1,
				},
			},
		}),
		Entry("LWS without slice provisioning label should not create Slices", &testCase{
			lwset: constructLWS("test-lws-2",
				withLWSTPU("tpu7x", "4x4x4"),
				withLWSTemplates(1),
			),
			wantSliceCreation: false,
		}),
	)
})

func constructLWS(name string, opts ...LWSOption) *lws.LeaderWorkerSet {
	lwset := &lws.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: lws.LeaderWorkerSetSpec{
			RolloutStrategy: lws.RolloutStrategy{
				Type: lws.RollingUpdateStrategyType,
			},
			StartupPolicy: lws.LeaderCreatedStartupPolicy,
			LeaderWorkerTemplate: lws.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "worker",
								Image: "test-image",
							},
						},
					},
				},
			},
		},
	}
	for _, opt := range opts {
		opt(lwset)
	}
	return lwset
}

type LWSOption func(*lws.LeaderWorkerSet)

func withLWSLabel(key, value string) LWSOption {
	return func(l *lws.LeaderWorkerSet) {
		if l.Labels == nil {
			l.Labels = make(map[string]string)
		}
		l.Labels[key] = value
	}
}

func withLWSAnnotation(key, value string) LWSOption {
	return func(l *lws.LeaderWorkerSet) {
		if l.Annotations == nil {
			l.Annotations = make(map[string]string)
		}
		l.Annotations[key] = value
	}
}

func withLWSTPU(accel, topo string) LWSOption {
	return func(l *lws.LeaderWorkerSet) {
		if l.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector == nil {
			l.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector = make(map[string]string)
		}
		l.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector["cloud.google.com/gke-tpu-accelerator"] = accel
		if l.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations == nil {
			l.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations = make(map[string]string)
		}
		l.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations["cloud.google.com/gke-tpu-topology"] = topo
	}
}

func withLWSTemplates(replicas int32) LWSOption {
	return func(l *lws.LeaderWorkerSet) {
		l.Spec.Replicas = &replicas
	}
}

func assertLWSSlicesCreated(ctx context.Context, lwset *lws.LeaderWorkerSet, expectedSlices []ExpectedSliceSpec) {
	Eventually(func() error {
		var sliceList v1beta1.SliceList
		err := k8sClient.List(ctx, &sliceList)
		if err != nil {
			return err
		}
		count := 0
		for _, slice := range sliceList.Items {
			if slice.Labels != nil &&
				slice.Labels[controller.SliceOwnerNameLabel] == lwset.Name &&
				slice.Labels[controller.SliceOwnerKindLabel] == controller.LWSOwnerKind {
				count++
			}
		}
		totalExpected := 0
		for _, e := range expectedSlices {
			totalExpected += e.Replicas
		}
		if count != totalExpected {
			return fmt.Errorf("expected %d slices, got %d", totalExpected, count)
		}
		return nil
	}, 10*time.Second, time.Second).Should(Succeed())
}

func assertLWSSlicesNotCreated(ctx context.Context, lwset *lws.LeaderWorkerSet) {
	Consistently(func() int {
		var sliceList v1beta1.SliceList
		k8sClient.List(ctx, &sliceList)
		count := 0
		for _, slice := range sliceList.Items {
			if slice.Labels != nil &&
				slice.Labels[controller.SliceOwnerNameLabel] == lwset.Name &&
				slice.Labels[controller.SliceOwnerKindLabel] == controller.LWSOwnerKind {
				count++
			}
		}
		return count
	}, 3*time.Second, time.Second).Should(Equal(0))
}
