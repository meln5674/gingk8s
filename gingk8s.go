package gingk8s

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/google/uuid"
	"github.com/meln5674/godag"
	"github.com/meln5674/gosh"
)

var log = klog.NewKlogr().WithName("Gingk8s")

func ByStartStop(msg string) func() {
	By(fmt.Sprintf("STARTING: %s", msg))
	return func() { By(fmt.Sprintf("FINISHED: %s", msg)) }
}

func newID() string {
	id, err := uuid.NewRandom()
	Expect(err).ToNot(HaveOccurred())
	return id.String()
}

type Gingk8s struct {
	*specState
}

func ForSuite(g ginkgo.FullGinkgoTInterface) Gingk8s {

	state.ginkgo = g

	return Gingk8s{specState: &state.specState}
}

func (g Gingk8s) Options(opts SuiteOpts) {
	g.suite.opts = opts
}

func (g Gingk8s) GetOptions() SuiteOpts {
	return g.suite.opts
}

func (g Gingk8s) ForSpec() Gingk8s {
	child := g.child()
	return Gingk8s{specState: &child}
}

func (g *Gingk8s) Setup(ctx context.Context) {
	if g.specState.parent == nil || g.suite.opts.KLogFlags != nil {
		klogFlags := flag.NewFlagSet("klog", flag.PanicOnError)
		klog.InitFlags(klogFlags)
		Expect(klogFlags.Parse(g.suite.opts.KLogFlags)).To(Succeed())
		klog.SetOutput(GinkgoWriter)
	}
	if g.suite.opts.CustomImageTag == "" {
		g.suite.opts.CustomImageTag = DefaultCustomImageTag
	}
	if g.suite.opts.ExtraCustomImageTags == nil {
		g.suite.opts.ExtraCustomImageTags = make([]string, len(DefaultExtraCustomImageTags))
		copy(g.suite.opts.ExtraCustomImageTags, DefaultExtraCustomImageTags)
	}
	if g.suite.opts.Images == nil {
		g.suite.opts.Images = DefaultImages
	}
	if g.suite.opts.Manifests == nil {
		g.suite.opts.Manifests = DefaultManifests
	}
	if g.suite.opts.Helm == nil {
		g.suite.opts.Helm = DefaultHelm
	}
	if g.suite.opts.Kubectl == nil {
		g.suite.opts.Kubectl = DefaultKubectl
	}

	repos := make(map[string]*HelmRepo, len(g.releases))
	repoReleases := make(map[string]string)

	for _, release := range g.releases {
		if !release.Chart.IsLocal() && !release.Chart.IsOCI() {
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
		repoAdds = append(repoAdds, g.suite.opts.Helm.AddRepo(ctx, repo))
	}
	Expect(gosh.FanOut(repoAdds...).WithLog(log).Run()).To(Succeed())

	ex := godag.Executor[string, *specNode]{
		Log: log.WithName("Setup"),
	}

	log.V(10).Info("Executing Spec", "spec", fmt.Sprintf("%#v", g.specState))

	nodes := make([]*specNode, len(g.setup))
	copy(nodes, g.setup)
	if g.parent != nil {
		noopIDs := make(
			[]string,
			0,
			len(g.parent.clusters)+
				len(g.parent.thirdPartyImages)+
				len(g.parent.customImages)+
				len(g.parent.releases)+
				len(g.parent.clusterActions)+
				len(g.parent.manifests),
		)
		for id := range g.parent.clusters {
			noopIDs = append(noopIDs, id)
			for _, id := range g.parent.clusterCustomLoads[id] {
				noopIDs = append(noopIDs, id)
			}
			for _, id := range g.parent.clusterThirdPartyLoads[id] {
				noopIDs = append(noopIDs, id)
			}
		}
		for id := range g.parent.thirdPartyImages {
			noopIDs = append(noopIDs, id)
		}
		for id := range g.parent.customImages {
			noopIDs = append(noopIDs, id)
		}
		for id := range g.parent.releases {
			noopIDs = append(noopIDs, id)
		}
		for id := range g.parent.manifests {
			noopIDs = append(noopIDs, id)
		}
		for id := range g.parent.clusterActions {
			noopIDs = append(noopIDs, id)
		}

		for _, id := range noopIDs {
			nodes = append(nodes, &specNode{
				id:         id,
				specAction: &specNoop{},
				state:      g.specState,
			})
		}

		log.V(10).Info("Adding dummy DAG ids", "ids", noopIDs)
	}

	for ix := range nodes {
		nodes[ix].ctx = ctx
	}

	dag, err := godag.Build[string, *specNode](nodes)
	Expect(err).ToNot(HaveOccurred())
	DeferCleanup(func(ctx context.Context) {
		startFrom := godag.NewSet[string]()
		for _, node := range g.cleanup {
			startFrom.Add(node.id)
		}
		if startFrom.Len() == 0 {
			startFrom = godag.Set[string]{}
		}
		cleanupDag := godag.DAG[string, cleanupSpecNode]{Nodes: make(map[string]cleanupSpecNode)}
		for k, v := range dag.Nodes {
			cleanupDag.Nodes[k] = cleanupSpecNode{specNode: v, ctx: ctx}
		}

		cleanupEx := godag.Executor[string, godag.NodeWithDependencies[string, cleanupSpecNode]]{
			Log: klog.NewKlogr().WithName("Cleanup"),
		}

		reversed := godag.Reverse[string, cleanupSpecNode](cleanupDag)
		log.Info("Cleaning up", "reversedDAG", reversed)
		Expect(cleanupEx.Run(ctx, reversed, godag.Options[string]{
			StartFrom: startFrom,
		})).To(Succeed())
	})
	if os.Getenv("GINGK8S_INTERACTIVE") != "" {
		DeferCleanup(func(ctx context.Context) {
			if g.suite.ginkgo.Failed() {
				fmt.Println(g.suite.ginkgo.F("{{red}}{{bold}}This setup has failed and you are running in interactive mode.  Here's a timeline of the spec:{{/}}"))
				fmt.Println(g.suite.ginkgo.Fi(1, g.suite.ginkgo.Name()))
				fmt.Println(g.suite.ginkgo.Fi(1, g.suite.ginkgo.RenderTimeline()))

				fmt.Println(g.suite.ginkgo.F("{{red}}{{bold}}Gingk8s will now sleep so you can interact with the cluster(s).  Hit ^C when you're done to shut down the suite{{/}}"))
				<-ctx.Done()
			}
		})
	}
	log.V(10).Info("Running setup", "dag", dag)
	Expect(ex.Run(ctx, dag, godag.Options[string]{})).To(Succeed())
}

type serializableGingk8s struct {
	Specs      []serializableSpec
	ClusterIDs []string
}

// Serialize takes a gingk8s instance and a set of cluster IDs and serializes them to be
// used with ginkgo.BeforeSuiteSynchronized.
// Only cluster ID's will be valid dependencies in the "rehydrated" gingk8s instance, so
// Serialize() MUST be called AFTER Setup(), as any images, manifests, releases, etc,
// will be inaccessible on the parallel processes.
func (g *Gingk8s) Serialize(clusterIDs ...ClusterID) []byte {
	g2 := serializableGingk8s{
		Specs:      []serializableSpec{},
		ClusterIDs: make([]string, len(clusterIDs)),
	}
	for spec := g.specState; spec != nil; spec = g.specState.parent {
		g2.Specs = append(g2.Specs, spec.serialize())
	}
	// TODO: This is on the honnor system, we don't check if these ID's are valid,
	// but, there shouldn't be a way for a user to construct a cluster id
	for ix, id := range clusterIDs {
		g2.ClusterIDs[ix] = id.id
	}

	out, err := json.Marshal(&g2)
	Expect(err).ToNot(HaveOccurred())
	return out
}

// Deserialize takes the opaque output from Serialize() and restores a stub gingk8s, along with
// the same set of cluster IDs.
// It is the user's responsibility to ensure that the equivalent cluster ID's are passed to both
// Serialize and Deserialize in the same order.
func (g *Gingk8s) Deserialize(in []byte, gt ginkgo.FullGinkgoTInterface, clusterIDs ...*ClusterID) {
	var g2 serializableGingk8s
	Expect(json.Unmarshal(in, &g2)).To(Succeed())

	var suite suiteState
	suite.suite = &suite
	suite.ginkgo = gt

	var parent *specState
	spec := &suite.specState
	for ix := len(g2.Specs) - 1; ix >= 0; ix-- {
		spec.deserialize(parent, g2.Specs[ix])
		parent = spec
	}
	for ix, id := range g2.ClusterIDs {
		clusterIDs[ix].id = id
	}

	g.specState = &suite.specState
}
