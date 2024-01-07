package gingk8s

import (
	"context"
	"path/filepath"

	"github.com/meln5674/gosh"
)

var (
	// DefaultDockerCommand is the command used to execute odcker if none is provided
	DefaultDockerCommand = []string{"docker"}
)

// DockerCommand is a reference to an install docker client binary
type DockerCommand struct {
	// Command is the command to execute for docker.
	// If absent, $PATH is used
	Command []string
}

func (d *DockerCommand) docker(ctx context.Context, args []string) *gosh.Cmd {
	cmd := []string{}
	if len(d.Command) != 0 {
		cmd = append(cmd, d.Command...)
	} else {
		cmd = append(cmd, DefaultDockerCommand...)
	}
	cmd = append(cmd, args...)
	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr).WithLog(log)
}

func (d *DockerCommand) Docker(ctx context.Context, args ...string) *gosh.Cmd {
	return d.docker(ctx, args)
}

// Pull implements Images
func (d *DockerCommand) Pull(ctx context.Context, image *ThirdPartyImage) gosh.Commander {
	pull := d.docker(ctx, []string{"pull", image.Name})
	tag := d.docker(ctx, []string{"tag", image.Name, image.Retag})
	parts := []gosh.Commander{}
	if !image.NoPull {
		parts = append(parts, pull)
	}
	if image.Retag != "" {
		parts = append(parts, tag)
	}
	return gosh.And(parts...)
}

// Build implements Images
func (d *DockerCommand) Build(ctx context.Context, image *CustomImage, tag string, extraTags []string) gosh.Commander {
	args := []string{"build", "--tag", image.WithTag(tag)}
	for _, tag := range extraTags {
		args = append(args, "--tag", image.WithTag(tag))
	}
	if image.Dockerfile != "" {
		args = append(args, "--file", image.Dockerfile)
	}
	for k, v := range image.BuildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}
	for _, v := range image.Flags {
		args = append(args, v)
	}
	dir := image.ContextDir
	if dir == "" {
		dir = "."
	}
	args = append(args, dir)
	return d.docker(ctx, args)
}

// Save implements Images
func (d *DockerCommand) Save(ctx context.Context, images []string, dest string) (gosh.Commander, ImageFormat) {
	args := []string{"save"}
	args = append(args, images...)
	return gosh.And(
		gosh.FromFunc(ctx, MkdirAll(filepath.Dir(dest), 0700)),
		d.docker(ctx, args).WithStreams(gosh.FileOut(dest)),
	), DockerImageFormat
}

func (d *DockerCommand) Remove(ctx context.Context, images []string) gosh.Commander {
	args := []string{"image", "rm"}
	for _, image := range images {
		args = append(args, image)
	}
	return d.docker(ctx, args)
}
