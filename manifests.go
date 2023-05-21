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

func (g Gingk8s) Manifests(cluster ClusterID, manifests *KubernetesManifests, deps ...ResourceDependencies) ManifestsID {
	manifestID := newID()
	g.manifests[manifestID] = manifests

	node := specNode{state: g.specState, id: manifestID, dependsOn: []string{cluster.id}, specAction: &manifestsAction{id: manifestID, clusterID: cluster.id}}
	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	g.setup = append(g.setup, &node)

	return ManifestsID{id: manifestID}
}

type manifestsAction struct {
	id        string
	clusterID string
}

func (m *manifestsAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoDeps {
		By(fmt.Sprintf("SKIPPED: Creating manifest set %s", state.manifests[m.id].Name))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Creating manifest set %s", state.manifests[m.id].Name))()
	return state.suite.opts.Manifests.CreateOrUpdate(ctx, state.clusters[m.clusterID], state.manifests[m.id]).Run()
}

func (m *manifestsAction) Cleanup(ctx context.Context, state *specState) {
	if state.suite.opts.NoSuiteCleanup {
		return
	}
	defer ByStartStop("Deleteing a set of manifests")()
	Expect(state.suite.opts.Manifests.Delete(ctx, state.clusters[m.clusterID], state.manifests[m.id]).Run()).To(Succeed())
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
	Wait    []WaitFor
}

// Manifests knows how to manage raw kubernetes manifests
type Manifests interface {
	// CreateOrUpdate creates or updates a set of manifests
	CreateOrUpdate(ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander
	// Delete removes a set of manifests
	Delete(ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander
}
