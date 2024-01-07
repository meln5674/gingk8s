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

func (r ReleaseID) AddResourceDependency(dep *ResourceDependencies) {
	dep.Releases = append(dep.Releases, r)
}

func (g Gingk8s) Release(cluster ClusterID, release *HelmRelease, deps ...ResourceDependency) ReleaseID {
	releaseID := newID()
	g.releases[releaseID] = release

	dependsOn := append([]string{cluster.id}, forResourceDependencies(deps...).allIDs(g.specState, cluster.id)...)
	node := specNode{
		state:      g.specState,
		id:         releaseID,
		dependsOn:  dependsOn,
		specAction: &releaseAction{id: releaseID, clusterID: cluster.id, g: g},
	}

	g.setup = append(g.setup, &node)

	return ReleaseID{id: releaseID}
}

type releaseAction struct {
	id        string
	clusterID string
	g         Gingk8s
}

func (r *releaseAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoDeps {
		return nil
	}
	return state.suite.opts.Helm.InstallOrUpgrade(r.g, ctx, state.getCluster(r.clusterID), state.releases[r.id]).Run()
}

func (r *releaseAction) Cleanup(ctx context.Context, state *specState) {
	if state.NoCleanup() || state.releases[r.id].SkipDelete {
		return
	}
	Expect(state.suite.opts.Helm.Delete(ctx, state.getCluster(r.clusterID), state.releases[r.id], true).Run()).To(Succeed())
}

func (r *releaseAction) Title(state *specState) string {
	return fmt.Sprintf("Creating helm release %s", state.releases[r.id].Name)
}

// HelmRepo represents a repository to pull helm charts from
type HelmRepo struct {
	// Name is the name of the repository to use in `helm repo add`
	Name string
	// URL is the URL to provide in `helm repo add`
	URL string
	// Flags are any extra flags to provide to the `helm repo add` command
	Flags []string
	// Update indicates that `helm repo update` should be run for this repo
	Update bool
}

type HelmRegistry struct {
	Hostname   string
	LoginFlags []string
}

// LocalChartInfo is a reference to a helm chart from a local directory or tarball
type LocalChartInfo struct {
	// Path is the path to the local directory or tarball
	Path             string
	DependencyUpdate bool
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

// OCIChartInfo is a reference to a helm chart from a local directory or tarball
type OCIChartInfo struct {
	// Registry is the image registry of the chart
	Registry HelmRegistry
	// Repository is the repository of the chart within the registry
	Repository string
	// Version is the version of the chart, or equivalently, the tag of the OCI artifact in the repository
	Version string
}

// HelmChart represents a chart to be installed
type HelmChart struct {
	// LocalChartInfo is the location of the local chart. Mutually exclusive with RemoteChartInfo and OCIChartInfo
	LocalChartInfo
	// RemoteChartInfo is the location of the remote chart. Mutually exclusive with LocalChartInfo and OCIChartInfo
	RemoteChartInfo
	// OCIChartInfo is the location of a chart in an OCI registry. Mutually exclusive with LocalChartInfo and RemoteChartInfo
	OCIChartInfo

	// UpgradeFlags are extra flags to pass to the `helm upgrade` command
	UpgradeFlags []string
}

func (h *HelmChart) IsLocal() bool {
	return h.Path != ""
}

func (h *HelmChart) IsOCI() bool {
	return h.Registry.Hostname != ""
}

func (h *HelmChart) Fullname() string {
	if h.IsLocal() {
		if !strings.HasPrefix(h.Path, "/") && !strings.HasPrefix(h.Path, "./") && !strings.HasPrefix(h.Path, "../") {
			return "./" + h.Path
		} else {
			return h.Path
		}
	}
	if h.IsOCI() {
		return "oci://" + h.Registry.Hostname + "/" + h.Repository
	}
	return h.Repo.Name + "/" + h.Name
}

func (h *HelmChart) Version() string {
	if h.IsOCI() {
		return h.OCIChartInfo.Version
	} else if h.IsLocal() {
		return ""
	} else {
		return h.RemoteChartInfo.Version
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
	// Wait is a set of resouces to wait for once the release has completed before considering the release to be complete
	Wait []WaitFor
	// NoWait, if true, prevents gingk8s from waiting for the helm release to become healthy (e.g. --wait).
	// This is useful is a release is expected to be in a failing state until an event in a child spec takes place
	NoWait bool

	SkipDelete bool
}

// Helm knows how to install and uninstall helm charts
type Helm interface {
	// AddRepo adds a repo that only this HelmReleaser can use
	AddRepo(ctx context.Context, repo *HelmRepo) gosh.Commander
	// InstallOrUpgrade upgrades or installs a release into a cluster
	InstallOrUpgrade(g Gingk8s, ctx context.Context, cluster Cluster, release *HelmRelease) gosh.Commander
	// Delete removes a release from a cluster. If skipNotExists is true, this should not fail if the release does not exist.
	Delete(ctx context.Context, cluster Cluster, release *HelmRelease, skipNotExists bool) gosh.Commander
}
