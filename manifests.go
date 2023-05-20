package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type ManifestsID struct {
	id string
}

func Manifests(cluster ClusterID, manifests *types.KubernetesManifests, deps ...ResourceDependencies) ManifestsID {
	manifestID := newID()
	state.manifests[manifestID] = manifests

	node := specNode{id: manifestID, dependsOn: []string{cluster.id}, specAction: &manifestsAction{id: manifestID, clusterID: cluster.id}}
	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	state.setup = append(state.setup, &node)

	return ManifestsID{id: manifestID}
}

type manifestsAction struct {
	id        string
	clusterID string
}

func (m *manifestsAction) Setup(ctx context.Context) error {
	if state.opts.NoDeps {
		By(fmt.Sprintf("SKIPPED: Creating manifest set %s", state.manifests[m.id].Name))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Creating manifest set %s", state.manifests[m.id].Name))()
	return state.opts.Manifests.CreateOrUpdate(ctx, state.clusters[m.clusterID], state.manifests[m.id]).Run()
}

func (m *manifestsAction) Cleanup(ctx context.Context) {
	if state.opts.NoSuiteCleanup {
		return
	}
	defer ByStartStop("Deleteing a set of manifests")()
	Expect(state.opts.Manifests.Delete(ctx, state.clusters[m.clusterID], state.manifests[m.id]).Run()).To(Succeed())
}
