package gingk8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
	"sigs.k8s.io/yaml"
)

var (
	// DefaultHelmCommand is the command used to execute helm if none is provided
	DefaultHelmCommand = []string{"helm"}
)

// HelmCommand is a reference to an installed helm binary
type HelmCommand struct {
	// Command is the command to execute for helm.
	// If absent, $PATH is used
	Command []string
}

var _ = Helm(&HelmCommand{})

func (h *HelmCommand) helm(ctx context.Context, kube *KubernetesConnection, args []string) gosh.Commander {
	cmd := []string{}
	if len(h.Command) != 0 {
		cmd = append(cmd, h.Command...)
	} else {
		cmd = append(cmd, DefaultHelmCommand...)
	}
	if kube.Kubeconfig != "" {
		cmd = append(cmd, "--kubeconfig", kube.Kubeconfig)
	}
	if kube.Context != "" {
		cmd = append(cmd, "--context", kube.Context)
	}
	cmd = append(cmd, args...)
	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr).WithLog(log)
}

// AddRepo implements Helm
func (h *HelmCommand) AddRepo(ctx context.Context, repo *HelmRepo) gosh.Commander {
	args := []string{"repo", "add", repo.Name, repo.URL}
	args = append(args, repo.Flags...)
	add := h.helm(ctx, &KubernetesConnection{}, args)
	if !repo.Update {
		return add
	}
	args = []string{"repo", "update", repo.Name}
	return gosh.And(add, h.helm(ctx, &KubernetesConnection{}, args))
}

// RegistryLogin implements Helm
func (h *HelmCommand) RegistryLogin(ctx context.Context, registry *HelmRegistry) gosh.Commander {
	args := []string{"registry", "login"}
	args = append(args, registry.LoginFlags...)
	return h.helm(ctx, &KubernetesConnection{}, args)
}

// InstallOrUpgrade implements Helm
func (h *HelmCommand) InstallOrUpgrade(g Gingk8s, ctx context.Context, cluster Cluster, release *HelmRelease) gosh.Commander {
	cmds := []gosh.Commander{}

	args := []string{"upgrade", "--install", release.Name, release.Chart.Fullname()}
	if !release.NoWait {
		args = append(args, "--wait")
	}
	version := release.Chart.Version()
	if version != "" {
		args = append(args, "--version", version)
	}
	args = append(args, release.Chart.UpgradeFlags...)
	args = append(args, release.ExtraFlags...)
	args = append(args, release.UpgradeFlags...)
	if release.Namespace != "" {
		args = append(args, "--namespace", release.Namespace)
	}
	for k, v := range release.Set {
		s := strings.Builder{}
		err := valueString(g, ctx, cluster, &s, v)
		if err != nil {
			panic(err)
		}
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, s.String()))
	}
	for k, v := range release.SetString {
		args = append(args, "--set-string", k+"="+v)
	}
	for k, v := range release.SetFile {
		args = append(args, "--set-file", k+"="+v)
	}
	for _, v := range release.ValuesFiles {
		args = append(args, "--values", v)
	}

	conn := cluster.GetConnection()
	if len(release.Values) != 0 {
		namespacePathPart := release.Namespace
		if namespacePathPart == "" {
			namespacePathPart = "_DEFAULT_"
		}
		valueDir := filepath.Join(ClusterTempPath(cluster, "helm", "releases", namespacePathPart, release.Name, "values"))
		tempValueBasename := func(ix int) string {
			return fmt.Sprintf("%d.yaml", ix)
		}
		tempValuePath := func(ix int) string {
			return filepath.Join(valueDir, tempValueBasename(ix))
		}
		mktemp := gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
			go func() {
				defer GinkgoRecover()
				var err error
				defer func() { done <- err; close(done) }()
				err = os.MkdirAll(valueDir, 0700)
				if err != nil {
					return
				}
				err = func() error {
					for ix, v := range release.Values {
						resolved, err := resolveNestedObject(g, ctx, cluster, v)
						if err != nil {
							return err
						}
						valuesYAML, err := yaml.Marshal(resolved)
						if err != nil {
							return err
						}
						valuesPath := tempValuePath(ix)
						err = os.WriteFile(valuesPath, valuesYAML, 0600)
						if err != nil {
							return err
						}
					}
					return nil
				}()
			}()
			return nil
		})
		for ix := range release.Values {
			args = append(args, "--values", tempValuePath(ix))
		}
		cmds = append(cmds, mktemp)
	}

	if release.Chart.LocalChartInfo.DependencyUpdate {
		cmds = append(cmds, h.helm(ctx, conn, []string{"dependency", "update", release.Chart.Fullname()}))
	}

	cmds = append(cmds, h.helm(ctx, conn, args))

	if len(release.Wait) != 0 {
		cmds = append(cmds, g.KubectlWait(ctx, cluster, release.Wait...))
	}

	return gosh.And(cmds...)
}

// Delete implements Helm
func (h *HelmCommand) Delete(ctx context.Context, cluster Cluster, release *HelmRelease, skipNotExists bool) gosh.Commander {
	args := []string{"delete", release.Name, "--wait"}
	if release.Namespace != "" {
		args = append(args, "--namespace", release.Namespace)
	}
	args = append(args, release.ExtraFlags...)
	args = append(args, release.DeleteFlags...)
	return h.helm(ctx, cluster.GetConnection(), args)
}
