package gingk8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/meln5674/gosh"

	corev1 "k8s.io/api/core/v1"
)

// RandomNamespace creates a new namespace with a randomized name
type RandomNamespace struct {
	namespace *string
	// If running on a cluster without the controller-manager (e.g. envtest), this must be true,
	// otherwise, the namespace will never terminate
	NeedFinalize bool
	// SkipDeleteWait indicates to not wait for the namespace to be finalized
	SkipDeleteWait bool
}

// Get returns the namespace that was created
func (r *RandomNamespace) Get() string {
	if r.namespace == nil {
		panic("RandomNamespace.Get() called before Gingk8s.Setup()")
	}
	return *r.namespace
}

func (r *RandomNamespace) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	namespaceUUID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	name := namespaceUUID.String()
	r.namespace = &name

	return g.Kubectl(ctx, cluster, "create", "namespace", name).Run()
}
func (r *RandomNamespace) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	cmds := []gosh.Commander{
		g.Kubectl(ctx, cluster, "delete", "namespace", *r.namespace, "--wait=false").WithStreams(GinkgoOutErr),
	}
	if r.NeedFinalize {
		cmds = append(cmds, gosh.And(
			gosh.Pipeline(
				g.Kubectl(ctx, cluster, "get", "namespace", *r.namespace, "-o", "json"),
				gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
					go func() {
						defer close(done)
						ns := new(corev1.Namespace)
						err := json.NewDecoder(stdin).Decode(ns)
						if err != nil {
							done <- err
							// panic(err)
							return
						}
						ns.Spec.Finalizers = nil
						err = json.NewEncoder(stdout).Encode(ns)
						if err != nil {
							done <- err
							// panic(err)
							return
						}
					}()
					return nil
				}),
				g.Kubectl(ctx, cluster, "replace", "--raw", fmt.Sprintf("/api/v1/namespaces/%s/finalize", *r.namespace), "-f", "-"),
			)).WithStreams(GinkgoOutErr),
		)
	}
	if !r.SkipDeleteWait {
		cmds = append(cmds, g.Kubectl(ctx, cluster, "wait", "namespace", *r.namespace, "--for=delete").WithStreams(GinkgoOutErr))
	}
	return gosh.And(cmds...).Run()
}
