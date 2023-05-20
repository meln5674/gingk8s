package gingk8s

import (
	"context"

	"github.com/meln5674/gosh"
)

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
