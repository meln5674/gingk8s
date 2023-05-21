package gingk8s

import (
	"context"
	"flag"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/google/uuid"
	"github.com/meln5674/godag"
	"github.com/meln5674/gosh"
)

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

func ForSuite() Gingk8s {
	klogFlags := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(klogFlags)
	Expect(klogFlags.Parse([]string{"-v=11"})).To(Succeed())

	return Gingk8s{specState: &state.specState}
}

func (g Gingk8s) Options(opts SuiteOpts) {
	g.suite.opts = opts
}

func (g Gingk8s) ForSpec() Gingk8s {
	child := g.child()
	return Gingk8s{specState: &child}
}

func (g *Gingk8s) Setup(ctx context.Context) {
	if g.suite.opts.CustomImageTag == "" {
		g.suite.opts.CustomImageTag = DefaultCustomImageTag
	}
	if g.suite.opts.ExtraCustomImageTags == nil {
		g.suite.opts.ExtraCustomImageTags = DefaultExtraCustomImageTags()
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
		repoAdds = append(repoAdds, g.suite.opts.Helm.AddRepo(ctx, repo))
	}
	Expect(gosh.FanOut(repoAdds...).Run()).To(Succeed())

	ex := godag.Executor[string, *specNode]{}

	for ix := range g.setup {
		g.setup[ix].ctx = ctx
		GinkgoWriter.Printf("Node: %#v\n", g.setup[ix])
	}

	dag, err := godag.Build[string, *specNode](g.setup)
	Expect(err).ToNot(HaveOccurred())

	Expect(ex.Run(dag, godag.Options[string]{})).To(Succeed())
}
