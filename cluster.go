package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type ClusterID struct {
	id string
}

type ClusterDependencies struct {
	ThirdPartyImages []ThirdPartyImageID
	CustomImages     []CustomImageID
}

func Cluster(cluster types.Cluster, deps ...ClusterDependencies) ClusterID {
	clusterID := newID()
	state.clusters[clusterID] = cluster
	state.clusterThirdPartyLoads[clusterID] = make(map[string]string)
	state.clusterCustomLoads[clusterID] = make(map[string]string)
	clusterNode := specNode{id: clusterID, specAction: &createClusterAction{id: clusterID}}
	for _, deps := range deps {
		for _, image := range deps.ThirdPartyImages {
			loadID := newID()
			state.clusterThirdPartyLoads[clusterID][image.id] = loadID
			state.setup = append(state.setup, &specNode{id: loadID, dependsOn: []string{clusterID, image.id}, specAction: &loadThirdPartyImageAction{id: loadID, imageID: image.id, clusterID: clusterID}})
		}
		for _, image := range deps.CustomImages {
			loadID := newID()
			state.clusterCustomLoads[clusterID][image.id] = loadID
			state.setup = append(state.setup, &specNode{id: loadID, dependsOn: []string{clusterID, image.id}, specAction: &loadCustomImageAction{id: loadID, imageID: image.id, clusterID: clusterID}})
		}
	}
	state.setup = append(state.setup, &clusterNode)

	return ClusterID{id: clusterID}
}

type createClusterAction struct {
	id string
}

func (c *createClusterAction) Setup(ctx context.Context) error {
	defer ByStartStop("Creating cluster")()
	return state.clusters[c.id].Create(ctx, true).Run()
}

func (c *createClusterAction) Cleanup(ctx context.Context) {
	if state.opts.NoSuiteCleanup {
		return
	}
	defer ByStartStop(fmt.Sprintf("Deleting cluster"))()
	Expect(state.clusters[c.id].Delete(ctx).Run()).To(Succeed())
}
