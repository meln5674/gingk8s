package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/meln5674/gosh"
)

type ClusterID struct {
	id string
}

func (g Gingk8s) Cluster(cluster Cluster, deps ...ClusterDependency) ClusterID {
	clusterID := newID()
	g.clusters[clusterID] = cluster
	g.clusterThirdPartyLoads[clusterID] = make(map[string]string)
	g.clusterCustomLoads[clusterID] = make(map[string]string)
	clusterNode := specNode{state: g.specState, id: clusterID, specAction: &createClusterAction{id: clusterID}}
	allDeps := *forClusterDependencies(deps...)
	for _, image := range allDeps.ThirdPartyImages {
		loadID := newID()
		g.clusterThirdPartyLoads[clusterID][image.id] = loadID
		g.setup = append(g.setup, &specNode{
			state:     g.specState,
			id:        loadID,
			dependsOn: []string{clusterID, image.id},
			specAction: &loadThirdPartyImageAction{
				id:        loadID,
				imageID:   image.id,
				clusterID: clusterID,
				noCache:   g.suite.opts.NoCacheImages,
			},
		})
	}
	for _, image := range allDeps.CustomImages {
		loadID := newID()
		g.clusterCustomLoads[clusterID][image.id] = loadID
		g.setup = append(g.setup, &specNode{
			state:     g.specState,
			id:        loadID,
			dependsOn: []string{clusterID, image.id},
			specAction: &loadCustomImageAction{
				id:        loadID,
				imageID:   image.id,
				clusterID: clusterID,
				noCache:   g.suite.opts.NoCacheImages,
			},
		})
	}
	g.setup = append(g.setup, &clusterNode)

	return ClusterID{id: clusterID}
}

type createClusterAction struct {
	id string
}

func (c *createClusterAction) Setup(ctx context.Context, state *specState) error {
	defer ByStartStop("Creating cluster")()
	return state.clusters[c.id].Create(ctx, true).Run()
}

func (c *createClusterAction) Cleanup(ctx context.Context, state *specState) {
	if state.suite.opts.NoSuiteCleanup {
		return
	}
	defer ByStartStop(fmt.Sprintf("Deleting cluster"))()
	Expect(state.clusters[c.id].Delete(ctx).Run()).To(Succeed())
}

// KubernetesConnection defines a connection to a kubernetes API server
type KubernetesConnection struct {
	// Kubeconfig is the path to a kubeconfig file. If blank, the default loading rules are used.
	Kubeconfig string
	// Context is the name of the context within the kubeconfig file to use. If blank, the current context is used.
	Context string
}

// Cluster knows how to manage a temporary cluster
type Cluster interface {
	// Create creates a cluster. If skipExisting is true, it will not fail if the cluster already exists
	Create(ctx context.Context, skipExisting bool) gosh.Commander
	// GetConnection returns the kubeconfig, context, etc, to use to connect to the cluster's api server.
	// GetConnection must not fail, so any potentially failing operations must be done during Create()
	GetConnection() *KubernetesConnection
	// GetTempPath returns the path to a file or directory to use for temporary operations against this cluster
	GetTempPath(group string, path string) string
	// LoadImages loads a set of images of a given format from.
	// If noCache is set, LoadImages must remove any copies of the image outside of the cluster.
	LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander
	// Delete deletes the cluster. Delete should not fail if the cluster exists.
	Delete(ctx context.Context) gosh.Commander
}

// DummyCluster implements Cluster but only the methods Create() (which does nothing)
// and GetConnection() (which returns a canned connection struct), any other calls panic
type DummyCluster struct {
	Connection KubernetesConnection
}

func (d *DummyCluster) Create(ctx context.Context, skipExisting bool) gosh.Commander {
	panic("UNSUPPORTED")
}
func (d *DummyCluster) GetConnection() *KubernetesConnection {
	return &d.Connection
}
func (d *DummyCluster) GetTempPath(group string, path string) string {
	panic("UNSUPPORTED")
}
func (d *DummyCluster) LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander {
	panic("UNSUPPORTED")
}
func (d *DummyCluster) Delete(ctx context.Context) gosh.Commander {
	panic("UNSUPPORTED")
}
