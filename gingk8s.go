package gingk8s

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/google/uuid"
	"github.com/meln5674/godag"
	"github.com/meln5674/gosh"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type suiteAction interface {
	Setup(context.Context) error
	Cleanup(context.Context)
}

type pullThirdPartyImageAction struct {
	id string
}

func (p *pullThirdPartyImageAction) Setup(ctx context.Context) error {
	if state.opts.NoPull {
		By(fmt.Sprintf("SKIPPED: Pulling image %s", state.thirdPartyImages[p.id].Name))
		return nil
	}
	By(fmt.Sprintf("Pulling image %s", state.thirdPartyImages[p.id].Name))
	return state.opts.Images.Pull(ctx, state.thirdPartyImages[p.id]).Run()
}

func (p *pullThirdPartyImageAction) Cleanup(ctx context.Context) {}

type buildCustomImageAction struct {
	id string
}

func ByStartStop(msg string) func() {
	By(fmt.Sprintf("STARTING: %s", msg))
	return func() { By(fmt.Sprintf("FINISHED: %s", msg)) }
}

func (b *buildCustomImageAction) Setup(ctx context.Context) error {
	if state.opts.NoBuild {
		By(fmt.Sprintf("SKIPPED: Building image %s", state.customImages[b.id].WithTag(state.opts.CustomImageTag)))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Building image %s", state.customImages[b.id].WithTag(state.opts.CustomImageTag)))()
	return state.opts.Images.Build(ctx, state.customImages[b.id], state.opts.CustomImageTag, state.opts.ExtraCustomImageTags).Run()
}

func (b *buildCustomImageAction) Cleanup(ctx context.Context) {}

type loadThirdPartyImageAction struct {
	id        string
	clusterID string
	imageID   string
}

func (l *loadThirdPartyImageAction) Setup(ctx context.Context) error {
	if state.opts.NoLoadPulled {
		By(fmt.Sprintf("SKIPPED: Loading image %s", state.thirdPartyImages[l.id].Name))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Loading image %s", state.thirdPartyImages[l.imageID].Name))()
	return state.clusters[l.clusterID].LoadImages(ctx, state.opts.Images, state.thirdPartyImageFormats[l.imageID], []string{state.thirdPartyImages[l.imageID].Name}).Run()
}

func (l *loadThirdPartyImageAction) Cleanup(ctx context.Context) {}

type loadCustomImageAction struct {
	id        string
	clusterID string
	imageID   string
}

func (l *loadCustomImageAction) Setup(ctx context.Context) error {
	if state.opts.NoLoadBuilt {
		By(fmt.Sprintf("SKIPPED: Loading image %s", state.customImages[l.imageID].WithTag(state.opts.CustomImageTag)))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Loading image %s", state.customImages[l.imageID].WithTag(state.opts.CustomImageTag)))()
	allTags := []string{state.customImages[l.imageID].WithTag(state.opts.CustomImageTag)}

	return state.clusters[l.clusterID].LoadImages(ctx, state.opts.Images, state.customImageFormats[l.imageID], allTags).Run()
}

func (l *loadCustomImageAction) Cleanup(ctx context.Context) {}

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

type manifestsAction struct {
	id        string
	clusterID string
}

func (m *manifestsAction) Setup(ctx context.Context) error {
	if state.opts.NoDeps {
		By(fmt.Sprintf("SKIPPED: Creating manifest set %s", state.manifests[m.id].Name))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Creating manifest set %s", state.manifests[m.id].Name))()
	return state.opts.Manifests.CreateOrUpdate(ctx, state.clusters[m.clusterID], state.manifests[m.id]).Run()
}

func (m *manifestsAction) Cleanup(ctx context.Context) {
	if state.opts.NoSuiteCleanup {
		return
	}
	defer ByStartStop("Deleteing a set of manifests")()
	Expect(state.opts.Manifests.Delete(ctx, state.clusters[m.clusterID], state.manifests[m.id]).Run()).To(Succeed())
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

type clusterActionAction struct {
	id        string
	name      string
	clusterID string
	cleanup   types.ClusterAction
}

func (c *clusterActionAction) Setup(ctx context.Context) error {
	if state.opts.NoDeps {
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Executing action %s", c.name))()
	return state.clusterActions[c.id](ctx, state.clusters[c.clusterID])
}

func (c *clusterActionAction) Cleanup(ctx context.Context) {
	if state.opts.NoSuiteCleanup {
		return
	}
	if c.cleanup == nil {

	}

	defer ByStartStop(fmt.Sprintf("Undoing action %s", c.name))()
	Expect(c.cleanup(ctx, state.clusters[c.clusterID])).To(Succeed())
}

type suiteNode struct {
	ctx context.Context
	suiteAction
	id        string
	dependsOn []string
}

var _ = godag.Node[string](&suiteNode{})

func (s *suiteNode) DoDAGTask() error {
	// defer GinkgoRecover()
	defer ByStartStop(fmt.Sprintf("Gingk8s Node: %s", s.id))()
	err := s.Setup(s.ctx)
	if err != nil {
		return err
	}
	DeferCleanup(s.Cleanup)
	return nil
}

func (s *suiteNode) GetID() string {
	return s.id
}

func (s *suiteNode) GetDependencies() godag.Set[string] {
	return godag.SetFrom[string](s.dependsOn)
}

type suiteState struct {
	opts types.SuiteOpts

	thirdPartyImages       map[string]*types.ThirdPartyImage
	thirdPartyImageFormats map[string]types.ImageFormat
	customImages           map[string]*types.CustomImage
	customImageFormats     map[string]types.ImageFormat

	clusterThirdPartyLoads map[string]map[string]string
	clusterCustomLoads     map[string]map[string]string

	clusters map[string]types.Cluster

	manifests      map[string]*types.KubernetesManifests
	releases       map[string]*types.HelmRelease
	clusterActions map[string]types.ClusterAction

	setup []*suiteNode
}

var (
	state = suiteState{
		thirdPartyImages:       make(map[string]*types.ThirdPartyImage),
		thirdPartyImageFormats: make(map[string]types.ImageFormat),
		customImages:           make(map[string]*types.CustomImage),
		customImageFormats:     make(map[string]types.ImageFormat),

		clusterThirdPartyLoads: make(map[string]map[string]string),
		clusterCustomLoads:     make(map[string]map[string]string),

		clusters: make(map[string]types.Cluster),

		manifests:      make(map[string]*types.KubernetesManifests),
		releases:       make(map[string]*types.HelmRelease),
		clusterActions: make(map[string]types.ClusterAction),

		setup: make([]*suiteNode, 0),
	}
)

type ClusterID struct {
	id string
}

type ThirdPartyImageID struct {
	id string
}

type CustomImageID struct {
	id string
}

type ManifestsID struct {
	id string
}

type ReleaseID struct {
	id string
}

type ClusterActionID struct {
	id string
}

func Gingk8sOptions(opts types.SuiteOpts) {
	state.opts = opts
}

func newID() string {
	id, err := uuid.NewRandom()
	Expect(err).ToNot(HaveOccurred())
	return id.String()
}

func ThirdPartyImage(image *types.ThirdPartyImage) ThirdPartyImageID {
	return ThirdPartyImages(image)[0]
}

func ThirdPartyImages(images ...*types.ThirdPartyImage) []ThirdPartyImageID {
	newIDs := []ThirdPartyImageID{}
	for ix := range images {
		newID := newID()
		newIDs = append(newIDs, ThirdPartyImageID{id: newID})
		state.thirdPartyImages[newID] = images[ix]
		state.setup = append(state.setup, &suiteNode{id: newID, suiteAction: &pullThirdPartyImageAction{id: newID}})
	}
	return newIDs
}

func CustomImage(image *types.CustomImage) CustomImageID {
	return CustomImages(image)[0]
}

func CustomImages(images ...*types.CustomImage) []CustomImageID {
	newIDs := []CustomImageID{}
	for ix := range images {
		newID := newID()
		newIDs = append(newIDs, CustomImageID{id: newID})
		state.customImages[newID] = images[ix]
		state.setup = append(state.setup, &suiteNode{id: newID, suiteAction: &buildCustomImageAction{id: newID}})
	}
	return newIDs
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
	clusterNode := suiteNode{id: clusterID, suiteAction: &createClusterAction{id: clusterID}}
	for _, deps := range deps {
		for _, image := range deps.ThirdPartyImages {
			loadID := newID()
			state.clusterThirdPartyLoads[clusterID][image.id] = loadID
			state.setup = append(state.setup, &suiteNode{id: loadID, dependsOn: []string{clusterID, image.id}, suiteAction: &loadThirdPartyImageAction{id: loadID, imageID: image.id, clusterID: clusterID}})
		}
		for _, image := range deps.CustomImages {
			loadID := newID()
			state.clusterCustomLoads[clusterID][image.id] = loadID
			state.setup = append(state.setup, &suiteNode{id: loadID, dependsOn: []string{clusterID, image.id}, suiteAction: &loadCustomImageAction{id: loadID, imageID: image.id, clusterID: clusterID}})
		}
	}
	state.setup = append(state.setup, &clusterNode)

	return ClusterID{id: clusterID}
}

type ResourceDependencies struct {
	ThirdPartyImages []ThirdPartyImageID
	CustomImages     []CustomImageID
	Manifests        []ManifestsID
	Releases         []ReleaseID
	ClusterActions   []ClusterActionID
}

var NoDependencies = ResourceDependencies{}

func (r *ResourceDependencies) allIDs(clusterID string) []string {
	dependsOn := []string{}
	for _, image := range r.ThirdPartyImages {
		dependsOn = append(dependsOn, state.clusterThirdPartyLoads[clusterID][image.id])
	}
	for _, image := range r.CustomImages {
		dependsOn = append(dependsOn, state.clusterCustomLoads[clusterID][image.id])
	}
	for _, manifests := range r.Manifests {
		dependsOn = append(dependsOn, manifests.id)
	}
	for _, release := range r.Releases {
		dependsOn = append(dependsOn, release.id)
	}
	for _, action := range r.ClusterActions {
		dependsOn = append(dependsOn, action.id)
	}
	return dependsOn
}

func Manifests(cluster ClusterID, manifests *types.KubernetesManifests, deps ...ResourceDependencies) ManifestsID {
	manifestID := newID()
	state.manifests[manifestID] = manifests

	node := suiteNode{id: manifestID, dependsOn: []string{cluster.id}, suiteAction: &manifestsAction{id: manifestID, clusterID: cluster.id}}
	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	state.setup = append(state.setup, &node)

	return ManifestsID{id: manifestID}
}

func Release(cluster ClusterID, release *types.HelmRelease, deps ...ResourceDependencies) ReleaseID {
	releaseID := newID()
	state.releases[releaseID] = release

	node := suiteNode{id: releaseID, dependsOn: []string{cluster.id}, suiteAction: &releaseAction{id: releaseID, clusterID: cluster.id}}

	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	state.setup = append(state.setup, &node)

	return ReleaseID{id: releaseID}
}

func ClusterAction(cluster ClusterID, name string, f, cleanup types.ClusterAction, deps ...ResourceDependencies) ClusterActionID {
	actionID := newID()
	state.clusterActions[actionID] = f

	node := suiteNode{id: actionID, dependsOn: []string{cluster.id}, suiteAction: &clusterActionAction{id: actionID, clusterID: cluster.id, cleanup: cleanup, name: name}}

	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	state.setup = append(state.setup, &node)

	return ClusterActionID{id: actionID}

}

func Gingk8sSetup(ctx context.Context) {
	klogFlags := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(klogFlags)
	Expect(klogFlags.Parse([]string{"-v=11"})).To(Succeed())

	if state.opts.CustomImageTag == "" {
		state.opts.CustomImageTag = types.DefaultCustomImageTag
	}
	if state.opts.ExtraCustomImageTags == nil {
		state.opts.ExtraCustomImageTags = types.DefaultExtraCustomImageTags()
	}
	if state.opts.Images == nil {
		state.opts.Images = types.DefaultImages
	}
	if state.opts.Manifests == nil {
		state.opts.Manifests = types.DefaultManifests
	}
	if state.opts.Helm == nil {
		state.opts.Helm = types.DefaultHelm
	}
	if state.opts.Kubectl == nil {
		state.opts.Kubectl = types.DefaultKubectl
	}

	repos := make(map[string]*types.HelmRepo, len(state.releases))
	repoReleases := make(map[string]string)

	for _, release := range state.releases {
		if !release.Chart.IsLocal() {
			existing, ok := repos[release.Chart.Name]
			if !ok {
				repos[release.Chart.Repo.Name] = release.Chart.Repo
				repoReleases[release.Chart.Repo.Name] = release.Name
				continue
			}
			Expect(existing).To(Equal(release.Chart.Repo), fmt.Sprintf("Releases %s and %s have incompatible chart repos", release.Name, repoReleases[release.Chart.Repo.Name]))
		}
	}

	repoAdds := make([]gosh.Commander, 0, len(repos))
	for _, repo := range repos {
		repoAdds = append(repoAdds, state.opts.Helm.AddRepo(ctx, repo))
	}
	Expect(gosh.FanOut(repoAdds...).Run()).To(Succeed())

	ex := godag.Executor[string, *suiteNode]{}

	for ix := range state.setup {
		state.setup[ix].ctx = ctx
		GinkgoWriter.Printf("Node: %#v\n", state.setup[ix])
	}

	dag, err := godag.Build[string, *suiteNode](state.setup)
	Expect(err).ToNot(HaveOccurred())

	Expect(ex.Run(dag, godag.Options[string]{})).To(Succeed())
}

func Kubectl(ctx context.Context, cluster types.Cluster, args ...string) *gosh.Cmd {
	return state.opts.Kubectl.Kubectl(ctx, cluster, args).WithStreams(types.GinkgoOutErr)
}

func KubectlGetSecretValue(ctx context.Context, cluster types.Cluster, name, key string, value *string, args ...string) *gosh.Cmd {
	allArgs := []string{
		"get", "secret", name,
		"--template", fmt.Sprintf(`{{ index .data "%s" }}`, key),
	}
	allArgs = append(allArgs, args...)
	return Kubectl(ctx, cluster, allArgs...).
		WithStreams(
			gosh.FuncOut(func(stdout io.Reader) error {
				var err error
				var bytes []byte
				bytes, err = ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, stdout))
				*value = string(bytes)
				return err
			}),
			types.GinkgoErr,
		)
}

func KubectlExec(ctx context.Context, cluster types.Cluster, name, cmd string, cmdArgs []string, args ...string) *gosh.Cmd {
	allArgs := []string{"exec", "-i", name}
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, "--", cmd)
	allArgs = append(allArgs, cmdArgs...)

	return Kubectl(ctx, cluster, allArgs...).WithStreams(types.GinkgoOutErr)
}

type KubectlWatcher struct {
	Kind  string
	Name  string
	Flags []string
	cmd   *gosh.Cmd
}

func (k *KubectlWatcher) Setup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		args := []string{"get", "-w"}
		if k.Name != "" {
			args = append(args, fmt.Sprintf("%s/%s", k.Kind, k.Name))
		} else {
			args = append(args, k.Kind)
		}
		args = append(args, k.Flags...)
		k.cmd = Kubectl(ctx, cluster, args...)
		return k.cmd.Start()
	}
}

func (k *KubectlWatcher) Cleanup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		err := k.cmd.Kill()
		if err != nil {
			return err
		}
		return k.cmd.Wait()
	}
}

type KubectlPortForwarder struct {
	Kind        string
	Name        string
	Ports       []string
	Flags       []string
	RetryPeriod time.Duration
	stop        chan struct{}
	stopped     chan struct{}
}

func (k *KubectlPortForwarder) Setup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		k.stop = make(chan struct{})
		k.stopped = make(chan struct{})
		go func() {
			defer func() { close(k.stopped) }()
			ref := k.Kind
			if ref != "" {
				ref += "/"
			}
			ref += k.Name
			args := []string{"port-forward", ref}
			args = append(args, k.Ports...)
			args = append(args, k.Flags...)
			for {
				err := Kubectl(ctx, cluster, args...).Run()
				if errors.Is(err, context.Canceled) {
					return
				}
				_, stop := <-k.stop
				if stop {
					return
				}
				GinkgoWriter.Printf("Port-forward for %s failed, waiting %s before retrying: %v\n", ref, k.RetryPeriod.String(), err)
			}
		}()
		return nil
	}
}

func (k *KubectlPortForwarder) Cleanup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		close(k.stop)
		<-k.stopped
		return nil
	}
}

func WaitForResourceExists(pollPeriod time.Duration, refs ...types.ResourceReference) types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		for _, ref := range refs {
			for {
				args := []string{"get", ref.Kind, ref.Name}
				if ref.Namespace != "" {
					args = append(args, "--namespace", ref.Namespace)
				}
				err := Kubectl(ctx, cluster, args...).Run()
				if err == nil {
					break
				}
				if errors.Is(err, context.Canceled) {
					return err
				}
				if ref.Namespace != "" {
					GinkgoWriter.Printf("%s %s/%s does not yet exist, waiting...", ref.Kind, ref.Namespace, ref.Name)
				} else {
					GinkgoWriter.Printf("%s %s does not yet exist, waiting...", ref.Kind, ref.Name)
				}
				time.Sleep(pollPeriod)
			}
		}
		return nil
	}
}
