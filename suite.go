package gingk8s

import "github.com/onsi/ginkgo/v2"

// SuiteOpts controls the behavior of the suite
type SuiteOpts struct {
	// NoSuiteCleanup disables deleting the cluster after the suite has finishes
	NoSuiteCleanup bool
	// NoSpecCleanup disables deleting third party resources after specs have completed
	NoSpecCleanup bool
	// NoPull disables pulling third party images
	NoPull bool
	// NoLoadPulled disables loading pulled third party images
	NoLoadPulled bool
	// NoBuild disables building custom images
	NoBuild bool
	// NoLoadBuilt disables loading built custom images
	NoLoadBuilt bool
	// NoDeps disables re-installing third party resources
	NoDeps bool

	// CustomImageTag is the tag to set for all custom images
	CustomImageTag string
	// ExtraCustomImageTags are a set of extra tags to set for all custom images
	ExtraCustomImageTags []string

	// Images is how to pull, build, and save images
	Images Images
	// Manifests is how to deploy and delete kubernetes manifests
	Manifests Manifests
	// Helm is how to deploy and delete helm charts
	Helm Helm
	// Kubectl is how to execute kubectl
	Kubectl Kubectl

	// KLogFlags are a set of command line flags to configure the klog library with
	KLogFlags []string
}

type suiteState struct {
	specState

	opts SuiteOpts

	ginkgo ginkgo.FullGinkgoTInterface

	setup []*specNode
}

var (
	state = suiteState{

		setup: make([]*specNode, 0),
	}
)

func init() {
	state.specState = newSpecState(&state, nil)
	state.suite = &state
}
