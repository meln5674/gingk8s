package gingk8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/meln5674/gosh"

	corev1 "k8s.io/api/core/v1"
)

// RandomNamespace creates a new namespace with a randomized name
type RandomNamespace struct {
	namespace string
	// If running on a cluster without the controller-manager (e.g. envtest), this must be true,
	// otherwise, the namespace will never terminate
	NeedFinalize bool
}

// Get returns the namespace that was created
func (r *RandomNamespace) Get() string {
	return r.namespace
}

func (r *RandomNamespace) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	namespaceUUID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	r.namespace = namespaceUUID.String()

	return g.Kubectl(ctx, cluster, "create", "namespace", r.namespace).Run()
}
func (r *RandomNamespace) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	cmds := []gosh.Commander{
		g.Kubectl(ctx, cluster, "delete", "namespace", r.namespace).WithStreams(GinkgoOutErr),
	}
	if r.NeedFinalize {
		// https://stackoverflow.com/a/62959634/17621440
		// tl;dr: EnvTest can't finalize namespaces, and kubectl can't finalize objects, by design, it seems, so we have to do this nonsense.
		finalize := gosh.And(
			// If we execute the finalize after the delete, the delete never finishes
			// If we execute the finalize and delete concurrently, we get a conflict
			// This is a hack that hopefully waits long enough for the delete to actually have "started"
			gosh.FromFunc(ctx, func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
				go func() {
					defer close(done)
					time.Sleep(5 * time.Second)
				}()
				return nil
			}),
			gosh.Pipeline(
				g.Kubectl(ctx, cluster, "get", "namespace", r.namespace, "-o", "json"),
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
				g.Kubectl(ctx, cluster, "replace", "--raw", fmt.Sprintf("/api/v1/namespaces/%s/finalize", r.namespace), "-f", "-"),
			)).WithStreams(GinkgoOutErr)
		cmds = append(cmds, finalize)
	}
	return gosh.FanOut(cmds...).Run()
}
