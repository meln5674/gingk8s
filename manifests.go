package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meln5674/gosh"
)

type ManifestsID struct {
	id string
}

func (m ManifestsID) AddResourceDependency(dep *ResourceDependencies) {
	dep.Manifests = append(dep.Manifests, m)
}

func (g Gingk8s) Manifests(cluster ClusterID, manifests *KubernetesManifests, deps ...ResourceDependency) ManifestsID {
	manifestID := newID()
	g.manifests[manifestID] = manifests

	dependsOn := append([]string{cluster.id}, forResourceDependencies(deps...).allIDs(g.specState, cluster.id)...)
	node := specNode{
		state:      g.specState,
		id:         manifestID,
		dependsOn:  dependsOn,
		specAction: &manifestsAction{id: manifestID, clusterID: cluster.id, g: g},
	}

	g.setup = append(g.setup, &node)

	return ManifestsID{id: manifestID}
}

type manifestsAction struct {
	id        string
	clusterID string
	g         Gingk8s
}

func (m *manifestsAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoDeps {
		By(fmt.Sprintf("SKIPPED: Creating Manifests %s", state.manifests[m.id].Name))
		return nil
	}
	return state.suite.opts.Manifests.CreateOrUpdate(m.g, ctx, state.getCluster(m.clusterID), state.manifests[m.id]).Run()
}

func (m *manifestsAction) Cleanup(ctx context.Context, state *specState) {
	if state.NoCleanup() || state.manifests[m.id].SkipDelete {
		return
	}
	Expect(state.suite.opts.Manifests.Delete(m.g, ctx, state.getCluster(m.clusterID), state.manifests[m.id]).Run()).To(Succeed())
}

func (m *manifestsAction) Title(state *specState) string {
	return fmt.Sprintf("Submit manifest set %s to cluster %s", state.manifests[m.id].Name, state.clusters[m.clusterID].GetName())
}

var (
	// DefaultManifests is the default interface used to manage manifests if none is specified.
	// It defaults to using the "kubectl" command on the $PATH
	DefaultManifests = &KubectlCommand{}
)

// KubernetesResources is a set of kubernetes manifests from literal strings, files, and directories.
// Ordering of resources is not guaranteed, and may be performed concurrently
type KubernetesManifests struct {
	// Name is a human-readable name to give the manifest set in logs
	Name string
	// Namespace is the namespace to use for manifests. If set, all manifests must either have no namespace set, or their namespace must match.
	Namespace string
	// ResourceObjects are objects to convert to YAML and treat as manifests.
	// If using Object or NestedObject, functions are handled appropriately.
	ResourceObjects []interface{}
	// Resources are strings containing the manifests
	Resources []string
	// ResourcePaths are paths to individual resource files and (non-recursive) directories
	ResourcePaths []string
	// ResourceRecursiveDirs are paths to directories recursively containing resource files
	ResourceRecursiveDirs []string
	// Replace indicates these resources should be replaced, not applied
	Replace bool
	// Create indicates these resources should be created, not applied
	Create bool
	Wait   []WaitFor
	// SkipDelete indicates not to remove this resource on cleanup.
	SkipDelete bool
	// SkipDeleteWait indicates that after deleting these resources, do not wait for them to be fully removed
	SkipDeleteWait bool

	// Created is the list of resources that were created/applied, in order.
	// If nil, it is ignored.
	// If non-nil, it must be a pre-populated with the same length as the number of resources.
	// yaml.Unmarshal is used
	Created []interface{}
}

// Manifests knows how to manage raw kubernetes manifests
type Manifests interface {
	// CreateOrUpdate creates or updates a set of manifests
	CreateOrUpdate(g Gingk8s, ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander
	// Delete removes a set of manifests
	Delete(g Gingk8s, ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander
}
