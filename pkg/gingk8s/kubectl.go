package gingk8s

import (
	"context"
	"io"
	"reflect"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
	"sigs.k8s.io/yaml"
)

var (
	// DefaultKubectlCommand is the command used to execute kubectl if none is provided
	DefaultKubectlCommand = []string{"kubectl"}
	// DefaultKubectl is the default interface used to do kubectl things if none is specified
	// It defaults to using the "kubectl" command on the path.
	DefaultKubectl = &KubectlCommand{}
)

// Kubectl knows how to execute kubectl commands
type Kubectl interface {
	Kubectl(ctx context.Context, cluster Cluster, args []string) *gosh.Cmd
}

// KubectlCommand is a reference to a kubectl binary
type KubectlCommand struct {
	// Command is the command to execute for kubectl.
	// If absent, $PATH is used
	Command []string
}

func (k *KubectlCommand) Kubectl(ctx context.Context, cluster Cluster, args []string) *gosh.Cmd {
	cmd := []string{}
	if len(k.Command) != 0 {
		cmd = append(cmd, k.Command...)
	} else {
		cmd = append(cmd, DefaultKubectlCommand...)
	}
	conn := cluster.GetConnection()
	if conn.Kubeconfig != "" {
		cmd = append(cmd, "--kubeconfig", conn.Kubeconfig)
	}
	if conn.Context != "" {
		cmd = append(cmd, "--context", conn.Context)
	}
	cmd = append(cmd, args...)

	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr)
}

// CreateOrUpates implements Manifests
func (k *KubectlCommand) CreateOrUpdate(ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander {
	applies := []gosh.Commander{}
	applyFileArgs := func(path string, recursive bool) []string {
		args := []string{"apply", "--filename", path}
		if manifests.Replace {
			args = append(args, "--replace")
		}
		if recursive {
			args = append(args, "--recursive")
		}
		return args
	}
	if len(manifests.ResourceObjects) != 0 {
		applies = append(applies, gosh.Pipeline(
			gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
				go func() {
					defer GinkgoRecover()
					var err error
					defer func() { done <- err; close(done) }()
					err = func() error {
						for _, obj := range manifests.ResourceObjects {
							var err error
							var resolved interface{}
							switch v := obj.(type) {
							case Object:
								resolved, err = resolveRObject(ctx, cluster, reflect.ValueOf(v))
							case NestedObject:
								resolved, err = resolveRNestedObject(ctx, cluster, reflect.ValueOf(v))
							default:
								resolved = v
							}
							if err != nil {
								return err
							}
							objBytes, err := yaml.Marshal(resolved)
							if err != nil {
								return err
							}
							_, err = stdout.Write(objBytes)
							if err != nil {
								return err
							}
							_, err = stdout.Write([]byte("\n---\n"))
							if err != nil {
								return err
							}
						}
						return nil
					}()
				}()
				return nil
			}),
			k.Kubectl(ctx, cluster, applyFileArgs("-", false)),
		))
	}
	for _, resource := range manifests.Resources {
		applies = append(applies, k.Kubectl(ctx, cluster, applyFileArgs("-", false)).WithStreams(gosh.StringIn(resource)))
	}
	for _, path := range manifests.ResourcePaths {
		applies = append(applies, k.Kubectl(ctx, cluster, applyFileArgs(path, false)))
	}
	for _, path := range manifests.ResourceRecursiveDirs {
		applies = append(applies, k.Kubectl(ctx, cluster, applyFileArgs(path, true)))
	}

	waits := []gosh.Commander{}
	for _, wait := range manifests.Wait {
		args := []string{"wait", wait.Resource}
		for k, v := range wait.For {
			args = append(args, "--for", k+"="+v)
		}
		waits = append(waits, k.Kubectl(ctx, cluster, args))
	}
	return gosh.And(gosh.FanOut(applies...), gosh.FanOut(waits...))
}

// Delete implements Manifests
func (k *KubectlCommand) Delete(ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander {
	// TODO: this could be refactored
	cmds := []gosh.Commander{}
	applyFileArgs := func(path string, recursive bool) []string {
		args := []string{"delete", "--filename", path}
		if manifests.Replace {
			args = append(args, "--replace")
		}
		if recursive {
			args = append(args, "--recursive")
		}
		return args
	}
	for _, resource := range manifests.Resources {
		cmds = append(cmds, k.Kubectl(ctx, cluster, applyFileArgs("-", false)).WithStreams(gosh.StringIn(resource)))
	}
	for _, path := range manifests.ResourcePaths {
		cmds = append(cmds, k.Kubectl(ctx, cluster, applyFileArgs(path, false)))
	}
	for _, path := range manifests.ResourceRecursiveDirs {
		cmds = append(cmds, k.Kubectl(ctx, cluster, applyFileArgs(path, true)))
	}
	//return gosh.FanOut(cmds...)
	return gosh.And(cmds...)
}
