package gingk8s

import types "github.com/meln5674/gingk8s/pkg/gingk8s"

type suiteState struct {
	specState

	opts types.SuiteOpts

	manifests      map[string]*types.KubernetesManifests
	releases       map[string]*types.HelmRelease
	clusterActions map[string]types.ClusterAction

	setup []*specNode
}

var (
	state = suiteState{

		manifests:      make(map[string]*types.KubernetesManifests),
		releases:       make(map[string]*types.HelmRelease),
		clusterActions: make(map[string]types.ClusterAction),

		setup: make([]*specNode, 0),
	}
)

func init() {
	state.specState = newSpecState(&state, nil)
}

func Gingk8sOptions(opts types.SuiteOpts) {
	state.opts = opts
}
