package gingk8s

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

func Kubectl(ctx context.Context, cluster types.Cluster, args ...string) *gosh.Cmd {
	return state.opts.Kubectl.Kubectl(ctx, cluster, args).WithStreams(types.GinkgoOutErr)
}

func KubectlGetSecretValue(ctx context.Context, cluster types.Cluster, name, key string, value *string, args ...string) *gosh.Cmd {
	allArgs := []string{
		"get", "secret", name,
		"--template", fmt.Sprintf(`{{ index .data "%s" }}`, key),
	}
	allArgs = append(allArgs, args...)
	return Kubectl(ctx, cluster, allArgs...).
		WithStreams(
			gosh.FuncOut(func(stdout io.Reader) error {
				var err error
				var bytes []byte
				bytes, err = ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, stdout))
				*value = string(bytes)
				return err
			}),
			types.GinkgoErr,
		)
}

func KubectlReturnSecretValue(ctx context.Context, cluster types.Cluster, name, key string, args ...string) string {
	var out string
	Expect(KubectlGetSecretValue(ctx, cluster, name, key, &out).Run()).To(Succeed())
	return out
}

func KubectlGetSecretBase64(ctx context.Context, cluster types.Cluster, name, key string, value *string, args ...string) *gosh.Cmd {
	allArgs := []string{
		"get", "secret", name,
		"--template", fmt.Sprintf(`{{ index .data "%s" }}`, key),
	}
	allArgs = append(allArgs, args...)
	return Kubectl(ctx, cluster, allArgs...).
		WithStreams(
			gosh.FuncOut(func(stdout io.Reader) error {
				var err error
				var bytes []byte
				bytes, err = ioutil.ReadAll(stdout)
				*value = string(bytes)
				return err
			}),
			types.GinkgoErr,
		)
}

func KubectlExec(ctx context.Context, cluster types.Cluster, name, cmd string, cmdArgs []string, args ...string) *gosh.Cmd {
	allArgs := []string{"exec", "-i", name}
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, "--", cmd)
	allArgs = append(allArgs, cmdArgs...)

	return Kubectl(ctx, cluster, allArgs...).WithStreams(types.GinkgoOutErr)
}

type KubectlWatcher struct {
	Kind  string
	Name  string
	Flags []string
	cmd   *gosh.Cmd
}

func (k *KubectlWatcher) Setup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		args := []string{"get", "-w"}
		if k.Name != "" {
			args = append(args, fmt.Sprintf("%s/%s", k.Kind, k.Name))
		} else {
			args = append(args, k.Kind)
		}
		args = append(args, k.Flags...)
		k.cmd = Kubectl(ctx, cluster, args...)
		return k.cmd.Start()
	}
}

func (k *KubectlWatcher) Cleanup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		err := k.cmd.Kill()
		if err != nil {
			return err
		}
		k.cmd.Wait()
		return nil
	}
}

type KubectlPortForwarder struct {
	Kind        string
	Name        string
	Ports       []string
	Flags       []string
	RetryPeriod time.Duration
	stop        chan struct{}
	stopped     chan struct{}
}

func (k *KubectlPortForwarder) Setup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		k.stop = make(chan struct{})
		k.stopped = make(chan struct{})
		go func() {
			defer func() { close(k.stopped) }()
			ref := k.Kind
			if ref != "" {
				ref += "/"
			}
			ref += k.Name
			args := []string{"port-forward", ref}
			args = append(args, k.Ports...)
			args = append(args, k.Flags...)
			for {
				err := Kubectl(ctx, cluster, args...).Run()
				if errors.Is(err, context.Canceled) {
					return
				}
				_, stop := <-k.stop
				if stop {
					return
				}
				GinkgoWriter.Printf("Port-forward for %s failed, waiting %s before retrying: %v\n", ref, k.RetryPeriod.String(), err)
			}
		}()
		return nil
	}
}

func (k *KubectlPortForwarder) Cleanup() types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		close(k.stop)
		<-k.stopped
		return nil
	}
}

func WaitForResourceExists(pollPeriod time.Duration, refs ...types.ResourceReference) types.ClusterAction {
	return func(ctx context.Context, cluster types.Cluster) error {
		for _, ref := range refs {
			for {
				args := []string{"get", ref.Kind, ref.Name}
				if ref.Namespace != "" {
					args = append(args, "--namespace", ref.Namespace)
				}
				err := Kubectl(ctx, cluster, args...).Run()
				if err == nil {
					break
				}
				if errors.Is(err, context.Canceled) {
					return err
				}
				if ref.Namespace != "" {
					GinkgoWriter.Printf("%s %s/%s does not yet exist, waiting...", ref.Kind, ref.Namespace, ref.Name)
				} else {
					GinkgoWriter.Printf("%s %s does not yet exist, waiting...", ref.Kind, ref.Name)
				}
				time.Sleep(pollPeriod)
			}
		}
		return nil
	}
}
