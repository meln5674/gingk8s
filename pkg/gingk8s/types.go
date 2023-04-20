package gingk8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"time"

	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"

	"github.com/meln5674/gosh"
)

var (
	// GinkgoErr is a gosh wrapper that forwards stderr to Ginkgo
	GinkgoErr = gosh.WriterErr(GinkgoWriter)
	// GinkgoOut is a gosh wrapper that forwards stdout to Gingko
	GinkgoOut = gosh.WriterOut(GinkgoWriter)
	// GinkgoOut is a gosh wrapper that forwards stdout and stderr to Gingko
	GinkgoOutErr = gosh.SetStreams(GinkgoOut, GinkgoErr)
)

var (
	// DefaultKindCommand is the command used to execute kind if none is provided
	DefaultKindCommand = []string{"kind"}
	// DefaultHelmCommand is the command used to execute helm if none is provided
	DefaultHelmCommand = []string{"helm"}
	// DefaultKubectlCommand is the command used to execute kubectl if none is provided
	DefaultKubectlCommand = []string{"kubectl"}
	// DefaultKustomizeCommand is the command used to execute kustomize if none is provided
	DefaultKustomizeCommand = []string{"kustomize"}
	// DefaultDockerCommand is the command used to execute odcker if none is provided
	DefaultDockerCommand = []string{"docker"}
	// DefaultBuildahCommand is the command used to execute buildah if none is provided
	DefaultBuildahCommand = []string{"buildah"}
	// DefaultDeleteCommand is the command used to remove directories if none is provided
	DefaultDeleteCommand = []string{"rm", "-rf"}

	// DefaultCustomImageTag is the default tag to apply to custom images if none is specified
	DefaultCustomImageTag = "gingk8s-latest"

	// DefaultImages is the default interface used to manage images if none is specified.
	// It defaults to using the "docker" command on the $PATH
	DefaultImages = &DockerCommand{}
	// DefaultManifests is the default interface used to manage manifests if none is specified.
	// It defaults to using the "kubectl" command on the $PATH
	DefaultManifests = &KubectlCommand{}
	// DefaultKubectl is the default interface used to do kubectl things if none is specified
	// It defaults to using the "kubectl" command on the path.
	DefaultKubectl = &KubectlCommand{}
	// DefaultKubectl is the default interface used to manage helm repos, charts, and releasess if none is specified
	// It defaults to using the "helm" command on the path.
	DefaultHelm = &HelmCommand{}
	// DefaultKubectl is the default interface used to manage kind clusters if none is specified
	// It defaults to using the "kind" command on the path.
	DefaultKind = &KindCommand{}
)

// Value is any valid value, but represents that values should be one of the following
// Scalars (strings, bools, numbers)
// Arrays/Slices of valid values (See `Array`)
// A function with argument list of (), (context.Context) or (context.Context, Cluster) and return types of (Value) or (Value, error)
// values of these types will be correctly handled and evaluated
type Value = interface{}

// NestedValues must be either a `Value`, `Object`, or `Array`
type NestedValue = interface{}

// Object is a map from string keys to values
type Object = map[string]Value

// StringObject is a map from string keys to string values
type StringObject = map[string]string

// Array is a slice of values
type Array = []Value

// A NestedObject is like an `Object`, but recursive
type NestedObject = map[string]NestedValue

func MkdirAll(path string, mode os.FileMode) gosh.Func {
	return func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		go func() {
			var err error
			defer func() { done <- err; close(done) }()
			err = os.MkdirAll(path, mode)
		}()
		return nil
	}
}
func DefaultExtraCustomImageTags() []string {
	return []string{fmt.Sprintf("gingk8s-ts-%d", time.Now().Unix())}
}

// SuiteOpts controls the behavior of the suite
type SuiteOpts struct {
	// NoSuiteCleanup disables deleting the cluster after the suite has finishes
	NoSuiteCleanup bool
	// NoSpecCleanup disables deleting third party resources after specs have completed
	NoSpecCleanup bool
	// NoPull disables pulling third party images
	NoPull bool
	// NoLoadPulled disables loading pulled third party images
	NoLoadPulled bool
	// NoBuild disables building custom images
	NoBuild bool
	// NoLoadBuilt disables loading built custom images
	NoLoadBuilt bool
	// NoDeps disables re-installing third party resources
	NoDeps bool

	// CustomImageTag is the tag to set for all custom images
	CustomImageTag string
	// ExtraCustomImageTags are a set of extra tags to set for all custom images
	ExtraCustomImageTags []string

	// Images is how to pull, build, and save images
	Images Images
	// Manifests is how to deploy and delete kubernetes manifests
	Manifests Manifests
	// Helm is how to deploy and delete helm charts
	Helm Helm
	// Kubectl is how to execute kubectl
	Kubectl Kubectl
}

// KubernetesConnection defines a connection to a kubernetes API server
type KubernetesConnection struct {
	// Kubeconfig is the path to a kubeconfig file. If blank, the default loading rules are used.
	Kubeconfig string
	// Context is the name of the context within the kubeconfig file to use. If blank, the current context is used.
	Context string
}

type WaitFor struct {
	Resource string
	For      StringObject
	Flags    []string
}

// KubernetesResources is a set of kubernetes manifests from literal strings, files, and directories.
// Ordering of resources is not guaranteed, and may be performed concurrently
type KubernetesManifests struct {
	// Name is a human-readable name to give the manifest set in logs
	Name string
	// Resources are strings containing the manifests
	Resources []string
	// ResourcePaths are paths to individual resource files and (non-recursive) directories
	ResourcePaths []string
	// ResourceRecursiveDirs are paths to directories recursively containing resource files
	ResourceRecursiveDirs []string
	// Replace indicates these resources should be replaced, not applied
	Replace bool
	Wait    []WaitFor
}

// Manifests knows how to manage raw kubernetes manifests
type Manifests interface {
	// CreateOrUpdate creates or updates a set of manifests
	CreateOrUpdate(ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander
	// Delete removes a set of manifests
	Delete(ctx context.Context, cluster Cluster, manifests *KubernetesManifests) gosh.Commander
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

// HelmRepo represents a repository to pull helm charts from
type HelmRepo struct {
	// Name is the name of the repository to use in `helm repo add`
	Name string
	// URL is the URL to provide in `helm repo add`
	URL string
	// Flags are any extra flags to provide to the `helm repo add` command
	Flags []string
}

// LocalChartInfo is a reference to a helm chart from a local directory or tarball
type LocalChartInfo struct {
	// Path is the path to the local directory or tarball
	Path string
}

// RemoteChartInfo is a reference to a helm chart from a remote repository
type RemoteChartInfo struct {
	// Name is the name of the chart to be installed from a remote repo
	Name string
	// Repo is a reference to the HelmRepo object representing the respository to pull the chart from
	Repo *HelmRepo
	// Version is the version of the chart to install. Will use latest if omitted.
	Version string
}

// HelmChart represents a chart to be installed
type HelmChart struct {
	// LocalChartInfo is the location of the local chart. Mutually exclusive with RemoteChartInfo
	LocalChartInfo
	// RemoteChartInfo is the location of the remote chart. Mutually exclusive with LocalChartInfo
	RemoteChartInfo

	// UpgradeFlags are extra flags to pass to the `helm upgrade` command
	UpgradeFlags []string
}

func (h *HelmChart) IsLocal() bool {
	return h.Path != ""
}

func (h *HelmChart) Fullname() string {
	if h.IsLocal() {
		if !strings.HasPrefix(h.Path, "./") && !strings.HasPrefix(h.Path, "../") {
			return "./" + h.Path
		} else {
			return h.Path
		}
	} else {
		return h.Repo.Name + "/" + h.Name
	}
}

// HelmRelease represents a helm chart to be released into the cluster
type HelmRelease struct {
	// Name is the name of the release
	Name string
	// Namespace is the namespace of the release. Will use whatever the kubeconfig context namespace is omitted (usually "default")
	Namespace string
	// Chart is a reference to the chart to release
	Chart *HelmChart
	// Set is a map of --set arguments
	Set Object
	// SetString is a map of --set-string arguments
	SetString StringObject
	// SetFile is a map of --set-file arguments
	SetFile StringObject
	// ValuesFiles is a list of files to be provided with --values
	ValuesFiles []string
	// Values is a set of objects to be serialized as YAML files and provided with --values
	Values []NestedObject
	// ExtraFlags is a set of extra arguments to pass to both helm upgrade and delete
	ExtraFlags []string
	// UpgradeFlags is a set of extra arguments to pass to helm upgrade
	UpgradeFlags []string
	// DeleteFlags is a set of extra arguments to pass to helm delete
	DeleteFlags []string
}

// Helm knows how to install and uninstall helm charts
type Helm interface {
	// AddRepo adds a repo that only this HelmReleaser can use
	AddRepo(ctx context.Context, repo *HelmRepo) gosh.Commander
	// InstallOrUpgrade upgrades or installs a release into a cluster
	InstallOrUpgrade(ctx context.Context, cluster Cluster, release *HelmRelease) gosh.Commander
	// Delete removes a release from a cluster. If skipNotExists is true, this should not fail if the release does not exist.
	Delete(ctx context.Context, cluster Cluster, release *HelmRelease, skipNotExists bool) gosh.Commander
}

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

func scalarString(ctx context.Context, cluster Cluster, s *strings.Builder, v Value) error {
	s.WriteString(strings.ReplaceAll(fmt.Sprintf("%v", v), ",", `\,`))
	return nil
}

func resolveRArray(ctx context.Context, cluster Cluster, val reflect.Value) ([]interface{}, error) {
	var err error
	out := make([]interface{}, val.Len())
	for ix := 0; ix < val.Len(); ix++ {
		out[ix], err = resolveRValue(ctx, cluster, val.Index(ix))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func resolveRNestedArray(ctx context.Context, cluster Cluster, val reflect.Value) ([]interface{}, error) {
	var err error
	out := make([]interface{}, val.Len())
	for ix := 0; ix < val.Len(); ix++ {
		out[ix], err = resolveRNestedValue(ctx, cluster, val.Index(ix))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rarrayString(ctx context.Context, cluster Cluster, s *strings.Builder, val reflect.Value) error {
	s.WriteString("{")
	defer s.WriteString("}")
	for ix := 0; ix < val.Len(); ix++ {
		if ix != 0 {
			s.WriteString(",")
		}
		err := rvalueString(ctx, cluster, s, val.Index(ix))
		if err != nil {
			return err
		}
	}
	return nil
}

func resolveRFunc(ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	typ := val.Type()
	/*
		tooManyArgs := typ.NumIn() > 2
		firstArgNotContext := typ.NumIn() > 0 && !typ.In(0).AssignableTo(reflect.ValueOf(context.Context(nil)).Type())
		secondArgNotCluster := typ.NumIn() > 1 && !typ.In(1).AssignableTo(reflect.ValueOf(Cluster(nil)).Type())
		notEnoughReturns := typ.NumOut() == 0
		tooManyReturns := typ.NumOut() > 2
		secondReturnNotError := typ.NumOut() == 2 && !typ.Out(1).AssignableTo(reflect.ValueOf(error(nil)).Type())

		badType := tooManyArgs ||
			firstArgNotContext ||
			secondArgNotCluster ||
			notEnoughReturns ||
			tooManyReturns ||
			secondReturnNotError
		if badType {
			panic(fmt.Sprintf("func() values provided to Set must take zero arguments, a context.Context, or a context.Context and a gingk8s.Cluster, and must return a compliant type or a compliant type and an error. Instead, got %v", typ))
		}
	*/
	// Go reflection is dumb, there doesn't appear to be a way to check if a function argument is an interface, so we just have to not have a nice error message
	var in []reflect.Value
	if typ.NumIn() > 0 {
		in = append(in, reflect.ValueOf(ctx))
	}
	if typ.NumIn() > 1 {
		in = append(in, reflect.ValueOf(cluster))
	}
	out := val.Call(in)
	if typ.NumOut() == 2 {
		errV := out[1].Interface()
		if errV != nil {
			return out[0].Interface(), errV.(error)
		}
	}
	return resolveRValue(ctx, cluster, out[0])
}
func resolveRNestedFunc(ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	typ := val.Type()
	/*
		tooManyArgs := typ.NumIn() > 2
		firstArgNotContext := typ.NumIn() > 0 && !typ.In(0).AssignableTo(reflect.ValueOf(context.Context(nil)).Type())
		secondArgNotCluster := typ.NumIn() > 1 && !typ.In(1).AssignableTo(reflect.ValueOf(Cluster(nil)).Type())
		notEnoughReturns := typ.NumOut() == 0
		tooManyReturns := typ.NumOut() > 2
		secondReturnNotError := typ.NumOut() == 2 && !typ.Out(1).AssignableTo(reflect.ValueOf(error(nil)).Type())

		badType := tooManyArgs ||
			firstArgNotContext ||
			secondArgNotCluster ||
			notEnoughReturns ||
			tooManyReturns ||
			secondReturnNotError
		if badType {
			panic(fmt.Sprintf("func() values provided to Set must take zero arguments, a context.Context, or a context.Context and a gingk8s.Cluster, and must return a compliant type or a compliant type and an error. Instead, got %v", typ))
		}
	*/
	// Go reflection is dumb, there doesn't appear to be a way to check if a function argument is an interface, so we just have to not have a nice error message
	var in []reflect.Value
	if typ.NumIn() > 0 {
		in = append(in, reflect.ValueOf(ctx))
	}
	if typ.NumIn() > 1 {
		in = append(in, reflect.ValueOf(cluster))
	}
	out := val.Call(in)
	if typ.NumOut() == 2 {
		errV := out[1].Interface()
		if errV != nil {
			return out[0].Interface(), errV.(error)
		}
	}
	return resolveRNestedValue(ctx, cluster, out[0])
}
func rfuncString(ctx context.Context, cluster Cluster, s *strings.Builder, val reflect.Value) error {
	out, err := resolveRFunc(ctx, cluster, val)
	if err != nil {
		return err
	}
	return valueString(ctx, cluster, s, out)
}

func rvalueString(ctx context.Context, cluster Cluster, s *strings.Builder, val reflect.Value) error {
	typ := val.Type()
	switch val.Kind() {
	case reflect.Interface:
		return rvalueString(ctx, cluster, s, val.Elem())
	case reflect.Array, reflect.Slice:
		return rarrayString(ctx, cluster, s, val)
	case reflect.Func:
		return rfuncString(ctx, cluster, s, val)
	case reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Map, reflect.Struct, reflect.UnsafePointer:
		return fmt.Errorf("Helm substitution does not support %v values (%#v). Values must be typical scalar, arrays/slices of valid types, and functions return return valid types", typ, val)
	}
	return scalarString(ctx, cluster, s, val)
}
func resolveRValue(ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	typ := val.Type()
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		return resolveRValue(ctx, cluster, val.Elem())
	case reflect.Array, reflect.Slice:
		return resolveRArray(ctx, cluster, val)
	case reflect.Func:
		return resolveRFunc(ctx, cluster, val)
	case reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Map, reflect.Struct, reflect.UnsafePointer:
		return nil, fmt.Errorf("Helm substitution does not support %v values (%#v). Values must be typical scalar, arrays/slices of valid types, and functions return return valid types", typ, val)
	}
	return val.Interface(), nil
}

func valueString(ctx context.Context, cluster Cluster, s *strings.Builder, v Value) error {
	return rvalueString(ctx, cluster, s, reflect.ValueOf(v))
}

func resolveRNestedValue(ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		return resolveRNestedValue(ctx, cluster, val.Elem())
	case reflect.Array, reflect.Slice:
		return resolveRNestedArray(ctx, cluster, val)
	case reflect.Func:
		return resolveRNestedFunc(ctx, cluster, val)
	case reflect.Map:
		var err error
		out := make(map[string]interface{}, val.Len())
		iter := val.MapRange()
		for iter.Next() {
			out[iter.Key().Interface().(string)], err = resolveRNestedValue(ctx, cluster, iter.Value())
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return resolveRValue(ctx, cluster, val)
}
func resolveNestedValue(ctx context.Context, cluster Cluster, v NestedValue) (interface{}, error) {
	return resolveRNestedValue(ctx, cluster, reflect.ValueOf(v))
}

func resolveNestedObject(ctx context.Context, cluster Cluster, o NestedObject) (interface{}, error) {
	return resolveNestedValue(ctx, cluster, o)
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
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, s.String())) // TODO: use reflect to find arrays and correctly serialize them
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
	// Save exports a built image as a tarball and indicates the format it will do so with
	Save(ctx context.Context, image string, dest string) (gosh.Commander, ImageFormat)
}

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
	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr)
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
func (d *DockerCommand) Save(ctx context.Context, image string, dest string) (gosh.Commander, ImageFormat) {
	return gosh.And(
		gosh.FromFunc(ctx, MkdirAll(filepath.Dir(dest), 0700)),
		d.docker(ctx, []string{"save", image}).WithStreams(gosh.FileOut(dest)),
	), DockerImageFormat
}

// Clsuter knows how to manage a temporary cluster
type Cluster interface {
	// Create creates a cluster. If skipExisting is true, it will not fail if the cluster already exists
	Create(ctx context.Context, skipExisting bool) gosh.Commander
	// GetConnection returns the kubeconfig, context, etc, to use to connect to the cluster's api server.
	// GetConnection must not fail, so any potentially failing operations must be done during Create()
	GetConnection() *KubernetesConnection
	// GetTempPath returns the path to a file or directory to use for temporary operations against this cluster
	GetTempPath(group string, path string) string
	// LoadImages loads a set of images of a given format from.
	LoadImages(ctx context.Context, from Images, format ImageFormat, images []string) gosh.Commander
	// Delete deletes the cluster. Delete should not fail if the cluster exists.
	Delete(ctx context.Context) gosh.Commander
}

// KindCommand is a reference to a kind binary
type KindCommand struct {
	// Command is the command to execute for kind.
	// If absent, $PATH is used
	Command []string
}

func (k *KindCommand) kind(ctx context.Context, args []string) *gosh.Cmd {
	cmd := []string{}
	if len(k.Command) != 0 {
		cmd = append(cmd, k.Command...)
	} else {
		cmd = append(cmd, DefaultKindCommand...)
	}
	cmd = append(cmd, args...)
	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr)
}

// KindCluster represents a kubernetes cluster made with `kind create cluster`
type KindCluster struct {
	*KindCommand
	// Name is the name of the cluster, the --name argument to kind
	Name string
	// TempDir is the location to store all temporary files related to the cluster
	TempDir string
	// ConfigFilePath is the path to the kind configuration YAML file
	ConfigFilePath string
	// ConfigFileTemplatePath, if present, is a path to a gotemplate file, which, when rendered, produces the kind configuration YAML file.
	// If set, ConfigFilePath will be used to store the output of the template, otherwise, a file under TempDir will be used.
	// .Env will be set to the current process environment variables
	// .Data will bet set to ConfigFileTemplateData
	ConfigFileTemplatePath string
	// ConfigFileTemplateData is data to be passed to ConfigFileTemplatePath
	ConfigFileTemplateData interface{}

	// CleanupDirs are paths, relative to TempDir, that should be deleted after the cluster is deleted.
	// This can be used to, for example, mount /var/lib/kubelet to a local directory and clean it after tests
	CleanupDirs []string
	// DeleteCommand is the command to delete CleanupDirs with.
	// If absent, a standard rm -rf is used.
	// Overriding is necessary if these directories may be mounted into the container, and thus, may be owned by root.
	DeleteCommand []string
}

func (k *KindCluster) kind(ctx context.Context, args []string) *gosh.Cmd {
	allArgs := []string{}
	if len(args) > 0 && args[0] != "get" && args[1] != "clusters" {
		allArgs = append(allArgs, "--name", k.Name)
	}
	allArgs = append(allArgs, args...)
	return k.kindCommand().kind(ctx, allArgs)
}

func (k *KindCluster) KubeconfigPath() string {
	return filepath.Join(k.TempDir, "kubeconfig")
}

func (k *KindCluster) kindCommand() *KindCommand {
	if k.KindCommand != nil {
		return k.KindCommand
	}
	return DefaultKind
}

// Create implements Cluster
func (k *KindCluster) Create(ctx context.Context, skipExisting bool) gosh.Commander {
	mkdir := gosh.FromFunc(ctx, MkdirAll(k.TempDir, 0700))
	configPath := k.ConfigFilePath
	if configPath == "" && k.ConfigFileTemplatePath != "" {
		configPath = filepath.Join(k.TempDir, "config.yaml")
	}
	mkConfig := gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		go func() {
			var err error
			defer func() { done <- err; close(done) }()
			templateBytes, err := os.ReadFile(k.ConfigFileTemplatePath)
			if err != nil {
				return
			}
			configTemplate, err := template.New(k.ConfigFileTemplatePath).Parse(string(templateBytes))
			if err != nil {
				return
			}
			env := map[string]string{}
			for _, line := range os.Environ() {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 1 {
					parts = append(parts, "")
				}
				env[parts[0]] = parts[1]
			}
			templateData := map[string]interface{}{
				"Env":  env,
				"Data": k.ConfigFileTemplateData,
			}
			f, err := os.Create(k.ConfigFileTemplatePath)
			if err != nil {
				return
			}
			defer f.Close()
			err = configTemplate.Execute(f, templateData)
		}()
		return nil
	})

	clusterExists := gosh.Pipeline(
		k.kind(ctx, []string{"get", "clusters"}),
		gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
			go func() {
				var err error
				defer func() { done <- err; close(done) }()
				lines := bufio.NewScanner(stdin)
				for lines.Scan() {
					if lines.Text() == k.Name {
						err = nil
						return
					}
				}
				err = lines.Err()
				if err != nil {
					return
				}
				err = fmt.Errorf("No cluster name %s found", k.Name)
			}()
			return nil
		}),
	)

	args := []string{
		"create", "cluster",
		"--kubeconfig", k.KubeconfigPath(),
	}

	if configPath != "" {
		args = append(args, "--config", configPath)
	}

	createCluster := k.kind(ctx, args)

	var create gosh.Commander
	if skipExisting {
		create = gosh.Or(clusterExists, createCluster)
	} else {
		create = createCluster
	}
	if k.ConfigFileTemplatePath != "" {
		return gosh.And(mkdir, mkConfig, create)
	} else {
		return gosh.And(mkdir, create)
	}
}

// GetConnection implements cluster
func (k *KindCluster) GetConnection() *KubernetesConnection {
	return &KubernetesConnection{
		Kubeconfig: k.KubeconfigPath(),
	}
}

// GetTempPath implements cluster
func (k *KindCluster) GetTempPath(group string, path string) string {
	return filepath.Join(k.TempDir, "groups", group, path)
}

// LoadImages implements cluster
func (k *KindCluster) LoadImages(ctx context.Context, from Images, format ImageFormat, images []string) gosh.Commander {
	saves := []gosh.Commander{}
	for _, image := range images {
		path := filepath.Join(k.TempDir, "images", string(format), strings.ReplaceAll(image, ":", "/")+".tar")
		save, _ := from.Save(ctx, image, path)
		load := k.kind(ctx, []string{"load", "image-archive", path})
		saves = append(saves, gosh.And(save, load))
	}
	return gosh.FanOut(saves...)
}

// Delete implements cluster
func (k *KindCluster) Delete(ctx context.Context) gosh.Commander {
	deleteCluster := k.kind(ctx, []string{"delete", "cluster"})
	toDelete := []string{}
	for _, path := range k.CleanupDirs {
		toDelete = append(toDelete, filepath.Join(k.TempDir, path))
	}
	rmCmd := []string{}
	if len(k.DeleteCommand) != 0 {
		rmCmd = append(rmCmd, k.DeleteCommand...)
	} else {
		rmCmd = append(rmCmd, DefaultDeleteCommand...)
	}
	rmCmd = append(rmCmd, toDelete...)
	if len(toDelete) != 0 {
		return gosh.And(deleteCluster, gosh.Command(rmCmd...).WithStreams(GinkgoOutErr))
	} else {
		return deleteCluster
	}
}

type ClusterAction func(context.Context, Cluster) error

type ResourceReference struct {
	Namespace string
	Kind      string
	Name      string
}
