package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type ThirdPartyImageID struct {
	id string
}

type CustomImageID struct {
	id string
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
		state.setup = append(state.setup, &specNode{id: newID, specAction: &pullThirdPartyImageAction{id: newID}})
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
		state.setup = append(state.setup, &specNode{id: newID, specAction: &buildCustomImageAction{id: newID}})
	}
	return newIDs
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
	for _, extra := range state.opts.ExtraCustomImageTags {
		allTags = append(allTags, state.customImages[l.imageID].WithTag(extra))
	}
	return state.clusters[l.clusterID].LoadImages(ctx, state.opts.Images, state.customImageFormats[l.imageID], allTags).Run()
}

func (l *loadCustomImageAction) Cleanup(ctx context.Context) {}
