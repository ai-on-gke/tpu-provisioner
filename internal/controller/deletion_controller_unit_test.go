package controller

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	"google.golang.org/api/googleapi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type errorMockProvider struct {
	cloud.Mock
	err error
}

func (p *errorMockProvider) DeleteNodePoolForNode(node *corev1.Node, why string) error {
	return p.err
}

func TestDeleteNodePool(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "retry on 429",
			err:     &googleapi.Error{Code: http.StatusTooManyRequests},
			wantErr: true,
		},
		{
			name:    "retry on 503",
			err:     &googleapi.Error{Code: http.StatusServiceUnavailable},
			wantErr: true,
		},
		{
			name:    "retry on 504",
			err:     &googleapi.Error{Code: http.StatusGatewayTimeout},
			wantErr: true,
		},
		{
			name:    "retry on 500",
			err:     &googleapi.Error{Code: http.StatusInternalServerError},
			wantErr: true,
		},
		{
			name:    "retry on 408",
			err:     &googleapi.Error{Code: http.StatusRequestTimeout},
			wantErr: true,
		},
		{
			name:    "retry on other googleapi error",
			err:     &googleapi.Error{Code: http.StatusBadRequest},
			wantErr: true,
		},
		{
			name:    "retry on generic error",
			err:     errors.New("generic error"),
			wantErr: true,
		},
		{
			name:    "no retry on duplicate request",
			err:     cloud.ErrDuplicateRequest,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DeletionReconciler{
				Provider: &errorMockProvider{err: tt.err},
			}
			_, err := r.deleteNodePool(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}}, "test-reason")
			if (err != nil) != tt.wantErr {
				t.Errorf("deleteNodePool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
