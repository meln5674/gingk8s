package gingk8s

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/gomega"

	"github.com/meln5674/gosh"
)

var (
	// DefaultHelm is the default interface used to manage helm repos, charts, and releasess if none is specified
	// It defaults to using the "helm" command on the path.
	DefaultHelm = &HelmCommand{}
)

type ReleaseID struct {
	id string
}

func (g Gingk8s) Release(cluster ClusterID, release *HelmRelease, deps ...ResourceDependencies) ReleaseID {
	releaseID := newID()
	g.releases[releaseID] = release

	node := specNode{state: g.specState, id: releaseID, dependsOn: []string{cluster.id}, specAction: &releaseAction{id: releaseID, clusterID: cluster.id}}

	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	g.setup = append(g.setup, &node)

	return ReleaseID{id: releaseID}
}

type releaseAction struct {
	id        string
	clusterID string
}

func (r *releaseAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoDeps {
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Creating helm release %s", state.releases[r.id].Name))()
	return state.suite.opts.Helm.InstallOrUpgrade(ctx, state.clusters[r.clusterID], state.releases[r.id]).Run()
}

func (r *releaseAction) Cleanup(ctx context.Context, state *specState) {
	if state.suite.opts.NoSuiteCleanup {
		return
	}

	defer ByStartStop(fmt.Sprintf("Deleting helm release %s", state.releases[r.id].Name))()
	Expect(state.suite.opts.Helm.Delete(ctx, state.clusters[r.clusterID], state.releases[r.id], true).Run()).To(Succeed())
}

// HelmRepo represents a repository to pull helm charts from
type HelmRepo struct {
	// Name is the name of the repository to use in `helm repo add`
	Name string
	// URL is the URL to provide in `helm repo add`
	URL string
	// Flags are any extra flags to provide to the `helm repo add` command
	Flags []string
}

// LocalChartInfo is a reference to a helm chart from a local directory or tarball
type LocalChartInfo struct {
	// Path is the path to the local directory or tarball
	Path string
}

// RemoteChartInfo is a reference to a helm chart from a remote repository
type RemoteChartInfo struct {
	// Name is the name of the chart to be installed from a remote repo
	Name string
	// Repo is a reference to the HelmRepo object representing the respository to pull the chart from
	Repo *HelmRepo
	// Version is the version of the chart to install. Will use latest if omitted.
	Version string
}

// HelmChart represents a chart to be installed
type HelmChart struct {
	// LocalChartInfo is the location of the local chart. Mutually exclusive with RemoteChartInfo
	LocalChartInfo
	// RemoteChartInfo is the location of the remote chart. Mutually exclusive with LocalChartInfo
	RemoteChartInfo

	// UpgradeFlags are extra flags to pass to the `helm upgrade` command
	UpgradeFlags []string
}

func (h *HelmChart) IsLocal() bool {
	return h.Path != ""
}

func (h *HelmChart) Fullname() string {
	if h.IsLocal() {
		if !strings.HasPrefix(h.Path, "./") && !strings.HasPrefix(h.Path, "../") {
			return "./" + h.Path
		} else {
			return h.Path
		}
	} else {
		return h.Repo.Name + "/" + h.Name
	}
}

// HelmRelease represents a helm chart to be released into the cluster
type HelmRelease struct {
	// Name is the name of the release
	Name string
	// Namespace is the namespace of the release. Will use whatever the kubeconfig context namespace is omitted (usually "default")
	Namespace string
	// Chart is a reference to the chart to release
	Chart *HelmChart
	// Set is a map of --set arguments
	Set Object
	// SetString is a map of --set-string arguments
	SetString StringObject
	// SetFile is a map of --set-file arguments
	SetFile StringObject
	// ValuesFiles is a list of files to be provided with --values
	ValuesFiles []string
	// Values is a set of objects to be serialized as YAML files and provided with --values
	Values []NestedObject
	// ExtraFlags is a set of extra arguments to pass to both helm upgrade and delete
	ExtraFlags []string
	// UpgradeFlags is a set of extra arguments to pass to helm upgrade
	UpgradeFlags []string
	// DeleteFlags is a set of extra arguments to pass to helm delete
	DeleteFlags []string
	Wait        []WaitFor
}

// Helm knows how to install and uninstall helm charts
type Helm interface {
	// AddRepo adds a repo that only this HelmReleaser can use
	AddRepo(ctx context.Context, repo *HelmRepo) gosh.Commander
	// InstallOrUpgrade upgrades or installs a release into a cluster
	InstallOrUpgrade(ctx context.Context, cluster Cluster, release *HelmRelease) gosh.Commander
	// Delete removes a release from a cluster. If skipNotExists is true, this should not fail if the release does not exist.
	Delete(ctx context.Context, cluster Cluster, release *HelmRelease, skipNotExists bool) gosh.Commander
}
