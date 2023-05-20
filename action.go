package gingk8s

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"

	types "github.com/meln5674/gingk8s/pkg/gingk8s"
)

type ClusterActionID struct {
	id string
}

func ClusterAction(cluster ClusterID, name string, f, cleanup types.ClusterAction, deps ...ResourceDependencies) ClusterActionID {
	actionID := newID()
	state.clusterActions[actionID] = f

	node := specNode{id: actionID, dependsOn: []string{cluster.id}, specAction: &clusterActionAction{id: actionID, clusterID: cluster.id, cleanup: cleanup, name: name}}

	for _, deps := range deps {
		node.dependsOn = append(node.dependsOn, deps.allIDs(cluster.id)...)
	}

	state.setup = append(state.setup, &node)

	return ClusterActionID{id: actionID}

}

type clusterActionAction struct {
	id        string
	name      string
	clusterID string
	cleanup   types.ClusterAction
}

func (c *clusterActionAction) Setup(ctx context.Context) error {
	if state.opts.NoDeps {
		return nil
	}
	defer ByStartStop(fmt.Sprintf("Executing action %s", c.name))()
	return state.clusterActions[c.id](ctx, state.clusters[c.clusterID])
}

func (c *clusterActionAction) Cleanup(ctx context.Context) {
	if state.opts.NoSuiteCleanup {
		return
	}
	if c.cleanup == nil {
		return
	}

	defer ByStartStop(fmt.Sprintf("Undoing action %s", c.name))()
	Expect(c.cleanup(ctx, state.clusters[c.clusterID])).To(Succeed())
}
