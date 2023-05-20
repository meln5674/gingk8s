package gingk8s

import (
	"context"

	"github.com/meln5674/gosh"
)

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
	LoadImages(ctx context.Context, from Images, format ImageFormat, images []string) gosh.Commander
	// Delete deletes the cluster. Delete should not fail if the cluster exists.
	Delete(ctx context.Context) gosh.Commander
}
