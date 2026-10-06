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

func TestDetectConnFallbackOnUnsupported(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "ssh://root@host")
	got := detectConn()
	if got != (transport.Conn{Network: "unix", Address: "/run/podman/podman.sock"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectConnNeverEmpty(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	got := detectConn()
	if got.Network == "" || got.Address == "" {
		t.Fatalf("got %+v", got)
	}
}
