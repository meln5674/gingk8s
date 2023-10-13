package gingk8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
)

const (
	// DefaultCustomImageTag is the default tag to apply to custom images if none is specified
	DefaultCustomImageTag = "gingk8s-latest"
)

var (
	// DefaultExtraCustomImageTags are the default additional tags to apply to custom images if none are specified
	DefaultExtraCustomImageTags = []string{fmt.Sprintf("gingk8s-ts-%d", time.Now().Unix())}

	// DefaultImages is the default interface used to manage images if none is specified.
	// It defaults to using the "docker" command on the $PATH
	DefaultImages = &DockerCommand{}
)

type ThirdPartyImageID struct {
	id string
}

func (t ThirdPartyImageID) AddResourceDependency(dep *ResourceDependencies) {
	dep.ThirdPartyImages = append(dep.ThirdPartyImages, t)
}

func (t ThirdPartyImageID) AddClusterDependency(dep *ClusterDependencies) {
	dep.ThirdPartyImages = append(dep.ThirdPartyImages, t)
}

func (g Gingk8s) ThirdPartyImage(image *ThirdPartyImage) ThirdPartyImageID {
	return g.ThirdPartyImages(image)[0]
}

type ThirdPartyImageIDs []ThirdPartyImageID

func (t ThirdPartyImageIDs) AddResourceDependency(dep *ResourceDependencies) {
	dep.ThirdPartyImages = append(dep.ThirdPartyImages, t...)
}

func (t ThirdPartyImageIDs) AddClusterDependency(dep *ClusterDependencies) {
	dep.ThirdPartyImages = append(dep.ThirdPartyImages, t...)
}

func (g Gingk8s) ThirdPartyImages(images ...*ThirdPartyImage) ThirdPartyImageIDs {
	newIDs := []ThirdPartyImageID{}
	for ix := range images {
		newID := newID()
		newIDs = append(newIDs, ThirdPartyImageID{id: newID})
		g.thirdPartyImages[newID] = images[ix]
		g.setup = append(g.setup, &specNode{state: g.specState, id: newID, specAction: &pullThirdPartyImageAction{id: newID}})
	}
	return newIDs
}

type CustomImageID struct {
	id string
}

func (t CustomImageID) AddResourceDependency(dep *ResourceDependencies) {
	dep.CustomImages = append(dep.CustomImages, t)
}

func (t CustomImageID) AddClusterDependency(dep *ClusterDependencies) {
	dep.CustomImages = append(dep.CustomImages, t)
}

func (g Gingk8s) CustomImage(image *CustomImage) CustomImageID {
	return g.CustomImages(image)[0]
}

type CustomImageIDs []CustomImageID

func (t CustomImageIDs) AddResourceDependency(dep *ResourceDependencies) {
	dep.CustomImages = append(dep.CustomImages, t...)
}

func (t CustomImageIDs) AddClusterDependency(dep *ClusterDependencies) {
	dep.CustomImages = append(dep.CustomImages, t...)
}

func (g Gingk8s) CustomImages(images ...*CustomImage) CustomImageIDs {
	newIDs := []CustomImageID{}
	for ix := range images {
		newID := newID()
		newIDs = append(newIDs, CustomImageID{id: newID})
		g.customImages[newID] = images[ix]
		g.setup = append(g.setup, &specNode{state: g.specState, id: newID, specAction: &buildCustomImageAction{id: newID}})
	}
	return newIDs
}

type pullThirdPartyImageAction struct {
	id string
}

func (p *pullThirdPartyImageAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoPull {
		By(fmt.Sprintf("SKIPPED: Pulling image %s", state.thirdPartyImages[p.id].Name))
		return nil
	}
	By(fmt.Sprintf("Pulling image %s", state.thirdPartyImages[p.id].Name))
	return state.suite.opts.Images.Pull(ctx, state.thirdPartyImages[p.id]).Run()
}

func (p *pullThirdPartyImageAction) Cleanup(ctx context.Context, state *specState) {}

type buildCustomImageAction struct {
	id string
}

func (b *buildCustomImageAction) Setup(ctx context.Context, state *specState) error {
	image := state.customImages[b.id]
	if state.suite.opts.NoBuild {
		By(fmt.Sprintf("SKIPPED: Building image %s", image.WithTag(state.suite.opts.CustomImageTag)))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Building image %s", image.WithTag(state.suite.opts.CustomImageTag)))()
	builder := image.Builder
	if builder == nil {
		builder = state.suite.opts.Images
	}
	return builder.Build(ctx, image, state.suite.opts.CustomImageTag, state.suite.opts.ExtraCustomImageTags).Run()
}

func (b *buildCustomImageAction) Cleanup(ctx context.Context, state *specState) {}

type loadThirdPartyImageAction struct {
	id        string
	clusterID string
	imageID   string
	noCache   bool
}

func (l *loadThirdPartyImageAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoLoadPulled {
		By(fmt.Sprintf("SKIPPED: Loading image %s", state.thirdPartyImages[l.imageID].Name))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Loading image %s", state.thirdPartyImages[l.imageID].Name))()
	return state.getCluster(l.clusterID).LoadImages(ctx, state.suite.opts.Images, state.thirdPartyImageFormats[l.imageID], []string{state.thirdPartyImages[l.imageID].Name}, l.noCache).Run()
}

func (l *loadThirdPartyImageAction) Cleanup(ctx context.Context, state *specState) {}

type loadCustomImageAction struct {
	id        string
	clusterID string
	imageID   string
	noCache   bool
}

func (l *loadCustomImageAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoLoadBuilt {
		By(fmt.Sprintf("SKIPPED: Loading image %s", state.customImages[l.imageID].WithTag(state.suite.opts.CustomImageTag)))
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Loading image %s", state.customImages[l.imageID].WithTag(state.suite.opts.CustomImageTag)))()
	allTags := []string{state.customImages[l.imageID].WithTag(state.suite.opts.CustomImageTag)}
	for _, extra := range state.suite.opts.ExtraCustomImageTags {
		allTags = append(allTags, state.customImages[l.imageID].WithTag(extra))
	}
	return state.getCluster(l.clusterID).LoadImages(ctx, state.suite.opts.Images, state.customImageFormats[l.imageID], allTags, l.noCache).Run()
}

func (l *loadCustomImageAction) Cleanup(ctx context.Context, state *specState) {}

// ThirdPartyImage represents an externally hosted image to be pulled and loaded into the cluster
type ThirdPartyImage struct {
	// Name is the name of the image to pull
	Name string
	// Retag is the name of the local image to re-tag as before loading into the cluster. If absent, will be loaded as Name
	Retag string
	// NoPull indicates the image should not be pulled, e.g. if its built by another local process outside of gingk8s
	NoPull bool
}

// CustomImage represents a custom image to be built from the local filesystem and loaded into the cluster
type CustomImage struct {
	// Registry is the registry component of the image name
	Registry string
	// Repository is the repository component of the image name
	Repository string
	// ContextDir is the directory to build the image from
	ContextDir string
	// Dockerfile is the path relative to ContextDir containing the Dockerfile/Containerfile
	Dockerfile string
	// BuildArgs is a map of --build-arg args
	BuildArgs map[string]string
	// Flags are extra flags to the build command
	Flags []string
	// Builder is the custom image builder, if present, otherwise, the default image builder will be used
	Builder Images
}

func (c *CustomImage) WithTag(tag string) string {
	image := strings.Builder{}
	if c.Registry != "" {
		image.WriteString(c.Registry)
		image.WriteString("/")
	}
	image.WriteString(c.Repository)
	if tag != "" {
		image.WriteString(":")
		image.WriteString(tag)
	}
	return image.String()
}

// ImageFormat is what format an image is exported as
type ImageFormat string

const (
	// DockerImageFormat indicates an image was exported as if from `docker save`
	DockerImageFormat ImageFormat = "docker"
	// OCIImageFormat indicates an image was exported as if from `buildah push`
	OCIImageFormat ImageFormat = "oci"
)

// Images knows how to handle images
type Images interface {
	// Pull pulls (and, if requested, re-tags) third party images
	Pull(ctx context.Context, image *ThirdPartyImage) gosh.Commander
	// Build builds a local image with one or more tags
	Build(ctx context.Context, image *CustomImage, tag string, extraTags []string) gosh.Commander
	// Save exports a set of built images as a tarball and indicates the format it will do so with.
	Save(ctx context.Context, images []string, dest string) (gosh.Commander, ImageFormat)
	// Remove removes an image
	Remove(ctx context.Context, images []string) gosh.Commander
}
