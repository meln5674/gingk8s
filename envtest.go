package gingk8s

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/meln5674/gosh"

	. "github.com/onsi/ginkgo/v2"

	configv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

type EnvTestCluster struct {
	Environment envtest.Environment
	TempDir     string
	Name        string
}

var _ = Cluster(&EnvTestCluster{})

func (e *EnvTestCluster) KubeconfigPath() string {
	return filepath.Join(e.TempDir, "kubeconfig")
}

// Create creates a cluster. If skipExisting is true, it will not fail if the cluster already exists
func (e *EnvTestCluster) Create(ctx context.Context, skipExisting bool) gosh.Commander {
	if e.Environment.ControlPlane.APIServer == nil {
		e.Environment.ControlPlane.APIServer = &envtest.APIServer{}
	}
	if e.Environment.ControlPlane.APIServer.Out == nil {
		e.Environment.ControlPlane.APIServer.Out = GinkgoWriter
	}
	if e.Environment.ControlPlane.APIServer.Err == nil {
		e.Environment.ControlPlane.APIServer.Err = GinkgoWriter
	}
	if e.Environment.ControlPlane.Etcd == nil {
		e.Environment.ControlPlane.Etcd = &envtest.Etcd{}
	}
	if e.Environment.ControlPlane.Etcd.Out == nil {
		e.Environment.ControlPlane.Etcd.Out = GinkgoWriter
	}
	if e.Environment.ControlPlane.Etcd.Err == nil {
		e.Environment.ControlPlane.Etcd.Err = GinkgoWriter
	}

	return gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		go func() {
			defer close(done)
			_, err := e.Environment.Start()
			if err != nil {
				done <- err
				return
			}
			cfg := &configv1.Config{
				Clusters: []configv1.NamedCluster{
					{
						Name: "default",
						Cluster: configv1.Cluster{
							Server:                   e.Environment.Config.Host,
							TLSServerName:            e.Environment.Config.TLSClientConfig.ServerName,
							InsecureSkipTLSVerify:    e.Environment.Config.TLSClientConfig.Insecure,
							CertificateAuthority:     e.Environment.Config.TLSClientConfig.CAFile,
							CertificateAuthorityData: e.Environment.Config.TLSClientConfig.CAData,
						},
					},
				},
				AuthInfos: []configv1.NamedAuthInfo{
					{
						Name: "default",
						AuthInfo: configv1.AuthInfo{
							ClientCertificate:     e.Environment.Config.TLSClientConfig.CertFile,
							ClientCertificateData: e.Environment.Config.TLSClientConfig.CertData,
							ClientKey:             e.Environment.Config.TLSClientConfig.KeyFile,
							ClientKeyData:         e.Environment.Config.TLSClientConfig.KeyData,
							Token:                 e.Environment.Config.BearerToken,
							TokenFile:             e.Environment.Config.BearerTokenFile,
							Username:              e.Environment.Config.Username,
							Password:              e.Environment.Config.Password,
						},
					},
				},
				Contexts: []configv1.NamedContext{
					{
						Name: "default",
						Context: configv1.Context{

							Cluster:   "default",
							AuthInfo:  "default",
							Namespace: "default",
						},
					},
				},
				CurrentContext: "default",
			}
			cfgBytes, err := yaml.Marshal(cfg)
			if err != nil {
				done <- err
				return
			}
			err = os.WriteFile(e.KubeconfigPath(), cfgBytes, 0600)
			if err != nil {
				done <- err
				return
			}
		}()
		return nil
	})
}

// GetConnection returns the kubeconfig, context, etc, to use to connect to the cluster's api server.
// GetConnection must not fail, so any potentially failing operations must be done during Create()
func (e *EnvTestCluster) GetConnection() *KubernetesConnection {
	return &KubernetesConnection{
		Kubeconfig: e.KubeconfigPath(),
		Context:    "default",
	}
}

// GetTempDir returns the path to a file or directory to use for temporary operations against this cluster
func (e *EnvTestCluster) GetTempDir() string {
	return e.TempDir
}

// GetName returns a user-supplied name for this cluster
func (e *EnvTestCluster) GetName() string {
	name := e.Name
	if name != "" {
		return name
	}
	return "envtest"
}

// LoadImages implements Cluster
func (e *EnvTestCluster) LoadImages(ctx context.Context, from Images, format ImageFormat, images []string, noCache bool) gosh.Commander {
	panic("LoadImages is not supported by EnvTest")
}

// LoadImageArchives implements Cluster
func (e *EnvTestCluster) LoadImageArchives(ctx context.Context, format ImageFormat, archives []string) gosh.Commander {
	panic("LoadImageArchives is not supported by EnvTest")
}

// Delete deletes the cluster. Delete should not fail if the cluster exists.
func (e *EnvTestCluster) Delete(ctx context.Context) gosh.Commander {
	return gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		go func() {
			defer close(done)
			os.Remove(e.KubeconfigPath())
			done <- e.Environment.Stop()
		}()
		return nil
	})
}
