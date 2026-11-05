package pconfig

import (
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/transport"
)

func TestConfigureEndpointFallbackWithoutExplicitHost(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	ep := transport.NewEndpoint()
	if err := configureEndpoint(ep, transport.Source{}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if _, err := ep.Dial(t.Context()); err != nil {
		t.Fatalf("dial placeholder: %v", err)
	}
}

func TestConfigureEndpointFailsOnExplicitBadHost(t *testing.T) {
	ep := transport.NewEndpoint()
	err := configureEndpoint(ep, transport.Source{ConnectionURI: "ssh://nobody@127.0.0.1:1/run/podman/podman.sock"})
	if err == nil {
		t.Fatal("expected error for unreachable explicit ssh URI")
	}
}
