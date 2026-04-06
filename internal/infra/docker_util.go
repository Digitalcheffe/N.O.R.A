package infra

import "github.com/google/uuid"

// dockerContainerNS is the fixed UUID v5 namespace used to derive stable
// source IDs for Docker containers from a (engineID, containerName) pair.
// Using a name-based key means metrics survive container ID changes caused
// by stop/remove/recreate cycles.
var dockerContainerNS = uuid.MustParse("c2d3e4f5-a6b7-8901-bcde-f12345678901")

// StableContainerSourceID returns a deterministic UUID for a container that
// belongs to a specific Docker engine, keyed on the container name rather than
// the ephemeral Docker container ID.  The same (engineID, containerName) pair
// always produces the same UUID.
func StableContainerSourceID(engineID, containerName string) string {
	return uuid.NewSHA1(dockerContainerNS, []byte(engineID+"/"+containerName)).String()
}
