package controllertest

import (
	"context"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/internal/cloud"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ cloud.Provider = &mockProvider{}

type mockProvider struct {
	sync.Mutex
	created                map[types.NamespacedName]bool
	deleted                map[string]time.Time
	staticNodepoolsCreated map[string]bool

	cloud.Provider
}

func (p *mockProvider) NodePoolLabelKey() string { return cloud.GKENodePoolNameLabel }

func (p *mockProvider) EnsureNodePoolForPod(pod *corev1.Pod, _ string) error {
	p.Lock()
	defer p.Unlock()
	p.created[types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}] = true
	return nil
}

func (p *mockProvider) EnsureStaticNodePools(ctx context.Context, reservationName, gscBlockName, nodepoolPrefix string, subblocks string, nodepoolConfig *cloud.StaticNodePoolConfig, concurrency int, _ client.Object) error {
	p.Lock()
	defer p.Unlock()
	p.staticNodepoolsCreated[gscBlockName] = true
	return nil
}

func (p *mockProvider) getCreated(nn types.NamespacedName) bool {
	p.Lock()
	defer p.Unlock()
	return p.created[nn]
}

func (p *mockProvider) getStaticNodepoolsCreated(gscBlockName string) bool {
	p.Lock()
	defer p.Unlock()
	return p.staticNodepoolsCreated[gscBlockName]
}

func (p *mockProvider) DeleteNodePoolForNode(node *corev1.Node, why string) error {
	return p.DeleteNodePool(node.Name, node, why)
}

func (p *mockProvider) ListNodePools() ([]cloud.NodePoolRef, error) {
	return []cloud.NodePoolRef{}, nil
}

func (p *mockProvider) DeleteNodePool(name string, eventObj client.Object, why string) error {
	p.Lock()
	defer p.Unlock()
	if _, exists := p.deleted[name]; !exists {
		p.deleted[name] = time.Now()
	}
	return nil
}

func (p *mockProvider) getDeleted(name string) (time.Time, bool) {
	p.Lock()
	defer p.Unlock()
	timestamp, exists := p.deleted[name]
	return timestamp, exists
}

func (p *mockProvider) DeleteStaticNodePools(ctx context.Context, nodepoolNames []string, concurrency int, eventObj client.Object, why string) []error {
	for _, name := range nodepoolNames {
		p.DeleteNodePool(name, eventObj, why)
	}
	return nil
}
