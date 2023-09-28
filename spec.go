package gingk8s

import (
	"context"
	"fmt"
	"sync"

	"github.com/meln5674/godag"
)

var cleanLock = sync.Mutex{}

type specAction interface {
	Setup(context.Context, *specState) error
	Cleanup(context.Context, *specState)
}

type specNoop struct{}

func (s *specNoop) Setup(context.Context, *specState) error { return nil }
func (s *specNoop) Cleanup(context.Context, *specState)     {}

type specNode struct {
	ctx context.Context
	specAction
	state     *specState
	id        string
	dependsOn []string
}

var _ = godag.Node[string, *specNode](&specNode{})

func (s *specNode) DoDAGTask() ([]*specNode, error) {
	// defer GinkgoRecover()
	defer ByStartStop(fmt.Sprintf("Gingk8s Node: %s", s.id))()
	err := s.Setup(s.ctx, s.state)
	if err != nil {
		return nil, err
	}
	cleanLock.Lock()
	defer cleanLock.Unlock()
	s.state.cleanup = append(s.state.cleanup, s)
	// DeferCleanup(func(ctx context.Context) { s.Cleanup(ctx, s.state) })
	return nil, nil
}

func (s *specNode) GetID() string {
	return s.id
}

func (s *specNode) GetDependencies() godag.Set[string] {
	return godag.SetFrom[string](s.dependsOn)
}

type cleanupSpecNode struct {
	*specNode
	ctx context.Context
}

func (s cleanupSpecNode) DoDAGTask() ([]cleanupSpecNode, error) {
	s.specNode.Cleanup(s.ctx, s.state)
	return nil, nil
}

type specState struct {
	thirdPartyImages       map[string]*ThirdPartyImage
	thirdPartyImageFormats map[string]ImageFormat
	clusterThirdPartyLoads map[string]map[string]string

	customImages       map[string]*CustomImage
	customImageFormats map[string]ImageFormat
	clusterCustomLoads map[string]map[string]string

	clusters map[string]Cluster

	manifests      map[string]*KubernetesManifests
	releases       map[string]*HelmRelease
	clusterActions map[string]ClusterAction

	parent *specState
	suite  *suiteState

	setup []*specNode

	cleanup []*specNode
}

func (s *specState) getCluster(id string) Cluster {
	c, ok := s.clusters[id]
	if !ok && s.parent != nil {
		return s.parent.getCluster(id)
	}
	return c
}

func (s *specState) child() specState {
	return newSpecState(s.suite, s)
}

func newSpecState(suite *suiteState, parent *specState) specState {
	return specState{
		thirdPartyImages:       make(map[string]*ThirdPartyImage),
		thirdPartyImageFormats: make(map[string]ImageFormat),
		customImages:           make(map[string]*CustomImage),
		customImageFormats:     make(map[string]ImageFormat),

		clusterThirdPartyLoads: make(map[string]map[string]string),
		clusterCustomLoads:     make(map[string]map[string]string),

		clusters: make(map[string]Cluster),

		manifests:      make(map[string]*KubernetesManifests),
		releases:       make(map[string]*HelmRelease),
		clusterActions: make(map[string]ClusterAction),

		suite: suite,

		parent: parent,
	}
}

type serializableSpec struct {
	Clusters map[string]*DummyCluster
}

func (s *specState) serialize() serializableSpec {
	serializable := serializableSpec{
		Clusters: make(map[string]*DummyCluster, len(s.clusters)),
	}
	for k, v := range s.clusters {
		serializable.Clusters[k] = &DummyCluster{Connection: *v.GetConnection()}
	}
	return serializable
}

func (s *specState) deserialize(parent *specState, serializable serializableSpec) {
	s.clusters = make(map[string]Cluster, len(serializable.Clusters))
	s.parent = parent
	if parent != nil {
		s.suite = s.parent.suite
	}
	for k, v := range serializable.Clusters {
		s.clusters[k] = v
	}
}
