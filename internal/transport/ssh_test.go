package transport

import (
	"testing"
)

func TestParseSSHURI(t *testing.T) {
	t.Parallel()
	spec, err := parseSSHURI("ssh://alice@example.com:2222/run/user/1000/podman/podman.sock")
	if err != nil {
		t.Fatal(err)
	}
	if spec.User != "alice" || spec.Host != "example.com" || spec.Port != "2222" {
		t.Fatalf("got %+v", spec)
	}
	if spec.RemoteSocket != "/run/user/1000/podman/podman.sock" {
		t.Fatalf("socket = %q", spec.RemoteSocket)
	}
}

func TestParseSSHURIDefaults(t *testing.T) {
	t.Setenv("USER", "bob")
	spec, err := parseSSHURI("ssh://pod.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if spec.User != "bob" || spec.Port != "22" || spec.RemoteSocket != "/run/podman/podman.sock" {
		t.Fatalf("got %+v", spec)
	}
}

func TestParseEndpointURISsh(t *testing.T) {
	got, err := ParseEndpointURI("ssh://root@host/run/podman/podman.sock")
	if err != nil {
		t.Fatal(err)
	}
	if got.SSH == nil || got.Direct != nil {
		t.Fatalf("got %+v", got)
	}
}
