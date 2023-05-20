package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type ReleaseID struct {
	id string
}

func Release(cluster ClusterID, release *types.HelmRelease, deps ...ResourceDependencies) ReleaseID {
	releaseID := newID()
	state.releases[releaseID] = release

	node := specNode{id: releaseID, dependsOn: []string{cluster.id}, specAction: &releaseAction{id: releaseID, clusterID: cluster.id}}

	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	state.setup = append(state.setup, &node)

	return ReleaseID{id: releaseID}
}

type releaseAction struct {
	id        string
	clusterID string
}

func (r *releaseAction) Setup(ctx context.Context) error {
	if state.opts.NoDeps {
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Creating helm release %s", state.releases[r.id].Name))()
	return state.opts.Helm.InstallOrUpgrade(ctx, state.clusters[r.clusterID], state.releases[r.id]).Run()
}

func (r *releaseAction) Cleanup(ctx context.Context) {
	if state.opts.NoSuiteCleanup {
		return
	}

	defer ByStartStop(fmt.Sprintf("Deleting helm release %s", state.releases[r.id].Name))()
	Expect(state.opts.Helm.Delete(ctx, state.clusters[r.clusterID], state.releases[r.id], true).Run()).To(Succeed())
}
