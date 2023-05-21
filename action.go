package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
)

type ClusterActionID struct {
	id string
}

type ClusterAction func(context.Context, Cluster) error

func (g Gingk8s) ClusterAction(cluster ClusterID, name string, f, cleanup ClusterAction, deps ...ResourceDependencies) ClusterActionID {
	actionID := newID()
	g.clusterActions[actionID] = f

	node := specNode{state: g.specState, id: actionID, dependsOn: []string{cluster.id}, specAction: &clusterActionAction{id: actionID, clusterID: cluster.id, cleanup: cleanup, name: name}}

	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	g.setup = append(g.setup, &node)

	return ClusterActionID{id: actionID}

}

type clusterActionAction struct {
	id        string
	name      string
	clusterID string
	cleanup   ClusterAction
}

func (c *clusterActionAction) Setup(ctx context.Context, state *specState) error {
	if state.suite.opts.NoDeps {
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Executing action %s", c.name))()
	return state.clusterActions[c.id](ctx, state.getCluster(c.clusterID))
}

func (c *clusterActionAction) Cleanup(ctx context.Context, state *specState) {
	if state.suite.opts.NoSuiteCleanup {
		return
	}
	if c.cleanup == nil {
		return
	}

	defer ByStartStop(fmt.Sprintf("Undoing action %s", c.name))()
	Expect(c.cleanup(ctx, state.getCluster(c.clusterID))).To(Succeed())
}
