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
	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr)
}

// AddRepo implements Helm
func (h *HelmCommand) AddRepo(ctx context.Context, repo *HelmRepo) gosh.Commander {
	args := []string{"repo", "add", repo.Name, repo.URL}
	args = append(args, repo.Flags...)
	return h.helm(ctx, &KubernetesConnection{}, args)
}

// InstallOrUpgrade implements Helm
func (h *HelmCommand) InstallOrUpgrade(ctx context.Context, cluster Cluster, release *HelmRelease) gosh.Commander {
	args := []string{"upgrade", "--install", "--wait", release.Name, release.Chart.Fullname()}
	if !release.Chart.IsLocal() && release.Chart.Version != "" {
		args = append(args, "--version", release.Chart.Version)
	}
	args = append(args, release.Chart.UpgradeFlags...)
	args = append(args, release.ExtraFlags...)
	args = append(args, release.UpgradeFlags...)
	if release.Namespace != "" {
		args = append(args, "--namespace", release.Namespace)
	}
	for k, v := range release.Set {
		s := strings.Builder{}
		err := valueString(ctx, cluster, &s, v)
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
	if len(release.Values) == 0 {
		return h.helm(ctx, conn, args)
	}
	namespacePathPart := release.Namespace
	if namespacePathPart == "" {
		namespacePathPart = "_DEFAULT_"
	}
	valueDir := filepath.Join(cluster.GetTempPath("helm", filepath.Join("releases", namespacePathPart, release.Name, "values")))
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
					resolved, err := resolveNestedObject(ctx, cluster, v)
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

	return gosh.And(mktemp, h.helm(ctx, conn, args))
}

// Delete implements Helm
func (h *HelmCommand) Delete(ctx context.Context, cluster Cluster, release *HelmRelease, skipNotExists bool) gosh.Commander {
	args := []string{"delete", release.Name}
	if release.Namespace != "" {
		args = append(args, "--namespace", release.Namespace)
	}
	args = append(args, release.ExtraFlags...)
	args = append(args, release.DeleteFlags...)
	return h.helm(ctx, cluster.GetConnection(), args)
}
