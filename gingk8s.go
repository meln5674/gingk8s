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

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
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

	ex := godag.Executor[string, *specNode]{}

	for ix := range state.setup {
		state.setup[ix].ctx = ctx
		GinkgoWriter.Printf("Node: %#v\n", state.setup[ix])
	}

	dag, err := godag.Build[string, *specNode](state.setup)
	Expect(err).ToNot(HaveOccurred())

	Expect(ex.Run(dag, godag.Options[string]{})).To(Succeed())
}
