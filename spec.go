package gingk8s

import (
	"context"
	"fmt"

	"github.com/meln5674/godag"
	. "github.com/onsi/ginkgo/v2"
)

type specAction interface {
	Setup(context.Context, *specState) error
	Cleanup(context.Context, *specState)
}

type specNode struct {
	ctx context.Context
	specAction
	state     *specState
	id        string
	dependsOn []string
}

var _ = godag.Node[string](&specNode{})

func (s *specNode) DoDAGTask() error {
	// defer GinkgoRecover()
	defer ByStartStop(fmt.Sprintf("Gingk8s Node: %s", s.id))()
	err := s.Setup(s.ctx, s.state)
	if err != nil {
		return err
	}
	DeferCleanup(s.Cleanup)
	return nil
}

func (s *specNode) GetID() string {
	return s.id
}

func (s *specNode) GetDependencies() godag.Set[string] {
	return godag.SetFrom[string](s.dependsOn)
}

type specState struct {
	thirdPartyImages map[string]*ThirdPartyImage

	thirdPartyImageFormats map[string]ImageFormat
	customImages           map[string]*CustomImage
	customImageFormats     map[string]ImageFormat

	clusterThirdPartyLoads map[string]map[string]string
	clusterCustomLoads     map[string]map[string]string

	clusters map[string]Cluster

	manifests      map[string]*KubernetesManifests
	releases       map[string]*HelmRelease
	clusterActions map[string]ClusterAction

	parent *specState
	suite  *suiteState

	setup []*specNode
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
