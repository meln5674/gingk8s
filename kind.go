package gingk8s

import (
	"bufio"
	"context"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/yaml"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
)

var (
	// DefaultKindCommand is the command used to execute kind if none is provided
	DefaultKindCommand = []string{"kind"}
	// DefaultKind is the default interface used to manage kind clusters if none is specified
	// It defaults to using the "kind" command on the path.
	DefaultKind = &KindCommand{}
)

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
	return gosh.Command(cmd...).WithContext(ctx).WithStreams(GinkgoOutErr).WithLog(log)
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

var _ = Cluster(&KindCluster{})

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
			defer GinkgoRecover()
			var err error
			defer func() {
				done <- err
				close(done)
			}()
			if k.ConfigFileTemplatePath == "" {
				log.Info("Kind config file template path is not set, assuming pre-made configuration path is ready", "path", configPath)
				return
			}
			defer func() {
				if err != nil {
					log.Info("FAILED: Generating Kind config for clusterfrom template", "path", configPath, "templatePath", k.ConfigFileTemplatePath, "error", err)
				}
			}()
			err = func() error {
				log.Info("Generating Kind config for cluster from template", "path", configPath, "templatePath", k.ConfigFileTemplatePath)

				templateBytes, err := os.ReadFile(k.ConfigFileTemplatePath)
				if err != nil {
					return err
				}
				configTemplate, err := template.New(k.ConfigFileTemplatePath).Funcs(map[string]interface{}{"realpath": filepath.Abs}).Parse(string(templateBytes))
				if err != nil {
					return err
				}
				env := map[string]string{}
				for _, line := range os.Environ() {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 1 {
						parts = append(parts, "")
					}
					env[parts[0]] = parts[1]
				}
				pwd, err := os.Getwd()
				if err != nil {
					return err
				}
				templateData := map[string]interface{}{
					"Env":  env,
					"Data": k.ConfigFileTemplateData,
					"Pwd":  pwd,
				}
				f, err := os.Create(configPath)
				if err != nil {
					return err
				}
				defer f.Close()
				err = configTemplate.Execute(f, templateData)
				if err != nil {
					return err
				}
				f.Close()
				config, err := os.ReadFile(configPath)
				if err != nil {
					return err
				}
				if err != nil {
					return err
				}
				log.Info("SUCCEEDED: Generating Kind config for cluster from template", "path", configPath, "templatePath", k.ConfigFileTemplatePath, "output", string(config))
				return nil
			}()
		}()
		return nil
	})

	clusterExists := gosh.Pipeline(
		k.kind(ctx, []string{"get", "clusters"}),
		gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
			go func() {
				defer GinkgoRecover()
				var err error
				defer func() { done <- err; close(done) }()
				lines := bufio.NewScanner(stdin)
				for lines.Scan() {
					if lines.Text() == k.Name {
						err = nil
						log.Info("Cluster exists, not creating", "name", k.Name)
						return
					}
				}
				err = lines.Err()
				if err != nil {
					return
				}
				err = fmt.Errorf("No cluster name %s found", k.Name)
				log.Info("Cluster does not exist, will create", "name", k.Name)
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
	return gosh.And(mkdir, mkConfig, create)
}

// GetConnection implements cluster
func (k *KindCluster) GetConnection() *KubernetesConnection {
	return &KubernetesConnection{
		Kubeconfig: k.KubeconfigPath(),
	}
}

// GetTempPath implements cluster
func (k *KindCluster) GetTempDir() string {
	return k.TempDir
}

// GetName implements cluster
func (k *KindCluster) GetName() string {
	if k.Name == "" {
		return "KinD"
	}
	return k.Name
}

// LoadImages implements cluster
func (k *KindCluster) LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander {
	if len(images) == 0 {
		return gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
			close(done)
			return nil
		})
	}
	saves := []gosh.Commander{}
	allSame := true
	last := ""
	archivePaths := make([]string, len(images))
	for ix, image := range images {
		parts := strings.Split(image, ":")
		if last != "" && parts[0] != last {
			allSame = false
			break
		}
		last = parts[0]
		archivePaths[ix] = filepath.Join(k.TempDir, "images", strings.Join(parts, "/"))
	}
	if allSame {
		cmds := make([]gosh.Commander, 0, 4)
		save, _ := from.Save(ctx, images, archivePaths[0])
		cmds = append(cmds, save)
		if noCache {
			cmds = append(cmds, from.Remove(ctx, images))
		}
		load := k.kind(ctx, []string{"load", "image-archive", archivePaths[0]})
		cmds = append(cmds, load)
		if noCache {
			cmds = append(cmds, gosh.FromFunc(ctx, Rm(archivePaths[0])))
		}
		return gosh.And(cmds...)
	}

	for ix, path := range archivePaths {
		cmds := make([]gosh.Commander, 0, 4)
		save, _ := from.Save(ctx, []string{images[ix]}, path)
		cmds = append(cmds, save)
		if noCache {
			cmds = append(cmds, from.Remove(ctx, []string{images[ix]}))
		}
		load := k.kind(ctx, []string{"load", "image-archive", path})
		cmds = append(cmds, load)
		if noCache {
			cmds = append(cmds, gosh.FromFunc(ctx, Rm(archivePaths[ix])))
		}
		saves = append(saves, gosh.And(cmds...))
	}
	return gosh.FanOut(saves...).WithLog(log)
}

// LoadImageArchive implements cluster
func (k *KindCluster) LoadImageArchives(ctx context.Context, format ImageFormat, archives []string) gosh.Commander {
	loads := make([]gosh.Commander, len(archives))
	for ix, archive := range archives {
		loads[ix] = k.kind(ctx, []string{"load", "image-archive", archive})
	}
	return gosh.FanOut(loads...).WithLog(log)
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
		return gosh.And(deleteCluster, gosh.Command(rmCmd...).WithStreams(GinkgoOutErr).WithLog(log))
	} else {
		return deleteCluster
	}
}

type KindNetworkInfo struct {
	NetworkCidr   string
	NodeIPs       map[string]string
	NodeHostnames []string
	PodCIDR       string
	ServiceCIDR   string
	ServiceDomain string
}

func (k *KindCluster) GetNetworkInfo(ctx context.Context, g Gingk8s) (*KindNetworkInfo, error) {
	var info KindNetworkInfo
	var err error

	var nodeList corev1.NodeList
	err = g.Kubectl(ctx, k, "get", "nodes", "-o", "json").WithStreams(gosh.FuncOut(gosh.SaveJSON(&nodeList))).Run()
	if err != nil {
		return nil, err
	}
	info.NodeIPs = make(map[string]string, len(nodeList.Items))
	info.NodeHostnames = make([]string, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		var nodeIP, nodeHostname string
		for _, addr := range node.Status.Addresses {
			switch addr.Type {
			case corev1.NodeInternalIP:
				nodeIP = addr.Address
			case corev1.NodeHostName:
				nodeHostname = addr.Address
			}
		}
		if nodeIP != "" && nodeHostname != "" {
			info.NodeIPs[nodeHostname] = nodeIP
			info.NodeHostnames = append(info.NodeHostnames, nodeHostname)
		}
	}

	var kubeadmConfigMap corev1.ConfigMap
	err = g.Kubectl(ctx, k, "get", "cm", "kubeadm-config", "-n", "kube-system", "-o", "json").WithStreams(gosh.FuncOut(gosh.SaveJSON(&kubeadmConfigMap))).Run()
	if err != nil {
		return nil, err
	}
	var kubeadmConfig map[string]interface{}
	err = yaml.Unmarshal([]byte(kubeadmConfigMap.Data["ClusterConfiguration"]), &kubeadmConfig)
	if err != nil {
		return nil, err
	}
	networkingConfig := kubeadmConfig["networking"].(map[string]interface{})
	info.PodCIDR = networkingConfig["podSubnet"].(string)
	info.ServiceCIDR = networkingConfig["serviceSubnet"].(string)
	info.ServiceDomain = networkingConfig["dnsDomain"].(string)

	return &info, nil
}
