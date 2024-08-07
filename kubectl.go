package gingk8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	// DefaultKubectlCommand is the command used to execute kubectl if none is provided
	DefaultKubectlCommand = []string{"kubectl"}
	// DefaultKubectl is the default interface used to do kubectl things if none is specified
	// It defaults to using the "kubectl" command on the path.
	DefaultKubectl = &KubectlCommand{}
)

func (g Gingk8s) Kubectl(ctx context.Context, cluster Cluster, args ...string) *gosh.Cmd {
	return g.suite.opts.Kubectl.Kubectl(ctx, cluster, args).WithStreams(GinkgoOutErr)
}

func (g Gingk8s) KubectlGetSecretValue(ctx context.Context, cluster Cluster, name, key string, value *string, args ...string) *gosh.Cmd {
	allArgs := []string{
		"get", "secret", name,
		"--template", fmt.Sprintf(`{{ index .data "%s" }}`, key),
	}
	allArgs = append(allArgs, args...)
	return g.Kubectl(ctx, cluster, allArgs...).
		WithStreams(
			gosh.FuncOut(func(stdout io.Reader) error {
				var err error
				var bytes []byte
				bytes, err = ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, stdout))
				*value = string(bytes)
				return err
			}),
			GinkgoErr,
		)
}

func (g Gingk8s) KubectlReturnSecretValue(ctx context.Context, cluster Cluster, name, key string, args ...string) string {
	var out string
	Expect(g.KubectlGetSecretValue(ctx, cluster, name, key, &out).Run()).To(Succeed())
	return out
}

func (g Gingk8s) KubectlGetSecretBase64(ctx context.Context, cluster Cluster, name, key string, value *string, args ...string) *gosh.Cmd {
	allArgs := []string{
		"get", "secret", name,
		"--template", fmt.Sprintf(`{{ index .data "%s" }}`, key),
	}
	allArgs = append(allArgs, args...)
	return g.Kubectl(ctx, cluster, allArgs...).
		WithStreams(
			gosh.FuncOut(func(stdout io.Reader) error {
				var err error
				var bytes []byte
				bytes, err = ioutil.ReadAll(stdout)
				*value = string(bytes)
				return err
			}),
			GinkgoErr,
		)
}

func (g Gingk8s) KubectlExec(ctx context.Context, cluster Cluster, name, cmd string, cmdArgs []string, args ...string) *gosh.Cmd {
	allArgs := []string{"exec", "-i", name}
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, "--", cmd)
	allArgs = append(allArgs, cmdArgs...)

	return g.Kubectl(ctx, cluster, allArgs...).WithStreams(GinkgoOutErr)
}

func (g Gingk8s) KubectlGetServiceNodePorts(ctx context.Context, cluster Cluster, name string, args ...string) (map[string]int32, error) {
	var err error

	var svc corev1.Service
	err = g.Kubectl(ctx, cluster, append([]string{"get", "svc", name, "-o", "json"}, args...)...).WithStreams(gosh.FuncOut(gosh.SaveJSON(&svc))).Run()
	if err != nil {
		return nil, err
	}

	ports := make(map[string]int32, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		if port.Name == "" {
			continue
		}
		if port.NodePort == 0 {
			continue
		}
		ports[port.Name] = port.NodePort
	}
	return ports, nil
}

type KubectlWatcher struct {
	Kind   string
	Name   string
	Flags  []string
	cmd    *gosh.Cmd
	cancel func()
}

func (k *KubectlWatcher) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	args := []string{"get", "-w"}
	if k.Name != "" {
		args = append(args, fmt.Sprintf("%s/%s", k.Kind, k.Name))
	} else {
		args = append(args, k.Kind)
	}
	ctx, k.cancel = context.WithCancel(ctx)
	args = append(args, k.Flags...)
	k.cmd = g.Kubectl(ctx, cluster, args...)
	return k.cmd.Start()
}

func (k *KubectlWatcher) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	k.cancel()
	return nil
}

type KubectlPortForwarder struct {
	Kind        string
	Name        string
	Ports       []string
	Flags       []string
	RetryPeriod time.Duration
	stop        chan struct{}
	cancel      func()
	stopped     chan struct{}
}

func (k *KubectlPortForwarder) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	k.stop = make(chan struct{})
	k.stopped = make(chan struct{})
	ctx, k.cancel = context.WithCancel(ctx)
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
			err := g.Kubectl(ctx, cluster, args...).Run()
			if errors.Is(err, context.Canceled) {
				return
			}
			select {
			case <-k.stop:
				return
			default:
				log.Info("Port-forward failed, waiting before retrying", "resource", ref, "delay", k.RetryPeriod.String(), "error", err)
				time.Sleep(k.RetryPeriod)
			}
		}
	}()
	return nil
}

func (k *KubectlPortForwarder) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	close(k.stop)
	k.cancel()
	<-k.stopped
	return nil
}

type KubectlLogger struct {
	Kind          string
	Name          string
	Flags         []string
	RetryPeriod   time.Duration
	StopOnSuccess bool
	stop          chan struct{}
	cancel        func()
	stopped       chan struct{}
}

func (k *KubectlLogger) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	if k.stopped != nil {
		Fail("Logger setup called twice: " + k.Name)
	}
	k.stop = make(chan struct{})
	k.stopped = make(chan struct{})
	ctx, k.cancel = context.WithCancel(ctx)
	since := time.Time{}
	go func() {
		defer func() { close(k.stopped) }()
		ref := k.Kind
		if ref != "" {
			ref += "/"
		}
		ref += k.Name
		args := []string{"logs", "-f", ref}
		if since != (time.Time{}) {
			args = append(args, "--since", since.String())
		}
		args = append(args, k.Flags...)
		for {
			err := g.Kubectl(ctx, cluster, args...).Run()
			since = time.Now()
			if errors.Is(err, context.Canceled) {
				log.Info("Kubectl logger was canceled")
				return
			}
			if err == nil && k.StopOnSuccess {
				log.Info("Kubectl Logs exited without error, not retrying", "resource", ref)
				return
			}
			select {
			case <-k.stop:
				return
			default:
				log.Info("Kubectl Logs failed, waiting before retrying", "resource", ref, "delay", k.RetryPeriod.String(), "error", err)
				time.Sleep(k.RetryPeriod)
			}
		}
	}()
	return nil
}

func (k *KubectlLogger) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	close(k.stop)
	k.cancel()
	<-k.stopped
	k.stop = nil
	k.stopped = nil
	return nil
}

func (g Gingk8s) KubectlWait(ctx context.Context, cluster Cluster, fors ...WaitFor) gosh.Commander {
	waits := []gosh.Commander{}
	for _, wait := range fors {
		args := []string{"wait", wait.Resource}
		for k, v := range wait.For {
			if v == "" {
				args = append(args, "--for", k)
			} else {
				args = append(args, "--for", k+"="+v)
			}
		}
		waits = append(waits, g.suite.opts.Kubectl.Kubectl(ctx, cluster, args))
	}
	return gosh.FanOut(waits...).WithLog(log)
}

func (g Gingk8s) KubectlRollout(ctx context.Context, cluster Cluster, ref ResourceReference) gosh.Commander {
	argsRestart := []string{"rollout", "restart", fmt.Sprintf("%s/%s", ref.Kind, ref.Name)}
	argsStatus := []string{"rollout", "status", fmt.Sprintf("%s/%s", ref.Kind, ref.Name)}
	if ref.Namespace != "" {
		argsRestart = append(argsRestart, "--namespace", ref.Namespace)
		argsStatus = append(argsStatus, "--namespace", ref.Namespace)
	}
	return gosh.And(
		g.Kubectl(ctx, cluster, argsRestart...),
		g.Kubectl(ctx, cluster, argsStatus...),
	)
}

func (g Gingk8s) WaitForResourceExists(pollPeriod time.Duration, refs ...ResourceReference) ClusterAction {
	return func(g Gingk8s, ctx context.Context, cluster Cluster) error {
		for _, ref := range refs {
			for {
				args := []string{"get", ref.Kind, ref.Name}
				if ref.Namespace != "" {
					args = append(args, "--namespace", ref.Namespace)
				}
				err := g.Kubectl(ctx, cluster, args...).Run()
				if err == nil {
					break
				}
				if errors.Is(err, context.Canceled) {
					return err
				}
				if ref.Namespace != "" {
					log.Info("Resource does not yet exist, waiting...", "kind", ref.Kind, "namespace", ref.Namespace, "name", ref.Name)
				} else {
					log.Info("Cluster Resource does not yet exist, waiting...", "kind", ref.Kind, "name", ref.Name)
				}
				time.Sleep(pollPeriod)
			}
		}
		return nil
	}
}

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

	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr).WithLog(log)
}

func (k *KubectlCommand) ResourceObjectsYAML(g Gingk8s, ctx context.Context, cluster Cluster, out io.Writer, objects []interface{}) error {
	for _, obj := range objects {
		var err error
		var resolved interface{}
		switch v := obj.(type) {
		case Object:
			resolved, err = resolveRObject(g, ctx, cluster, reflect.ValueOf(v))
		case NestedObject:
			resolved, err = resolveRNestedObject(g, ctx, cluster, reflect.ValueOf(v))
		default:
			resolved = v
		}
		if err != nil {
			return err
		}
		objBytes, err := json.Marshal(resolved)
		if err != nil {
			return err
		}
		_, err = out.Write(objBytes)
		if err != nil {
			return err
		}
		// _, err = out.Write([]byte("\n---\n"))
		// if err != nil {
		// 	return err
		// }
	}
	return nil
}

// CreateOrUpates implements Manifests
func (k *KubectlCommand) CreateOrUpdate(g Gingk8s, ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander {
	applies := []gosh.Commander{}
	applyFileArgs := func(path string, recursive bool) []string {
		var args []string
		if manifests.Create {
			args = []string{"create", "--filename", path, "--output", "yaml"}
		} else {
			args = []string{"apply", "--server-side", "--filename", path, "--output", "yaml"}
		}
		if manifests.Replace {
			args = append(args, "--replace")
		}
		if recursive {
			args = append(args, "--recursive")
		}
		if manifests.Namespace != "" {
			args = append(args, "-n", manifests.Namespace)
		}
		return args
	}
	createdIx := 0
	readYAMLs := func(stdout io.Reader) error {
		GinkgoRecover()
		if manifests.Created == nil {
			io.ReadAll(stdout)
			return nil
		}
		dec := yaml.NewYAMLOrJSONDecoder(stdout, 1024)
		for createdIx < len(manifests.Created) {
			err := dec.Decode(&manifests.Created[createdIx])
			createdIx++
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		io.ReadAll(stdout)
		return nil
	}
	objectsToYAML := func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		go func() {
			defer GinkgoRecover()
			var err error
			defer func() { done <- err; close(done) }()
			err = func() error {
				return k.ResourceObjectsYAML(g, ctx, cluster, stdout, manifests.ResourceObjects)
			}()
		}()
		return nil
	}
	if len(manifests.ResourceObjects) != 0 {
		applies = append(applies, gosh.Pipeline(
			gosh.FromFunc(ctx, objectsToYAML),
			k.Kubectl(ctx, cluster, applyFileArgs("-", false)).WithStreams(gosh.FuncOut(readYAMLs)),
		))
	}
	for _, resource := range manifests.Resources {
		applies = append(applies, k.Kubectl(ctx, cluster, applyFileArgs("-", false)).WithStreams(gosh.StringIn(resource), gosh.FuncOut(readYAMLs)))
	}
	for _, path := range manifests.ResourcePaths {
		applies = append(applies, k.Kubectl(ctx, cluster, applyFileArgs(path, false)).WithStreams(gosh.FuncOut(readYAMLs)))
	}
	for _, path := range manifests.ResourceRecursiveDirs {
		applies = append(applies, k.Kubectl(ctx, cluster, applyFileArgs(path, true)).WithStreams(gosh.FuncOut(readYAMLs)))
	}

	waits := []gosh.Commander{}
	for _, wait := range manifests.Wait {
		args := []string{"wait", wait.Resource}
		for k, v := range wait.For {
			args = append(args, "--for", k+"="+v)
		}
		waits = append(waits, k.Kubectl(ctx, cluster, args))
	}
	return gosh.And(gosh.FanOut(applies...).WithLog(log), gosh.FanOut(waits...).WithLog(log))
}

// Delete implements Manifests
func (k *KubectlCommand) Delete(g Gingk8s, ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander {
	// TODO: this could be refactored
	cmds := []gosh.Commander{}
	applyFileArgs := func(path string, recursive bool) []string {
		args := []string{"delete", "--filename", path}
		if recursive {
			args = append(args, "--recursive")
		}
		if manifests.Namespace != "" {
			args = append(args, "-n", manifests.Namespace)
		}
		if manifests.SkipDeleteWait {
			args = append(args, "--wait=false")
		}
		return args
	}
	if len(manifests.ResourceObjects) != 0 {
		cmds = append(cmds, gosh.Pipeline(
			gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
				go func() {
					defer GinkgoRecover()
					var err error
					defer func() { done <- err; close(done) }()
					err = func() error {
						return k.ResourceObjectsYAML(g, ctx, cluster, stdout, manifests.ResourceObjects)
					}()
				}()
				return nil
			}),
			k.Kubectl(ctx, cluster, applyFileArgs("-", false)),
		))
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
	//return gosh.FanOut(cmds...).WithLog(log)
	return gosh.And(cmds...)
}
