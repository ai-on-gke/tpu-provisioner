package controllertest

import (
	"context"
	"fmt"
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
	staticNodepoolsCreated map[string]cloud.NodePoolRef

	cloud.Provider
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		created:                make(map[types.NamespacedName]bool),
		deleted:                make(map[string]time.Time),
		staticNodepoolsCreated: make(map[string]cloud.NodePoolRef),
	}
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

	// Parse subblocks and generate nodepool names
	start, end, err := cloud.ParseSubBlocks(subblocks)
	if err != nil {
		return fmt.Errorf("parsing subblocks in mock: %w", err)
	}

	for i := start; i <= end; i++ {
		formattedSubblockIndex := fmt.Sprintf("%04d", i)
		nodePoolID := fmt.Sprintf("%s-%s", nodepoolPrefix, formattedSubblockIndex)

		p.staticNodepoolsCreated[nodePoolID] = cloud.NodePoolRef{
			Name: nodePoolID,
			Labels: map[string]string{
				cloud.LabelTPUProvisionerStaticNodepool: "true",
			},
			CreationTime: time.Now(),
		}
	}
	return nil
}

func (p *mockProvider) getCreated(nn types.NamespacedName) bool {
	p.Lock()
	defer p.Unlock()
	return p.created[nn]
}

func (p *mockProvider) DeleteNodePoolForNode(node *corev1.Node, why string) error {
	return p.DeleteNodePool(node.Name, node, why)
}

func (p *mockProvider) ListNodePools() ([]cloud.NodePoolRef, error) {
	p.Lock()
	defer p.Unlock()
	var refs []cloud.NodePoolRef
	for _, ref := range p.staticNodepoolsCreated {
		refs = append(refs, ref)
	}
	return refs, nil
}

func (p *mockProvider) DeleteNodePool(name string, eventObj client.Object, why string) error {
	p.Lock()
	defer p.Unlock()
	if _, exists := p.deleted[name]; !exists {
		p.deleted[name] = time.Now()
	}
	delete(p.staticNodepoolsCreated, name)
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
