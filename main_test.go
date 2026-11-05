package main

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/specdata"
	"github.com/geoffsee/pulumi-podman/internal/transport"
)

func TestProviderVersion(t *testing.T) {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(providerVersion) {
		t.Fatalf("VERSION = %q", providerVersion)
	}
}

func TestWriteTempSpec(t *testing.T) {
	path, cleanup, err := writeTempSpec()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, specdata.AdaptedSpec) {
		t.Fatalf("temp spec %s does not match embedded AdaptedSpec (%d vs %d bytes)", path, len(got), len(specdata.AdaptedSpec))
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleanup left %s: %v", path, err)
	}
}

func TestBootstrapEndpointFallbackOnSSH(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "ssh://root@host/run/podman/podman.sock")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	ep := transport.NewEndpoint()
	err := bootstrapEndpoint(ep)
	if err == nil {
		t.Fatal("expected ssh bootstrap error without credentials")
	}
	if _, err := ep.Dial(t.Context()); err == nil {
		t.Fatal("expected dial to fail on placeholder socket")
	}
}

func TestBootstrapEndpointNeverEmpty(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	ep := transport.NewEndpoint()
	if err := bootstrapEndpoint(ep); err != nil {
		t.Logf("bootstrap warning (ok without engine): %v", err)
	}
	if _, err := ep.Dial(t.Context()); err == nil {
		// placeholder unix may exist on CI hosts with podman
		t.Log("dial succeeded against local socket")
	}
}
