package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		network string
		address string
		err     string
	}{
		{in: "unix:///run/podman/podman.sock", network: "unix", address: "/run/podman/podman.sock"},
		{in: "unix:/run/podman/podman.sock", network: "unix", address: "/run/podman/podman.sock"},
		{in: "/run/podman/podman.sock", network: "unix", address: "/run/podman/podman.sock"},
		{in: "  unix:///tmp/p.sock  ", network: "unix", address: "/tmp/p.sock"},
		{in: "tcp://127.0.0.1:8080", network: "tcp", address: "127.0.0.1:8080"},
		{in: "http://127.0.0.1:1234/ignored", network: "tcp", address: "127.0.0.1:1234"},
		{in: "http://127.0.0.1", network: "tcp", address: "127.0.0.1"},
		{in: "https://example.com", network: "tcp", address: "example.com"},
		{in: "http://[::1]:8080", network: "tcp", address: "[::1]:8080"},
		{in: "http://[::1", err: "missing ']' in host"},
		{in: "", err: "empty connection URI"},
		{in: "   ", err: "empty connection URI"},
		{in: "ssh://user@host", err: "ssh connections are not supported"},
		{in: "ftp://host", err: "unsupported connection URI"},
	}
	for _, tc := range cases {
		name := tc.in
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseConn(tc.in)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want substring %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Network != tc.network || got.Address != tc.address {
				t.Fatalf("got %+v, want %s %s", got, tc.network, tc.address)
			}
		})
	}
}

func TestDetectConnContainerHostWins(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "unix:///from-container")
	t.Setenv("PODMAN_HOST", "unix:///from-podman")
	got, err := DetectConn()
	if err != nil {
		t.Fatal(err)
	}
	if got != (Conn{Network: "unix", Address: "/from-container"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectConnPodmanHost(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "tcp://127.0.0.1:9999")
	got, err := DetectConn()
	if err != nil {
		t.Fatal(err)
	}
	if got != (Conn{Network: "tcp", Address: "127.0.0.1:9999"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectConnXDGRuntimeDir(t *testing.T) {
	xdg := t.TempDir()
	sock := filepath.Join(xdg, "podman", "podman.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	machine := filepath.Join(home, ".local", "share", "containers", "podman", "machine")
	if err := os.MkdirAll(machine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(machine, "podman.sock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	t.Setenv("HOME", home)

	got, err := DetectConn()
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != sock {
		t.Fatalf("address = %s, want XDG socket %s", got.Address, sock)
	}
}

func TestDetectConnHomeMachineSocket(t *testing.T) {
	home := t.TempDir()
	sock := filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HOME", home)

	got, err := DetectConn()
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != sock {
		t.Fatalf("address = %s, want %s", got.Address, sock)
	}
}

func TestDetectConnSkipsDirectoryCandidate(t *testing.T) {
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "podman", "podman.sock"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	sock := filepath.Join(home, ".config", "containers", "podman", "machine", "podman.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	t.Setenv("HOME", home)

	got, err := DetectConn()
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != sock {
		t.Fatalf("address = %s, want config socket %s", got.Address, sock)
	}
}

func TestDetectConnMissing(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, p := range []string{"/run/podman/podman.sock", "/var/run/podman/podman.sock"} {
		if _, err := os.Stat(p); err == nil {
			t.Skipf("system socket %s is present", p)
		}
	}
	_, err := DetectConn()
	if err == nil {
		t.Fatal("expected error when no socket exists")
	}
	if !strings.Contains(err.Error(), "no Podman socket found") {
		t.Fatalf("err = %v", err)
	}
}

func TestDetectConnInvalidEnv(t *testing.T) {
	t.Setenv("CONTAINER_HOST", "ssh://root@host")
	_, err := DetectConn()
	if err == nil || !strings.Contains(err.Error(), "ssh") {
		t.Fatalf("err = %v", err)
	}
}
