package gingk8s

import (
	"context"
	"fmt"
	"sync"

	"github.com/meln5674/godag"

	. "github.com/onsi/ginkgo/v2"
)

var cleanLock = sync.Mutex{}

type specAction interface {
	Setup(context.Context, *specState) error
	Cleanup(context.Context, *specState)
	Title(*specState) string
}

type specNoop struct{}

func (s *specNoop) Setup(context.Context, *specState) error { return nil }
func (s *specNoop) Cleanup(context.Context, *specState)     {}
func (s *specNoop) Title(*specState) string {
	return "No-op"
}

type specNode struct {
	ctx context.Context
	specAction
	state     *specState
	id        string
	dependsOn []string
}

var _ = godag.Node[string, *specNode](&specNode{})

func (s *specNode) DoDAGTask() ([]*specNode, error) {
	defer GinkgoRecover()
	if _, ok := s.specAction.(*specNoop); !ok {
		defer ByStartStop(fmt.Sprintf("Gingk8s Node: %s (%s)", s.Title(s.state), s.id))()
	}
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

func (s *specNode) GetDescription() string {
	return s.specAction.Title(s.state)
}

func (s *specNode) GetDependencies() godag.Set[string] {
	return godag.SetFrom[string](s.dependsOn)
}

type cleanupSpecNode struct {
	*specNode
	ctx context.Context
}

func (s cleanupSpecNode) DoDAGTask() ([]cleanupSpecNode, error) {
	defer GinkgoRecover()
	if _, ok := s.specAction.(*specNoop); !ok {
		defer ByStartStop(fmt.Sprintf("Gingk8s Node (Undo): %s (%s)", s.Title(s.state), s.id))()
	}
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

	imageArchives            map[string]*ImageArchive
	clusterImageArchiveLoads map[string]map[string]string

	clusters map[string]Cluster

	manifests      map[string]*KubernetesManifests
	releases       map[string]*HelmRelease
	clusterActions map[string]ClusterAction

	parent *specState
	suite  *suiteState

	setup []*specNode

	cleanup []*specNode
}

func (s *specState) NoCleanup() bool {
	// If we have no parent, we must be the suite spec
	if s.parent == nil {
		return s.suite.opts.NoSuiteCleanup
	} else {
		return s.suite.opts.NoSpecCleanup
	}
}

func (s *specState) getCluster(id string) Cluster {
	c, ok := s.clusters[id]
	if !ok && s.parent != nil {
		return s.parent.getCluster(id)
	}
	return c
}

func (s *specState) child() *specState {
	s2 := newSpecState(s.suite, s)
	for id, cluster := range s.clusters {
		cluster2 := noopCluster{Cluster: cluster}
		s2.clusters[id] = cluster2
		s2.setup = append(s2.setup, &specNode{state: &s2, id: id, specAction: &createClusterAction{id: id}})
	}
	return &s2
}

func newSpecState(suite *suiteState, parent *specState) specState {
	return specState{
		thirdPartyImages:       make(map[string]*ThirdPartyImage),
		thirdPartyImageFormats: make(map[string]ImageFormat),

		customImages:       make(map[string]*CustomImage),
		customImageFormats: make(map[string]ImageFormat),

		imageArchives: make(map[string]*ImageArchive),

		clusterThirdPartyLoads:   make(map[string]map[string]string),
		clusterCustomLoads:       make(map[string]map[string]string),
		clusterImageArchiveLoads: make(map[string]map[string]string),

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
		serializable.Clusters[k] = &DummyCluster{Connection: *v.GetConnection(), TempDir: v.GetTempDir(), Name: v.GetName()}
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
