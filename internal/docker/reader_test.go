package docker

import (
	"testing"

	"github.com/docker/docker/api/types/swarm"
)

func TestActiveSwarmRecognizesWorker(t *testing.T) {
	if !activeSwarm(swarm.Info{LocalNodeState: swarm.LocalNodeStateActive}) {
		t.Fatal("active worker should be recognized as participating in Swarm")
	}
}

func TestActiveSwarmRejectsInactiveNode(t *testing.T) {
	if activeSwarm(swarm.Info{LocalNodeState: swarm.LocalNodeStateInactive}) {
		t.Fatal("inactive node should not be recognized as active Swarm")
	}
}
