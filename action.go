package gingk8s

import (
	"context"
	"fmt"

	"github.com/meln5674/gosh"
	. "github.com/onsi/gomega"
)

type ClusterActionID struct {
	id string
}

func (c ClusterActionID) AddResourceDependency(dep *ResourceDependencies) {
	dep.ClusterActions = append(dep.ClusterActions, c)
}

type ClusterActionable interface {
	Setup(Gingk8s, context.Context, Cluster) error
	Cleanup(Gingk8s, context.Context, Cluster) error
}

type ClusterAction func(Gingk8s, context.Context, Cluster) error

func (c ClusterAction) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	return c(g, ctx, cluster)
}

func (c ClusterAction) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	return nil
}

type ClusterCommander func(Gingk8s, context.Context, Cluster) gosh.Commander

func (c ClusterCommander) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	return c(g, ctx, cluster).Run()
}

func (c ClusterCommander) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	return nil
}

type ClusterActionFuncs struct {
	SetupFunc   func(Gingk8s, context.Context, Cluster) error
	CleanupFunc func(Gingk8s, context.Context, Cluster) error
}

func (c ClusterActionFuncs) Setup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	return c.SetupFunc(g, ctx, cluster)
}

func (c ClusterActionFuncs) Cleanup(g Gingk8s, ctx context.Context, cluster Cluster) error {
	return c.CleanupFunc(g, ctx, cluster)
}

func (g Gingk8s) ClusterAction(cluster ClusterID, name string, c ClusterActionable, deps ...ResourceDependency) ClusterActionID {
	actionID := newID()
	g.clusterActions[actionID] = c.Setup

	dependsOn := append([]string{cluster.id}, forResourceDependencies(deps...).allIDs(cluster.id)...)
	node := specNode{
		state:     g.specState,
		id:        actionID,
		dependsOn: dependsOn,
		specAction: &clusterActionAction{
			id:        actionID,
			clusterID: cluster.id,
			cleanup:   c.Cleanup,
			name:      name,
			g:         g,
		},
	}

	g.setup = append(g.setup, &node)

	return ClusterActionID{id: actionID}

}

type clusterActionAction struct {
	id        string
	name      string
	clusterID string
	cleanup   ClusterAction
	g         Gingk8s
}

func (c *clusterActionAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoDeps {
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Executing action %s", c.name))()
	return state.clusterActions[c.id](c.g, ctx, state.getCluster(c.clusterID))
}

func (c *clusterActionAction) Cleanup(ctx context.Context, state *specState) {
	if state.suite.opts.NoSuiteCleanup {
		return
	}
	if c.cleanup == nil {
		return
	}

	defer ByStartStop(fmt.Sprintf("Undoing action %s", c.name))()
	Expect(c.cleanup(c.g, ctx, state.getCluster(c.clusterID))).To(Succeed())
}
