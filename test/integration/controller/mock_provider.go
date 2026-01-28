package controllertest

import (
	"context"
	"fmt" // Added for injected failure
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
	gke                    *cloud.GKE
	created                map[types.NamespacedName]bool
	deleted                map[string]time.Time
	staticNodepoolsCreated map[string]cloud.NodePoolRef
	ensureCalls            int
	deleteCalls            int
	// Test hooks
	errorStates       map[string]bool
	failuresRemaining int
}

func newMockProvider(gke *cloud.GKE) *mockProvider {
	return &mockProvider{
		gke:                    gke,
		created:                make(map[types.NamespacedName]bool),
		deleted:                make(map[string]time.Time),
		staticNodepoolsCreated: make(map[string]cloud.NodePoolRef),
		errorStates:            make(map[string]bool),
	}
}

func (p *mockProvider) ResetCounters() {
	p.Lock()
	defer p.Unlock()
	p.ensureCalls = 0
	p.deleteCalls = 0
}

func (p *mockProvider) EnsureCalls() int {
	p.Lock()
	defer p.Unlock()
	return p.ensureCalls
}

func (p *mockProvider) DeleteCalls() int {
	p.Lock()
	defer p.Unlock()
	return p.deleteCalls
}

func (p *mockProvider) NodePoolLabelKey() string { return cloud.GKENodePoolNameLabel }

func (p *mockProvider) ProjectID() string { return "test-project" }

func (p *mockProvider) EnsureNodePoolForPod(pod *corev1.Pod, _ string) error {
	p.Lock()
	defer p.Unlock()
	p.created[types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}] = true
	return nil
}

func (p *mockProvider) DiffStaticNodePools(existingNodepools []cloud.NodePoolRef, desiredNodepools []*cloud.DesiredStaticNodePool) ([]*cloud.DesiredStaticNodePool, []string, []string, []string, error) {
	return p.gke.DiffStaticNodePools(existingNodepools, desiredNodepools)
}

func (p *mockProvider) EnsureStaticNodePools(ctx context.Context, desiredNodePools []*cloud.DesiredStaticNodePool, concurrency int, _ client.Object) error {
	p.Lock()
	p.ensureCalls++
	if p.failuresRemaining > 0 {
		p.failuresRemaining--
		p.Unlock()
		return fmt.Errorf("injected failure")
	}
	p.Unlock()

	for _, desired := range desiredNodePools {
		np, err := p.gke.StaticNodePoolForSubBlock(desired.Name, desired.SubblockToConsume, desired.Config)
		if err != nil {
			return err
		}

		p.Lock()
		p.staticNodepoolsCreated[desired.Name] = cloud.NodePoolRef{
			Name:          desired.Name,
			Labels:        np.Config.Labels,
			SubblockNames: np.Config.ReservationAffinity.Values,
		}
		// If it was in error, fixing it clears the error?
		// For the test, we might want manual clearing or auto-clearing.
		// Let's assume a successful Ensure clears the error state (recreated).
		delete(p.errorStates, desired.Name)
		p.Unlock()
	}
	return nil
}

func (p *mockProvider) DeleteStaticNodePools(ctx context.Context, nodepoolNames []string, concurrency int, eventObj client.Object, why string) []error {
	p.Lock()
	p.deleteCalls++
	p.Unlock()

	for _, name := range nodepoolNames {
		p.DeleteNodePool(name, eventObj, why)
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
		if p.errorStates[ref.Name] {
			ref.Error = true
			ref.Message = "Simulated Error"
		}
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
	delete(p.errorStates, name) // Clear error state on delete
	return nil
}

func (p *mockProvider) getDeleted(name string) (time.Time, bool) {
	p.Lock()
	defer p.Unlock()
	timestamp, exists := p.deleted[name]
	return timestamp, exists
}

// SetErrorState allows simulating a nodepool in ERROR state
func (p *mockProvider) SetErrorState(name string, isError bool) {
	p.Lock()
	defer p.Unlock()
	if isError {
		p.errorStates[name] = true
	} else {
		delete(p.errorStates, name)
	}
}

// InjectFailure causes EnsureStaticNodePools to return an error for N calls
func (p *mockProvider) InjectFailure(count int) {
	p.Lock()
	defer p.Unlock()
	p.failuresRemaining = count
}
