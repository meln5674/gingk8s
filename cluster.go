package gingk8s

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

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
	g.clusterImageArchiveLoads[clusterID] = make(map[string]string)
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
				noCache:   g.suite.opts.NoCacheImages && !g.thirdPartyImages[image.id].NoPull,
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
	for _, archive := range allDeps.ImageArchives {
		loadID := newID()
		g.clusterImageArchiveLoads[clusterID][archive.id] = loadID
		g.setup = append(g.setup, &specNode{
			state:     g.specState,
			id:        loadID,
			dependsOn: []string{clusterID, archive.id},
			specAction: &loadImageArchiveAction{
				id:        loadID,
				archiveID: archive.id,
				clusterID: clusterID,
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
	return state.clusters[c.id].Create(ctx, true).Run()
}

func (c *createClusterAction) Cleanup(ctx context.Context, state *specState) {
	if state.NoCleanup() {
		return
	}
	Expect(state.clusters[c.id].Delete(ctx).Run()).To(Succeed())
}

func (c *createClusterAction) Title(state *specState) string {
	return fmt.Sprintf("Create cluster %s", state.clusters[c.id].GetName())
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
	// GetTempDir returns a directory to use for temporary files related to this cluster.
	// It must return the same value every time its called.
	GetTempDir() string
	// LoadImages loads a set of images of a given format from an image source.
	// If noCache is set, LoadImages must remove any copies of the image outside of the cluster.
	LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander
	// LoadImageArchives loads a set of image archives of a given format from the filesystem.
	LoadImageArchives(ctx context.Context, format ImageFormat, archives []string) gosh.Commander
	// Delete deletes the cluster. Delete should not fail if the cluster exists.
	Delete(ctx context.Context) gosh.Commander
	// GetName returns a descriptive name for this cluster
	GetName() string
}

// returns the path to a file or directory to use for temporary operations against this cluster
// group is an arbitrary string that will be used as a subdirectory, it is split for convenience to be used as a constant
func ClusterTempPath(cluster Cluster, group string, path ...string) string {
	return filepath.Join(append([]string{cluster.GetTempDir(), group}, path...)...)
}

// DummyCluster implements Cluster but only the methods Create() (which does nothing)
// and GetConnection() (which returns a canned connection struct), any other calls panic
type DummyCluster struct {
	Connection KubernetesConnection
	TempDir    string
	Name       string
}

func (d *DummyCluster) Create(ctx context.Context, skipExisting bool) gosh.Commander {
	panic("UNSUPPORTED")
}
func (d *DummyCluster) GetConnection() *KubernetesConnection {
	return &d.Connection
}
func (d *DummyCluster) GetTempDir() string {
	return d.TempDir
}
func (d *DummyCluster) GetName() string {
	return d.Name
}
func (d *DummyCluster) LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander {
	panic("UNSUPPORTED")
}
func (d *DummyCluster) LoadImageArchives(ctx context.Context, format ImageFormat, archives []string) gosh.Commander {
	panic("UNSUPPORTED")
}
func (d *DummyCluster) Delete(ctx context.Context) gosh.Commander {
	panic("UNSUPPORTED")
}

type noopCluster struct {
	Cluster
}

func noopCommander(ctx context.Context) gosh.Commander {
	return gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		close(done)
		return nil
	})
}

func (n noopCluster) Create(ctx context.Context, skipExisting bool) gosh.Commander {
	return noopCommander(ctx)
}
func (n noopCluster) GetConnection() *KubernetesConnection {
	return n.Cluster.GetConnection()
}
func (n noopCluster) GetTempDir() string {
	return n.Cluster.GetTempDir()
}
func (n noopCluster) GetName() string {
	return n.Cluster.GetName()
}
func (n noopCluster) LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander {
	return n.Cluster.LoadImages(ctx, from, format, images, noCache)
}
func (n noopCluster) LoadImageArchives(ctx context.Context, format ImageFormat, archives []string) gosh.Commander {
	return n.Cluster.LoadImageArchives(ctx, format, archives)
}
func (n noopCluster) Delete(ctx context.Context) gosh.Commander {
	return noopCommander(ctx)
}
