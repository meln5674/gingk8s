package gingk8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meln5674/gosh"
)

var (
	// DefaultCustomImageTag is the default tag to apply to custom images if none is specified
	DefaultCustomImageTag = "gingk8s-latest"

	// DefaultImages is the default interface used to manage images if none is specified.
	// It defaults to using the "docker" command on the $PATH
	DefaultImages = &DockerCommand{}
)

func DefaultExtraCustomImageTags() []string {
	return []string{fmt.Sprintf("gingk8s-ts-%d", time.Now().Unix())}
}

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
	// Save exports a set of built images as a tarball and indicates the format it will do so with
	Save(ctx context.Context, images []string, dest string) (gosh.Commander, ImageFormat)
}
