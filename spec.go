package gingk8s

import (
	"context"
	"fmt"

	"github.com/meln5674/godag"
	. "github.com/onsi/ginkgo/v2"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type specAction interface {
	Setup(context.Context) error
	Cleanup(context.Context)
}

type specNode struct {
	ctx context.Context
	specAction
	id        string
	dependsOn []string
}

var _ = godag.Node[string](&specNode{})

func (s *specNode) DoDAGTask() error {
	// defer GinkgoRecover()
	defer ByStartStop(fmt.Sprintf("Gingk8s Node: %s", s.id))()
	err := s.Setup(s.ctx)
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
	thirdPartyImages map[string]*types.ThirdPartyImage

	thirdPartyImageFormats map[string]types.ImageFormat
	customImages           map[string]*types.CustomImage
	customImageFormats     map[string]types.ImageFormat

	clusterThirdPartyLoads map[string]map[string]string
	clusterCustomLoads     map[string]map[string]string

	clusters map[string]types.Cluster

	parent *specState
	suite  *suiteState

	setup []*specNode
}

func (s *specState) child() specState {
	return newSpecState(s.suite, s)
}

func newSpecState(suite *suiteState, parent *specState) specState {
	return specState{
		thirdPartyImages:       make(map[string]*types.ThirdPartyImage),
		thirdPartyImageFormats: make(map[string]types.ImageFormat),
		customImages:           make(map[string]*types.CustomImage),
		customImageFormats:     make(map[string]types.ImageFormat),

		clusterThirdPartyLoads: make(map[string]map[string]string),
		clusterCustomLoads:     make(map[string]map[string]string),

		clusters: make(map[string]types.Cluster),

		suite: suite,

		parent: parent,
	}
}
