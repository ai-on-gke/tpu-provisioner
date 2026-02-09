package webhook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/utils"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"
)

func TestLWSStatefulSetMutationHandler_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = lws.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	lwsObj := &lws.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-lws",
			Namespace: "default",
			UID:       "lws-uid-12345",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(lwsObj).Build()

	handler := &LWSStatefulSetMutationHandler{
		Client:  client,
		Decoder: decoder,
	}

	tests := []struct {
		name           string
		sts            *appsv1.StatefulSet
		expectedMutate bool
		expectedVal    string
	}{
		{
			name: "should mutate worker sts with correct labels and owner",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts-worker",
					Namespace: "default",
					Labels: map[string]string{
						LWSNameLabel:       "test-lws",
						LWSGroupIndexLabel: "1",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "LeaderWorkerSet",
							UID:  "lws-uid-12345",
						},
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								InjectSliceSelectorLabel: "true",
							},
						},
					},
				},
			},
			expectedMutate: true,
			expectedVal:    utils.LWSSliceName("test-lws", "lws-uid-12345", "worker", 1),
		},
		{
			name: "should mutate worker sts with correct labels even if owner is not LWS",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts-worker-no-owner",
					Namespace: "default",
					Labels: map[string]string{
						LWSNameLabel:       "test-lws",
						LWSGroupIndexLabel: "2",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Pod",
							Name: "leader-pod",
							UID:  "pod-uid-67890",
						},
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								InjectSliceSelectorLabel: "true",
							},
						},
					},
				},
			},
			expectedMutate: true,
			expectedVal:    utils.LWSSliceName("test-lws", "lws-uid-12345", "worker", 2),
		},
		{
			name: "should mutate leader sts with correct labels and owner",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts-leader",
					Namespace: "default",
					Labels: map[string]string{
						LWSNameLabel: "test-lws",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "LeaderWorkerSet",
							UID:  "lws-uid-12345",
						},
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								InjectSliceSelectorLabel: "true",
							},
						},
					},
				},
			},
			expectedMutate: true,
			expectedVal:    utils.LWSSliceName("test-lws", "lws-uid-12345", "leader", -1),
		},
		{
			name: "should not mutate if inject label is missing",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LWSNameLabel:       "test-lws",
						LWSGroupIndexLabel: "1",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								// InjectSliceSelectorLabel missing
							},
						},
					},
				},
			},
			expectedMutate: false,
		},
		{
			name: "should not mutate if LWS owner is missing",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LWSNameLabel:       "test-lws",
						LWSGroupIndexLabel: "1",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								InjectSliceSelectorLabel: "true",
							},
						},
					},
				},
			},
			expectedMutate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, _ := json.Marshal(tt.sts)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: raw},
				},
			}

			resp := handler.Handle(context.Background(), req)

			if tt.expectedMutate {
				if !resp.Allowed {
					t.Fatalf("expected allowed, got %v", resp.Result.Message)
				}
				if len(resp.Patches) == 0 {
					t.Fatal("expected patches, got none")
				}
				// Verify node selector in patch (simplified check)
				found := false
				for _, p := range resp.Patches {
					if p.Operation == "add" && p.Path == "/spec/template/spec/nodeSelector" {
						val := p.Value.(map[string]interface{})
						if val[SliceNodeSelector] == tt.expectedVal {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("expected node selector patch with value %s, not found in patches: %+v", tt.expectedVal, resp.Patches)
				}
			} else {
				if len(resp.Patches) > 0 {
					t.Errorf("expected no patches, got %d", len(resp.Patches))
				}
			}
		})
	}
}
